<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Expand, Home, LoaderCircle, MousePointer2, Play, Power, RotateCcw, SlidersHorizontal, Square, StopCircle, Volume1, Volume2 } from 'lucide-vue-next'
import { api, toAppError } from '@/services/api'
import { useUiStore } from '@/stores/ui'
import UiSelect from '@/components/common/UiSelect.vue'

const props = defineProps<{ deviceId: string; active: boolean }>()
const ui = useUiStore()
const canvas = ref<HTMLCanvasElement>()
const status = ref<'idle' | 'connecting' | 'live' | 'error'>('idle')
const frameCount = ref(0)
const lastFrameAt = ref('')
const connected = ref(false)
const container = ref<HTMLElement>()
const requestedFPS = ref('30')
const streamError = ref('')
const streamSize = ref({ width: 0, height: 0 })
const activePointers = new Set<number>()
const lastTouchMoveAt = new Map<number, number>()
let socket: WebSocket | undefined
let decoder: VideoDecoder | undefined
let codecConfig: Uint8Array | undefined
let intentionalClose = false
let connectionSeq = 0
let actionInFlight = false
let lastActionAt = 0

const statusText = computed(() => streamError.value || ({ idle: '投屏未开启', connecting: '正在启动实时投屏', live: '实时投屏运行中', error: '画面已断开' })[status.value])
const canStop = computed(() => connected.value || status.value === 'connecting')

function start() {
  if (socket?.readyState === WebSocket.CONNECTING || socket?.readyState === WebSocket.OPEN) return
  if (!('VideoDecoder' in window)) {
    streamError.value = '当前浏览器不支持 WebCodecs H.264 解码，请使用最新版 Chrome、Edge 或 Safari'
    status.value = 'error'
    ui.failure(toAppError({ errorCode: 'WEBCODECS_UNSUPPORTED', message: streamError.value, module: 'device.screen' }))
    return
  }
  status.value = 'connecting'
  streamError.value = ''
  connected.value = true
  intentionalClose = false
  const seq = connectionSeq + 1
  connectionSeq = seq
  const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const current = new WebSocket(`${protocol}//${location.host}/api/v1/devices/${encodeURIComponent(props.deviceId)}/screen?fps=${encodeURIComponent(requestedFPS.value)}`)
  socket = current
  current.binaryType = 'arraybuffer'
  current.onmessage = async (event) => {
    if (seq !== connectionSeq) return
    if (typeof event.data === 'string') {
      const meta = JSON.parse(event.data) as { type?: string; width?: number; height?: number; message?: string }
      if (meta.type === 'error') {
        streamError.value = meta.message || '实时投屏启动失败'
        status.value = 'error'
        ui.failure(toAppError({ errorCode: 'SCREEN_STREAM_FAILED', message: streamError.value, module: 'device.screen' }))
        return
      }
      if (meta.type !== 'meta' || !meta.width || !meta.height) { streamError.value = '投屏元数据无效'; status.value = 'error'; return }
      streamSize.value = { width: meta.width, height: meta.height }
      decoder?.close(); codecConfig = undefined
      decoder = new VideoDecoder({ output: (videoFrame) => {
        const target = canvas.value; if (target) { target.width = videoFrame.displayWidth; target.height = videoFrame.displayHeight; target.getContext('2d')?.drawImage(videoFrame, 0, 0) }
        videoFrame.close(); frameCount.value += 1; lastFrameAt.value = new Date().toLocaleTimeString(); status.value = 'live'
      }, error: (error) => { if (seq === connectionSeq) { streamError.value = `视频解码失败：${error.message}`; status.value = 'error' } } })
      return
    }
    if (!(event.data instanceof ArrayBuffer) || !decoder || event.data.byteLength < 2) return
    const bytes = new Uint8Array(event.data); const kind = bytes[0]; const data = bytes.slice(1)
    if (kind === 1) {
      codecConfig = data
      decoder.configure({ codec: codecName(data), optimizeForLatency: true })
      return
    }
    if (kind === 2) {
      if (!codecConfig) return
      const combined = new Uint8Array(codecConfig.length + data.length); combined.set(codecConfig); combined.set(data, codecConfig.length)
      decoder.decode(new EncodedVideoChunk({ type: 'key', timestamp: performance.now() * 1000, data: combined }))
    } else decoder.decode(new EncodedVideoChunk({ type: 'delta', timestamp: performance.now() * 1000, data }))
  }
  current.onerror = () => {
    if (seq === connectionSeq) status.value = 'error'
  }
  current.onclose = () => {
    if (socket === current) socket = undefined
    if (seq === connectionSeq) connected.value = false
    if (seq === connectionSeq && !intentionalClose && props.active) status.value = 'error'
  }
}

