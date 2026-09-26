<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Expand, Home, LoaderCircle, Minimize2, MousePointer2, Play, Power, RotateCcw, SlidersHorizontal, Square, StopCircle, Volume1, Volume2 } from 'lucide-vue-next'
import { api, toAppError } from '@/services/api'
import { companionReinstallDescription, companionReinstallRequired } from '@/services/companionInstall'
import { reportClientError } from '@/services/logs'
import { browserScreenDecoderPreference, createScreenDecoder, screenDecoderFallback, screenDecoderStatusText, type ScreenDecoder, type ScreenDecoderMode } from '@/services/screenDecoder'
import { screenGestureAction, type ScreenGestureStart, type ScreenPoint } from '@/services/screenControl'
import { parseScreenPacket } from '@/services/screenPacket'
import { useUiStore } from '@/stores/ui'
import UiSelect from '@/components/common/UiSelect.vue'
import ConfirmDialog from '@/components/feedback/ConfirmDialog.vue'

const props = defineProps<{ deviceId: string; active: boolean }>()
const ui = useUiStore()
const canvas = ref<HTMLCanvasElement>()
const video = ref<HTMLVideoElement>()
const status = ref<'idle' | 'connecting' | 'live' | 'error'>('idle')
const frameCount = ref(0)
const lastFrameAt = ref('')
const connected = ref(false)
const container = ref<HTMLElement>()
const requestedFPS = ref('30')
const streamError = ref('')
const streamSize = ref({ width: 0, height: 0 })
const controlTransport = ref<'scrcpy-control' | 'companion-accessibility'>('scrcpy-control')
const streamProtocol = ref(1)
const isFullscreen = ref(false)
const companionPrompt = ref(false)
const companionReinstallPrompt = ref(false)
const companionReinstallError = ref<ReturnType<typeof toAppError> | null>(null)
const companionInstalling = ref(false)
const pendingCompanionAction = ref<Record<string, unknown> | null>(null)
const activePointers = new Set<number>()
const lastTouchMoveAt = new Map<number, number>()
const gestureStarts = new Map<number, ScreenGestureStart>()
let socket: WebSocket | undefined
let decoder: ScreenDecoder | undefined
let decoderPacketQueue = Promise.resolve()
const decoderMode = ref<ScreenDecoderMode>(browserScreenDecoderPreference())
let intentionalClose = false
let connectionSeq = 0
let actionInFlight = false
let lastActionAt = 0
let firstFrameTimer: number | undefined
let gestureActionQueue = Promise.resolve()

const statusText = computed(() => streamError.value || ({ idle: '投屏未开启', connecting: '正在启动实时投屏', live: '实时投屏运行中', error: '画面已断开' })[status.value])
const canStop = computed(() => connected.value || status.value === 'connecting')

function start() {
  if (socket?.readyState === WebSocket.CONNECTING || socket?.readyState === WebSocket.OPEN) return
  decoderMode.value = browserScreenDecoderPreference()
  status.value = 'connecting'
  streamError.value = ''
  frameCount.value = 0
  lastFrameAt.value = ''
  connected.value = true
  intentionalClose = false
  const seq = connectionSeq + 1
  connectionSeq = seq
  const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const current = new WebSocket(`${protocol}//${location.host}/api/v1/devices/${encodeURIComponent(props.deviceId)}/screen?fps=${encodeURIComponent(requestedFPS.value)}&protocol=2`)
  socket = current
  current.binaryType = 'arraybuffer'
  decoderPacketQueue = Promise.resolve()
  current.onmessage = (event) => {
    if (seq !== connectionSeq) return
    decoderPacketQueue = decoderPacketQueue.then(() => handleStreamMessage(event, seq)).catch((error: unknown) => {
      if (seq !== connectionSeq) return
      const reason = error instanceof Error ? error.message : String(error)
      streamError.value = `视频解码失败：${reason}`
      status.value = 'error'
      reportScreenError('SCREEN_DECODER_FAILED', streamError.value, { decoder: decoderMode.value })
      ui.failure(toAppError({ errorCode: 'SCREEN_DECODER_FAILED', message: streamError.value, module: 'device.screen' }))
    })
  }
  current.onerror = () => {
    if (seq === connectionSeq) status.value = 'error'
  }
  current.onclose = () => {
    if (socket === current) socket = undefined
    if (seq === connectionSeq) connected.value = false
    if (seq === connectionSeq && !intentionalClose && props.active) {
      status.value = 'error'
      if (frameCount.value === 0) reportScreenError('SCREEN_STREAM_CLOSED_BEFORE_FRAME', '投屏连接在首帧显示前关闭', { decoder: decoderMode.value })
    }
  }
}

