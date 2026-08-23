<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Expand, Home, LoaderCircle, MousePointer2, Play, Power, RotateCcw, Square, StopCircle, Volume1, Volume2 } from 'lucide-vue-next'
import { api, toAppError } from '@/services/api'
import { useUiStore } from '@/stores/ui'

const props = defineProps<{ deviceId: string; active: boolean }>()
const ui = useUiStore()
const frame = ref('')
const status = ref<'idle' | 'connecting' | 'live' | 'error'>('idle')
const frameCount = ref(0)
const lastFrameAt = ref('')
const connected = ref(false)
const container = ref<HTMLElement>()
let socket: WebSocket | undefined
let intentionalClose = false
let connectionSeq = 0

const statusText = computed(() => ({ idle: '未开始', connecting: '正在连接画面', live: '实时投屏中', error: '画面已断开' })[status.value])
const canStop = computed(() => connected.value || status.value === 'connecting')

function start() {
  if (socket?.readyState === WebSocket.CONNECTING || socket?.readyState === WebSocket.OPEN) return
  status.value = 'connecting'
  connected.value = true
  intentionalClose = false
  const seq = connectionSeq + 1
  connectionSeq = seq
  const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const current = new WebSocket(`${protocol}//${location.host}/api/v1/devices/${encodeURIComponent(props.deviceId)}/screen`)
  socket = current
  current.binaryType = 'blob'
  current.onmessage = (event) => {
    if (seq !== connectionSeq) return
    if (!(event.data instanceof Blob)) return
    if (frame.value) URL.revokeObjectURL(frame.value)
    frame.value = URL.createObjectURL(event.data)
    frameCount.value += 1
    lastFrameAt.value = new Date().toLocaleTimeString()
    status.value = 'live'
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
  socket = undefined
  connected.value = false
  status.value = 'idle'
}

function reconnect() {
  stop()
  start()
}

async function action(type: string, key?: string) {
  try {
    await api(`/devices/${encodeURIComponent(props.deviceId)}/actions`, { method: 'POST', body: JSON.stringify({ type, key }) })
  } catch (error) {
    ui.failure(toAppError(error))
  }
}

async function tap(event: MouseEvent) {
  const image = event.currentTarget as HTMLImageElement
  const rect = image.getBoundingClientRect()
  const x = Math.round((event.clientX - rect.left) * image.naturalWidth / rect.width)
  const y = Math.round((event.clientY - rect.top) * image.naturalHeight / rect.height)
  try {
    await api(`/devices/${encodeURIComponent(props.deviceId)}/actions`, { method: 'POST', body: JSON.stringify({ type: 'tap', x, y }) })
  } catch (error) {
    ui.failure(toAppError(error))
  }
}

function fullscreen() { container.value?.requestFullscreen() }

watch(() => props.active, (active) => active ? start() : stop(), { immediate: true })
onBeforeUnmount(() => { stop(); if (frame.value) URL.revokeObjectURL(frame.value) })
</script>

<template>
  <div ref="container" class="card overflow-hidden bg-[#090d0b] dark:bg-black">
    <div class="flex flex-wrap items-center gap-2 border-b border-white/8 px-4 py-2 text-white">
      <span class="mr-2 size-1.5 rounded-full" :class="status === 'live' ? 'bg-brand-500' : status === 'error' ? 'bg-red-500' : 'bg-amber-400 animate-pulse-soft'" />
      <span class="text-xs font-medium">{{ statusText }}</span>
      <span class="hidden font-mono text-[10px] text-slate-500 sm:inline">帧 {{ frameCount }} · {{ lastFrameAt || '等待首帧' }}</span>
      <div class="ml-auto flex items-center gap-1.5">
        <button class="btn-secondary !h-8 !border-white/8 !bg-white/5 !px-2.5 !text-xs !text-slate-200 hover:!bg-white/10" :disabled="canStop" @click="start"><Play :size="14" />开始</button>
        <button class="btn-secondary !h-8 !border-white/8 !bg-white/5 !px-2.5 !text-xs !text-slate-200 hover:!bg-white/10" @click="reconnect"><RotateCcw :size="14" />重连</button>
        <button class="btn-secondary !h-8 !border-white/8 !bg-white/5 !px-2.5 !text-xs !text-slate-200 hover:!bg-white/10" :disabled="!canStop" @click="stop"><StopCircle :size="14" />停止</button>
        <button class="grid size-8 place-items-center rounded-lg text-slate-400 hover:bg-white/8 hover:text-white" aria-label="全屏" @click="fullscreen"><Expand :size="16" /></button>
      </div>
    </div>
    <div class="relative grid min-h-[460px] place-items-center p-5">
      <img v-if="frame" :src="frame" alt="设备实时屏幕" class="max-h-[680px] max-w-full cursor-crosshair select-none rounded-md object-contain shadow-2xl" draggable="false" @click="tap" />
      <div v-else class="text-center text-slate-500"><LoaderCircle v-if="status === 'connecting'" :size="28" class="mx-auto mb-3 animate-spin text-brand-500" /><MousePointer2 v-else :size="28" class="mx-auto mb-3" /><p class="m-0 text-sm">{{ statusText }}</p><p class="mt-2 text-xs">进入本页会自动连接，也可以手动重连。</p></div>
    </div>
    <div class="flex flex-wrap items-center justify-center gap-1.5 border-t border-white/8 bg-white/[.025] p-2.5">
      <button class="icon-button !border-white/8 !text-slate-400 hover:!bg-white/8 hover:!text-white" title="返回" @click="action('key', 'BACK')"><RotateCcw :size="17" /></button>
      <button class="icon-button !border-white/8 !text-slate-400 hover:!bg-white/8 hover:!text-white" title="主页" @click="action('key', 'HOME')"><Home :size="17" /></button>
      <button class="icon-button !border-white/8 !text-slate-400 hover:!bg-white/8 hover:!text-white" title="最近任务" @click="action('key', 'APP_SWITCH')"><Square :size="16" /></button>
      <span class="mx-1 h-5 w-px bg-white/10" />
      <button class="icon-button !border-white/8 !text-slate-400 hover:!bg-white/8 hover:!text-white" title="音量减" @click="action('key', 'VOLUME_DOWN')"><Volume1 :size="17" /></button>
      <button class="icon-button !border-white/8 !text-slate-400 hover:!bg-white/8 hover:!text-white" title="音量加" @click="action('key', 'VOLUME_UP')"><Volume2 :size="17" /></button>
      <button class="icon-button !border-white/8 !text-slate-400 hover:!bg-white/8 hover:!text-white" title="电源键" @click="action('key', 'POWER')"><Power :size="17" /></button>
    </div>
  </div>
</template>
