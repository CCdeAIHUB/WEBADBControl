<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { QrCode, RefreshCw, Wifi, X } from 'lucide-vue-next'
import {
	 cancelQRPairing,
	 createQRPairing,
  discoverWirelessDevices,
  normalizePairingCode,
	 pairQRDevice,
  pairWirelessDevice,
	 type QRPairingSession,
  type DiscoveredWirelessService,
} from '@/services/deviceConnection'
import { toAppError } from '@/services/api'
import { useDevicesStore } from '@/stores/devices'
import { useUiStore } from '@/stores/ui'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: [] }>()

type ConnectionMode = 'connect' | 'pair' | 'qr'
type ConnectionPhase = 'idle' | 'discovering' | 'pairing' | 'connecting' | 'creatingQr' | 'qrPairing'

const devices = useDevicesStore()
const ui = useUiStore()
const endpoint = ref('')
const pairingCode = ref('')
const mode = ref<ConnectionMode>('connect')
const phase = ref<ConnectionPhase>('idle')
const discovered = ref<DiscoveredWirelessService[]>([])
const qrSession = ref<QRPairingSession>()
let qrPairingAbort: AbortController | undefined
const busy = computed(() => phase.value !== 'idle')
const visibleServices = computed(() => discovered.value.filter(item => item.type === (mode.value === 'pair' ? 'pairing' : 'connect')))
const qrImage = computed(() => qrSession.value ? `data:image/svg+xml;charset=utf-8,${encodeURIComponent(qrSession.value.qrSvg)}` : '')
const qrExpiry = computed(() => qrSession.value ? new Date(qrSession.value.expiresAt * 1000).toLocaleTimeString() : '')

watch(() => props.open, (open) => {
  if (!open) return
  endpoint.value = ''
  pairingCode.value = ''
  mode.value = 'connect'
  qrSession.value = undefined
  void discover()
})

watch(() => props.open, (open) => {
  if (!open) void discardQRSession()
})

onBeforeUnmount(() => { void discardQRSession() })

async function discover() {
  phase.value = 'discovering'
  try {
    discovered.value = await discoverWirelessDevices()
  } catch (error) {
    discovered.value = []
    ui.failure(toAppError(error))
  } finally {
    phase.value = 'idle'
  }
}

function updatePairingCode(event: Event) {
  pairingCode.value = normalizePairingCode((event.target as HTMLInputElement).value)
}

async function discardQRSession() {
  qrPairingAbort?.abort()
  qrPairingAbort = undefined
  const sessionId = qrSession.value?.sessionId
  qrSession.value = undefined
  if (!sessionId) return
  try { await cancelQRPairing(sessionId) } catch { /* expiry/cancellation is intentionally best-effort */ }
}

async function selectMode(next: ConnectionMode) {
  if (busy.value || next === mode.value) return
  if (mode.value === 'qr') await discardQRSession()
  mode.value = next
  endpoint.value = ''
  if (next === 'qr') await createQR()
}

async function createQR() {
  if (busy.value) return
  await discardQRSession()
  phase.value = 'creatingQr'
  try {
    const session = await createQRPairing()
    qrSession.value = session
    phase.value = 'qrPairing'
    const controller = new AbortController()
    qrPairingAbort = controller
    void completeQRPairing(session, controller)
  } catch (error) {
    ui.failure(toAppError(error))
    phase.value = 'idle'
  }
}

async function completeQRPairing(session: QRPairingSession, controller: AbortController) {
  try {
    const result = await pairQRDevice(session.sessionId, controller.signal)
    if (qrSession.value?.sessionId !== session.sessionId) return
    qrSession.value = undefined
    qrPairingAbort = undefined
    ui.notify('二维码配对成功', `已配对 ${result.serviceName}。请切换到“直接连接”并选择设备显示的连接端口。`, 'success')
    mode.value = 'connect'
    await discover()
  } catch (error) {
    if (controller.signal.aborted) return
    ui.failure(toAppError(error))
  } finally {
    if (qrPairingAbort === controller) qrPairingAbort = undefined
    if (mode.value === 'qr' && qrSession.value?.sessionId === session.sessionId) phase.value = 'idle'
  }
}