async function handleStreamMessage(event: MessageEvent, seq: number) {
    if (seq !== connectionSeq) return
    if (typeof event.data === 'string') {
      const meta = JSON.parse(event.data) as { type?: string; width?: number; height?: number; message?: string; controlTransport?: string; streamProtocol?: number }
      if (meta.type === 'error') {
        streamError.value = meta.message || '实时投屏启动失败'
        status.value = 'error'
        ui.failure(toAppError({ errorCode: 'SCREEN_STREAM_FAILED', message: streamError.value, module: 'device.screen' }))
        return
      }
      if (meta.type !== 'meta' || !meta.width || !meta.height) { streamError.value = '投屏元数据无效'; status.value = 'error'; return }
      streamSize.value = { width: meta.width, height: meta.height }
      controlTransport.value = meta.controlTransport === 'companion-accessibility' ? 'companion-accessibility' : 'scrcpy-control'
      streamProtocol.value = meta.streamProtocol === 2 ? 2 : 1
      decoder?.dispose()
      decoder = undefined
      await nextTick()
      const created = await createCurrentDecoder(seq)
      if (seq !== connectionSeq) { created.dispose(); return }
      decoder = created
      armFirstFrameTimer(seq)
      return
    }
    if (!(event.data instanceof ArrayBuffer) || !decoder || event.data.byteLength < 2) return
    const packet = parseScreenPacket(new Uint8Array(event.data), streamProtocol.value)
    const { kind, data, presentationTimeUs } = packet
    if (kind === 1) {
      try {
        await decoder.configure(data)
      } catch (error) {
        const fallback = screenDecoderFallback(decoderMode.value)
        if (!fallback) throw error
        const failedMode = decoderMode.value
        decoder.dispose()
        decoder = undefined
        decoderMode.value = fallback
        await nextTick()
        const replacement = await createCurrentDecoder(seq)
        if (seq !== connectionSeq) { replacement.dispose(); return }
        decoder = replacement
        await replacement.configure(data)
        reportScreenError('SCREEN_DECODER_FALLBACK', `${screenDecoderStatusText(failedMode)} 初始化失败，已自动切换为软件视频解码`, {
          decoder: failedMode,
          fallback,
          reason: error instanceof Error ? error.message : String(error),
        })
      }
      return
    }
    await decoder.decode(data, kind === 2, presentationTimeUs)
}

async function createCurrentDecoder(seq: number) {
  const target = decoderMode.value === 'mse' ? video.value : canvas.value
  if (!target) throw new Error('投屏显示组件尚未就绪')
  return createScreenDecoder(decoderMode.value, target, {
    onFrame: () => {
      if (seq !== connectionSeq) return
      clearFirstFrameTimer()
      frameCount.value += 1
      lastFrameAt.value = new Date().toLocaleTimeString()
      status.value = 'live'
    },
    onError: (error) => {
      if (seq !== connectionSeq) return
      streamError.value = `视频解码失败：${error.message}`
      status.value = 'error'
      clearFirstFrameTimer()
      reportScreenError('SCREEN_DECODER_CALLBACK_FAILED', streamError.value, { decoder: decoderMode.value })
    },
  }, Number(requestedFPS.value))
}

function stop() {
  connectionSeq += 1
  intentionalClose = true
  socket?.close()
  decoder?.dispose(); decoder = undefined
  clearFirstFrameTimer()
  decoderPacketQueue = Promise.resolve()
  activePointers.clear(); lastTouchMoveAt.clear(); gestureStarts.clear()
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
    const appError = toAppError(error)
    if (['COMPANION_INSTALL_REQUIRED', 'COMPANION_UPGRADE_REQUIRED'].includes(appError.errorCode)) {
      pendingCompanionAction.value = payload
      companionPrompt.value = true
    } else {
      ui.failure(appError)
    }
  } finally {
    actionInFlight = false
  }
}

async function installCompanionAndRetry() {
  if (companionInstalling.value) return
  companionInstalling.value = true
  try {
    const result = await api<{ installedVersionName?: string; installedVersionCode: number }>(`/devices/${encodeURIComponent(props.deviceId)}/companion/install`, { method: 'POST', body: '{}' })
    companionPrompt.value = false
    ui.notify('伴侣应用已安装', `设备版本：${result.installedVersionName || result.installedVersionCode}；正在重试控制操作。`, 'success')
    const pending = pendingCompanionAction.value
    pendingCompanionAction.value = null
    actionInFlight = false
    lastActionAt = 0
    if (pending) await sendAction(pending)
  } catch (error) {
    const appError = toAppError(error)
    if (companionReinstallRequired(appError)) {
      companionPrompt.value = false
      companionReinstallError.value = appError
      companionReinstallPrompt.value = true
    } else {
      ui.failure(appError)
    }
  } finally {
    companionInstalling.value = false
  }
}