function stop() {
  connectionSeq += 1
  intentionalClose = true
  socket?.close()
  decoder?.close(); decoder = undefined; codecConfig = undefined
  activePointers.clear(); lastTouchMoveAt.clear()
  socket = undefined
  connected.value = false
  status.value = 'idle'
}

function reconnect() {
  stop()
  start()
}

async function sendAction(payload: Record<string, unknown>) {
  if (!reserveActionSlot()) return
  try {
    await api(`/devices/${encodeURIComponent(props.deviceId)}/actions`, { method: 'POST', body: JSON.stringify(payload) })
  } catch (error) {
    ui.failure(toAppError(error))
  } finally {
    actionInFlight = false
  }
}

function imagePoint(event: PointerEvent) {
  const image = event.currentTarget as HTMLCanvasElement
  const rect = image.getBoundingClientRect()
  return {
    x: Math.max(0, Math.min(streamSize.value.width - 1, Math.round((event.clientX - rect.left) * streamSize.value.width / rect.width))),
    y: Math.max(0, Math.min(streamSize.value.height - 1, Math.round((event.clientY - rect.top) * streamSize.value.height / rect.height))),
    width: streamSize.value.width,
    height: streamSize.value.height,
  }
}

function beginPointer(event: PointerEvent) {
  const point = imagePoint(event)
  activePointers.add(event.pointerId)
  ;(event.currentTarget as HTMLElement).setPointerCapture?.(event.pointerId)
  sendTouch(event.pointerId, 0, point)
}

function finishPointer(event: PointerEvent) {
  if (!activePointers.delete(event.pointerId)) return
  lastTouchMoveAt.delete(event.pointerId)
  const end = imagePoint(event)
  sendTouch(event.pointerId, 1, end)
}

function movePointer(event: PointerEvent) {
  const now = Date.now()
  if (!activePointers.has(event.pointerId) || now - (lastTouchMoveAt.get(event.pointerId) ?? 0) < 16) return
  lastTouchMoveAt.set(event.pointerId, now)
  const point = imagePoint(event)
  sendTouch(event.pointerId, 2, point)
}

function cancelPointer(event: PointerEvent) {
  if (!activePointers.delete(event.pointerId)) return
  const point = imagePoint(event)
  lastTouchMoveAt.delete(event.pointerId)
  sendTouch(event.pointerId, 1, point)
}

function sendTouch(pointerId: number, action: number, point: { x: number; y: number; width: number; height: number }) {
  if (socket?.readyState !== WebSocket.OPEN) return
  socket.send(JSON.stringify({ type: 'touch', action, pointerId: pointerId >>> 0, ...point }))
}

function codecName(config: Uint8Array) {
  for (let index = 0; index + 7 < config.length; index += 1) {
    const start = config[index] === 0 && config[index + 1] === 0 && (config[index + 2] === 1 || (config[index + 2] === 0 && config[index + 3] === 1))
    if (!start) continue
    const offset = index + (config[index + 2] === 1 ? 3 : 4)
    if ((config[offset] & 0x1f) !== 7) continue
    return `avc1.${[config[offset + 1], config[offset + 2], config[offset + 3]].map((value) => value.toString(16).padStart(2, '0')).join('').toUpperCase()}`
  }
  return 'avc1.42E01E'
}

function reserveActionSlot() {
  const now = Date.now()
  if (actionInFlight || now - lastActionAt < 220) return false
  actionInFlight = true
  lastActionAt = now
  return true
}

function fullscreen() { container.value?.requestFullscreen() }

watch(() => props.active, (active) => { if (!active) stop() }, { immediate: true })
watch(requestedFPS, () => { if (connected.value) reconnect() })
onBeforeUnmount(() => { stop() })
</script>