async function submit() {
  if (mode.value === 'qr') return
  if (busy.value || !endpoint.value.trim()) return
  phase.value = mode.value === 'pair' ? 'pairing' : 'connecting'
  try {
    if (mode.value === 'pair') {
      await pairWirelessDevice(endpoint.value, pairingCode.value)
      ui.notify('配对成功', '请切换到“直接连接”，并选择设备显示的连接端口。', 'success')
      mode.value = 'connect'
      endpoint.value = ''
      pairingCode.value = ''
      await discover()
      return
    }
    await devices.connect(endpoint.value.trim())
    ui.notify('连接成功', '无线设备已通过 ADB 验证在线。', 'success')
    emit('close')
  } catch (error) {
    ui.failure(toAppError(error))
  } finally {
    phase.value = 'idle'
  }
}
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="fixed inset-0 z-[80] grid place-items-center bg-black/35 p-4 backdrop-blur-[2px]" @click.self="emit('close')">
      <section class="surface w-full max-w-lg rounded-xl p-5 shadow-2xl" role="dialog" aria-modal="true" aria-labelledby="wireless-dialog-title">
        <div class="flex items-center"><div><div class="eyebrow">Wireless ADB</div><h2 id="wireless-dialog-title" class="mt-1 text-lg font-semibold">连接无线设备</h2></div><button class="icon-button ml-auto" aria-label="关闭连接窗口" @click="emit('close')"><X :size="18" /></button></div>
        <p class="mt-3 text-sm leading-6 text-slate-500 dark:text-slate-400">在 Android“无线调试”页面中，配对端口与连接端口不同；请按当前步骤复制对应地址。</p>
        <div class="mt-4 flex rounded-lg bg-slate-100 p-1 dark:bg-white/5" role="tablist" aria-label="连接方式"><button class="h-8 flex-1 rounded-md text-xs font-medium" role="tab" :aria-selected="mode === 'connect'" :class="mode === 'connect' ? 'bg-white text-brand-700 shadow-sm dark:bg-white/10 dark:text-brand-300' : 'text-slate-500'" @click="selectMode('connect')">直接连接</button><button class="h-8 flex-1 rounded-md text-xs font-medium" role="tab" :aria-selected="mode === 'pair'" :class="mode === 'pair' ? 'bg-white text-brand-700 shadow-sm dark:bg-white/10 dark:text-brand-300' : 'text-slate-500'" @click="selectMode('pair')">六位码配对</button><button class="h-8 flex-1 rounded-md text-xs font-medium" role="tab" :aria-selected="mode === 'qr'" :class="mode === 'qr' ? 'bg-white text-brand-700 shadow-sm dark:bg-white/10 dark:text-brand-300' : 'text-slate-500'" @click="selectMode('qr')">扫码配对</button></div>
        <template v-if="mode === 'qr'"><div class="mt-4 rounded-xl border border-brand-200 bg-brand-50/50 p-4 text-center dark:border-brand-500/20 dark:bg-brand-500/5"><QrCode v-if="phase === 'creatingQr'" :size="30" class="mx-auto animate-pulse text-brand-600" /><img v-else-if="qrImage" :src="qrImage" alt="无线 ADB 配对二维码" class="mx-auto size-52 rounded-lg bg-white p-2 shadow-sm" /><p class="mt-3 text-sm font-medium">用设备的“无线调试 → 使用二维码配对设备”扫描</p><p class="mt-1 text-xs text-slate-500 dark:text-slate-400">{{ phase === 'qrPairing' ? '服务器正在自动等待手机广播，扫码成功后会直接进入下一步。' : `二维码于 ${qrExpiry} 失效；配对密码仅保存在服务端核心内存。` }}</p><button class="btn-secondary mt-3 !h-8 !px-3 !text-xs" :disabled="busy" @click="createQR"><RefreshCw :size="14" />刷新二维码</button></div></template>
        <template v-else><div v-if="visibleServices.length" class="mt-3 max-h-28 overflow-auto rounded-lg border border-slate-200 p-1 dark:border-white/9"><button v-for="service in visibleServices" :key="service.endpoint" class="flex w-full items-center rounded-md px-2 py-1.5 text-left font-mono text-[10px] transition hover:bg-slate-50 dark:hover:bg-white/5" @click="endpoint=service.endpoint"><Wifi :size="13" class="mr-2 text-brand-600" />{{ service.endpoint }}</button></div><div v-else-if="phase === 'discovering'" class="mt-3 rounded-lg border border-slate-200 px-3 py-2 text-xs text-slate-400 dark:border-white/9">正在发现局域网设备…</div><label class="mt-4 block text-xs font-medium">{{ mode === 'pair' ? '配对地址' : '连接地址' }}</label><input v-model="endpoint" class="field mt-2 font-mono" :placeholder="mode === 'pair' ? '192.168.1.20:37123' : '192.168.1.20:5555'" autocomplete="off" @keyup.enter="submit" /><template v-if="mode === 'pair'"><label class="mt-4 block text-xs font-medium">六位配对码</label><input :value="pairingCode" inputmode="numeric" maxlength="6" class="field mt-2 font-mono tracking-[.3em]" placeholder="123456" autocomplete="one-time-code" @input="updatePairingCode" @keyup.enter="submit" /><p class="mt-2 text-[11px] leading-5 text-slate-400">配对码会很快过期，且仅发送给当前服务端，不会写入日志或配置。</p></template></template>
        <div class="mt-6 flex justify-end gap-2"><button class="btn-secondary" @click="emit('close')">取消</button><button class="btn-primary" :disabled="busy || mode === 'qr' || !endpoint.trim() || (mode === 'pair' && pairingCode.length !== 6)" @click="submit">{{ phase === 'creatingQr' ? '正在生成…' : phase === 'qrPairing' ? '等待手机扫码…' : phase === 'pairing' ? '正在配对…' : phase === 'connecting' ? '正在连接…' : mode === 'qr' ? '自动配对已就绪' : mode === 'pair' ? '配对设备' : '连接并验证' }}</button></div>
      </section>
    </div>
  </Teleport>
</template>