async function reinstallCompanionAndRetry() {
  if (companionInstalling.value) return
  companionInstalling.value = true
  try {
    const result = await api<{ installedVersionName?: string; installedVersionCode: number }>(`/devices/${encodeURIComponent(props.deviceId)}/companion/reinstall`, { method: 'POST', body: '{}' })
    companionReinstallPrompt.value = false
    companionReinstallError.value = null
    ui.notify('伴侣应用已重新安装', `设备版本：${result.installedVersionName || result.installedVersionCode}；请重新授予伴侣权限，正在重试控制操作。`, 'success')
    const pending = pendingCompanionAction.value
    pendingCompanionAction.value = null
    actionInFlight = false
    lastActionAt = 0
    if (pending) await sendAction(pending)
  } catch (error) {
    ui.failure(toAppError(error))
  } finally {
    companionInstalling.value = false
  }
}

function imagePoint(event: PointerEvent) {
  const image = event.currentTarget as HTMLElement
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
  gestureStarts.set(event.pointerId, { point, startedAt: Date.now() })
  ;(event.currentTarget as HTMLElement).setPointerCapture?.(event.pointerId)
  if (controlTransport.value === 'scrcpy-control') sendTouch(event.pointerId, 0, point)
}

function finishPointer(event: PointerEvent) {
  if (!activePointers.delete(event.pointerId)) return
  lastTouchMoveAt.delete(event.pointerId)
  const end = imagePoint(event)
  const start = gestureStarts.get(event.pointerId)
  gestureStarts.delete(event.pointerId)
  if (controlTransport.value === 'scrcpy-control') {
    sendTouch(event.pointerId, 1, end)
  } else if (start) {
    queueGestureAction(screenGestureAction(start, end, Date.now()))
  }
}

function movePointer(event: PointerEvent) {
  const now = Date.now()
  if (!activePointers.has(event.pointerId) || now - (lastTouchMoveAt.get(event.pointerId) ?? 0) < 16) return
  lastTouchMoveAt.set(event.pointerId, now)
  const point = imagePoint(event)
  if (controlTransport.value === 'scrcpy-control') sendTouch(event.pointerId, 2, point)
}

function cancelPointer(event: PointerEvent) {
  if (!activePointers.delete(event.pointerId)) return
  const point = imagePoint(event)
  lastTouchMoveAt.delete(event.pointerId)
  gestureStarts.delete(event.pointerId)
  if (controlTransport.value === 'scrcpy-control') sendTouch(event.pointerId, 1, point)
}

function sendTouch(pointerId: number, action: number, point: ScreenPoint) {
  if (socket?.readyState !== WebSocket.OPEN) return
  socket.send(JSON.stringify({ type: 'touch', action, pointerId: pointerId >>> 0, ...point }))
}

function queueGestureAction(payload: Record<string, unknown>) {
  gestureActionQueue = gestureActionQueue.then(async () => {
    if (companionPrompt.value || companionReinstallPrompt.value) return
    try {
      await api(`/devices/${encodeURIComponent(props.deviceId)}/actions`, { method: 'POST', body: JSON.stringify(payload) })
    } catch (error) {
      const appError = toAppError(error)
      if (['COMPANION_INSTALL_REQUIRED', 'COMPANION_UPGRADE_REQUIRED'].includes(appError.errorCode)) {
        pendingCompanionAction.value = payload
        companionPrompt.value = true
      } else {
        ui.failure(appError)
      }
    }
  })
}

function reserveActionSlot() {
  const now = Date.now()
  if (actionInFlight || now - lastActionAt < 220) return false
  actionInFlight = true
  lastActionAt = now
  return true
}

function reportScreenError(errorCode: string, message: string, details: Record<string, unknown>) {
  reportClientError({
    errorCode,
    message,
    source: 'screen.stream',
    route: location.pathname,
    details: { deviceId: props.deviceId, ...details },
  })
}