<template>
  <div ref="container" class="card overflow-hidden bg-[#090d0b] dark:bg-black">
    <div class="flex flex-wrap items-center gap-2 border-b border-white/8 px-4 py-2 text-white">
      <span class="mr-2 size-1.5 rounded-full" :class="status === 'live' ? 'bg-brand-500' : status === 'error' ? 'bg-red-500' : 'bg-amber-400 animate-pulse-soft'" />
      <span class="text-xs font-medium">{{ statusText }}</span>
      <span class="hidden font-mono text-[10px] text-slate-500 sm:inline">帧 {{ frameCount }} · {{ lastFrameAt || '等待首帧' }}</span>
      <div class="ml-auto flex items-center gap-1.5">
        <button class="btn-secondary !h-8 !border-white/8 !bg-white/5 !px-2.5 !text-xs !text-slate-200 hover:!bg-white/10" :disabled="canStop" @click="start"><Play :size="14" />开启投屏</button>
        <button class="btn-secondary !h-8 !border-white/8 !bg-white/5 !px-2.5 !text-xs !text-slate-200 hover:!bg-white/10" @click="reconnect"><RotateCcw :size="14" />重连</button>
        <button class="btn-secondary !h-8 !border-white/8 !bg-white/5 !px-2.5 !text-xs !text-slate-200 hover:!bg-white/10" :disabled="!canStop" @click="stop"><StopCircle :size="14" />停止投屏</button>
        <button class="grid size-8 place-items-center rounded-lg text-slate-400 hover:bg-white/8 hover:text-white" aria-label="全屏" @click="fullscreen"><Expand :size="16" /></button>
      </div>
    </div>
    <div class="flex flex-wrap items-center gap-3 border-b border-white/8 bg-white/[.025] px-4 py-2.5 text-xs text-slate-300">
      <SlidersHorizontal :size="15" class="text-brand-400" />
      <span class="font-medium text-slate-200">投屏参数</span>
      <label class="flex items-center gap-2">刷新帧率
        <UiSelect v-model="requestedFPS" class="!h-8 !w-32 !border-white/10 !bg-white/5 !text-xs !text-slate-100" aria-label="投屏刷新帧率">
          <option value="15">15 FPS · 省流</option><option value="30">30 FPS · 流畅</option><option value="60">60 FPS · 高刷</option>
        </UiSelect>
      </label>
      <span class="rounded-md border border-brand-400/20 bg-brand-400/10 px-2 py-1 text-brand-200">后端：scrcpy H.264 视频流</span>
      <span class="text-slate-400">移动端支持单指、多指触摸与拖动控制</span>
    </div>
    <div class="relative grid min-h-[460px] place-items-center p-5">
      <canvas v-if="streamSize.width" ref="canvas" aria-label="设备实时屏幕" class="max-h-[680px] max-w-full touch-none cursor-crosshair select-none rounded-md object-contain shadow-2xl" @pointerdown.prevent="beginPointer" @pointermove.prevent="movePointer" @pointerup.prevent="finishPointer" @pointercancel.prevent="cancelPointer" />
      <div v-else class="text-center text-slate-500"><LoaderCircle v-if="status === 'connecting'" :size="28" class="mx-auto mb-3 animate-spin text-brand-500" /><MousePointer2 v-else :size="28" class="mx-auto mb-3" /><p class="m-0 text-sm">{{ statusText }}</p><p class="mt-2 text-xs">请点击“开启投屏”；画面支持点击和拖动滑动。</p></div>
    </div>
    <div class="flex flex-wrap items-center justify-center gap-1.5 border-t border-white/8 bg-white/[.025] p-2.5">
      <button class="icon-button !border-white/8 !text-slate-400 hover:!bg-white/8 hover:!text-white" title="返回" @click="sendAction({ type: 'key', key: 'BACK' })"><RotateCcw :size="17" /></button>
      <button class="icon-button !border-white/8 !text-slate-400 hover:!bg-white/8 hover:!text-white" title="主页" @click="sendAction({ type: 'key', key: 'HOME' })"><Home :size="17" /></button>
      <button class="icon-button !border-white/8 !text-slate-400 hover:!bg-white/8 hover:!text-white" title="最近任务" @click="sendAction({ type: 'key', key: 'APP_SWITCH' })"><Square :size="16" /></button>
      <span class="mx-1 h-5 w-px bg-white/10" />
      <button class="icon-button !border-white/8 !text-slate-400 hover:!bg-white/8 hover:!text-white" title="音量减" @click="sendAction({ type: 'key', key: 'VOLUME_DOWN' })"><Volume1 :size="17" /></button>
      <button class="icon-button !border-white/8 !text-slate-400 hover:!bg-white/8 hover:!text-white" title="音量加" @click="sendAction({ type: 'key', key: 'VOLUME_UP' })"><Volume2 :size="17" /></button>
      <button class="icon-button !border-white/8 !text-slate-400 hover:!bg-white/8 hover:!text-white" title="电源键" @click="sendAction({ type: 'key', key: 'POWER' })"><Power :size="17" /></button>
    </div>
  </div>
</template>