function armFirstFrameTimer(seq: number) {
  clearFirstFrameTimer()
  firstFrameTimer = window.setTimeout(() => {
    if (seq !== connectionSeq || frameCount.value > 0 || status.value === 'idle') return
    streamError.value = `已收到视频流，但 ${screenDecoderStatusText(decoderMode.value)} 在 8 秒内未显示首帧`
    status.value = 'error'
    reportScreenError('SCREEN_FIRST_FRAME_TIMEOUT', streamError.value, { decoder: decoderMode.value })
    ui.failure(toAppError({ errorCode: 'SCREEN_FIRST_FRAME_TIMEOUT', message: streamError.value, module: 'device.screen' }))
  }, 8000)
}

function clearFirstFrameTimer() {
  if (firstFrameTimer !== undefined) window.clearTimeout(firstFrameTimer)
  firstFrameTimer = undefined
}

function syncFullscreenState() {
  isFullscreen.value = document.fullscreenElement === container.value
}

async function toggleFullscreen() {
  try {
    if (document.fullscreenElement === container.value) await document.exitFullscreen()
    else if (container.value) await container.value.requestFullscreen()
  } catch (error) {
    const appError = toAppError({ errorCode: 'SCREEN_FULLSCREEN_FAILED', message: error instanceof Error ? error.message : '无法切换全屏', module: 'device.screen' })
    reportScreenError(appError.errorCode, appError.message, {})
    ui.failure(appError)
  }
}

watch(() => props.active, (active) => { if (!active) stop() }, { immediate: true })
watch(requestedFPS, () => { if (connected.value) reconnect() })
onMounted(() => document.addEventListener('fullscreenchange', syncFullscreenState))
onBeforeUnmount(() => {
  document.removeEventListener('fullscreenchange', syncFullscreenState)
  stop()
})
</script>

<template>
  <div ref="container" class="card overflow-hidden bg-[#090d0b] dark:bg-black" :class="isFullscreen ? 'flex h-screen w-screen flex-col rounded-none' : ''">
    <div class="flex shrink-0 flex-wrap items-center gap-2 border-b border-white/8 px-4 py-2 text-white">
      <span class="mr-2 size-1.5 rounded-full" :class="status === 'live' ? 'bg-brand-500' : status === 'error' ? 'bg-red-500' : 'bg-amber-400 animate-pulse-soft'" />
      <span class="text-xs font-medium">{{ statusText }}</span>
      <span class="hidden font-mono text-[10px] text-slate-500 sm:inline">帧 {{ frameCount }} · {{ lastFrameAt || '等待首帧' }}</span>
      <div class="ml-auto flex items-center gap-1.5">
        <button class="btn-secondary !h-8 !border-white/8 !bg-white/5 !px-2.5 !text-xs !text-slate-200 hover:!bg-white/10" :disabled="canStop" @click="start"><Play :size="14" />开启投屏</button>
        <button class="btn-secondary !h-8 !border-white/8 !bg-white/5 !px-2.5 !text-xs !text-slate-200 hover:!bg-white/10" @click="reconnect"><RotateCcw :size="14" />重连</button>
        <button class="btn-secondary !h-8 !border-white/8 !bg-white/5 !px-2.5 !text-xs !text-slate-200 hover:!bg-white/10" :disabled="!canStop" @click="stop"><StopCircle :size="14" />停止投屏</button>
        <button class="grid size-8 place-items-center rounded-lg text-slate-400 hover:bg-white/8 hover:text-white" :aria-label="isFullscreen ? '退出全屏' : '进入全屏'" :title="isFullscreen ? '退出全屏' : '进入全屏'" @click="toggleFullscreen"><Minimize2 v-if="isFullscreen" :size="16" /><Expand v-else :size="16" /></button>
      </div>
    </div>
    <div class="flex shrink-0 flex-wrap items-center gap-3 border-b border-white/8 bg-white/[.025] px-4 py-2.5 text-xs text-slate-300">
      <SlidersHorizontal :size="15" class="text-brand-400" />
      <span class="font-medium text-slate-200">投屏参数</span>
      <label class="flex items-center gap-2">刷新帧率
        <UiSelect v-model="requestedFPS" class="w-32" control-class="ui-select-dark !h-8 !border-white/10 !bg-[#111916] !text-xs !text-slate-100" aria-label="投屏刷新帧率">
          <option value="15">15 FPS · 省流</option><option value="30">30 FPS · 流畅</option><option value="60">60 FPS · 高刷</option>
        </UiSelect>
      </label>
      <span class="rounded-md border border-brand-400/20 bg-brand-400/10 px-2 py-1 text-brand-200">后端：scrcpy H.264 视频流</span>
      <span class="rounded-md border border-sky-400/20 bg-sky-400/10 px-2 py-1 text-sky-200">解码：{{ screenDecoderStatusText(decoderMode) }}</span>
      <span class="text-slate-400">{{ controlTransport === 'companion-accessibility' ? '控制：伴侣无障碍点击与滑动' : '移动端支持单指、多指触摸与拖动控制' }}</span>
    </div>
    <div class="relative grid place-items-center" :class="isFullscreen ? 'min-h-0 flex-1 p-2 sm:p-4' : 'min-h-[460px] p-5'">
      <video v-if="streamSize.width && decoderMode === 'mse'" ref="video" aria-label="设备实时屏幕" autoplay muted playsinline disablepictureinpicture class="max-w-full touch-none cursor-crosshair select-none rounded-md bg-black object-contain shadow-2xl" :class="isFullscreen ? 'h-full max-h-full w-full' : 'max-h-[680px]'" @pointerdown.prevent="beginPointer" @pointermove.prevent="movePointer" @pointerup.prevent="finishPointer" @pointercancel.prevent="cancelPointer" />
      <canvas v-else-if="streamSize.width" ref="canvas" aria-label="设备实时屏幕" class="max-w-full touch-none cursor-crosshair select-none rounded-md object-contain shadow-2xl" :class="isFullscreen ? 'h-full max-h-full w-full' : 'max-h-[680px]'" @pointerdown.prevent="beginPointer" @pointermove.prevent="movePointer" @pointerup.prevent="finishPointer" @pointercancel.prevent="cancelPointer" />
      <div v-else class="text-center text-slate-500"><LoaderCircle v-if="status === 'connecting'" :size="28" class="mx-auto mb-3 animate-spin text-brand-500" /><MousePointer2 v-else :size="28" class="mx-auto mb-3" /><p class="m-0 text-sm">{{ statusText }}</p><p class="mt-2 text-xs">请点击“开启投屏”；画面支持点击和拖动滑动。</p></div>
    </div>
    <div class="flex shrink-0 flex-wrap items-center justify-center gap-1.5 border-t border-white/8 bg-white/[.025] p-2.5 pb-[max(.625rem,env(safe-area-inset-bottom))]">
      <button class="icon-button !border-white/8 !text-slate-400 hover:!bg-white/8 hover:!text-white" title="返回" @click="sendAction({ type: 'key', key: 'BACK' })"><RotateCcw :size="17" /></button>
      <button class="icon-button !border-white/8 !text-slate-400 hover:!bg-white/8 hover:!text-white" title="主页" @click="sendAction({ type: 'key', key: 'HOME' })"><Home :size="17" /></button>
      <button class="icon-button !border-white/8 !text-slate-400 hover:!bg-white/8 hover:!text-white" title="最近任务" @click="sendAction({ type: 'key', key: 'APP_SWITCH' })"><Square :size="16" /></button>
      <span class="mx-1 h-5 w-px bg-white/10" />
      <button class="icon-button !border-white/8 !text-slate-400 hover:!bg-white/8 hover:!text-white" title="音量减" @click="sendAction({ type: 'key', key: 'VOLUME_DOWN' })"><Volume1 :size="17" /></button>
      <button class="icon-button !border-white/8 !text-slate-400 hover:!bg-white/8 hover:!text-white" title="音量加" @click="sendAction({ type: 'key', key: 'VOLUME_UP' })"><Volume2 :size="17" /></button>
      <button class="icon-button !border-white/8 !text-slate-400 hover:!bg-white/8 hover:!text-white" title="电源键" @click="sendAction({ type: 'key', key: 'POWER' })"><Power :size="17" /></button>
    </div>
  </div>
  <ConfirmDialog
    :open="companionPrompt"
    title="该控制操作需要伴侣能力支持"
    description="设备系统拒绝了直接 ADB 控制，需要 ADBControl Companion 的无障碍能力继续操作。是否安装或覆盖安装服务端内置的伴侣 App？签名不一致时 Android 会拒绝覆盖，不会自动卸载现有应用。"
    :confirm-text="companionInstalling ? '正在安装…' : '安装 / 覆盖安装'"
    @cancel="companionPrompt = false; pendingCompanionAction = null"
    @confirm="installCompanionAndRetry"
  />
  <ConfirmDialog
    :open="companionReinstallPrompt"
    title="无法覆盖安装伴侣应用"
    :description="companionReinstallDescription(companionReinstallError)"
    :confirm-text="companionInstalling ? '正在重新安装…' : '卸载旧版并重新安装'"
    destructive
    @cancel="companionReinstallPrompt = false; companionReinstallError = null; pendingCompanionAction = null"
    @confirm="reinstallCompanionAndRetry"
  />
</template>
