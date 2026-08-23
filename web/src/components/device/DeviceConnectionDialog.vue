<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Wifi, X } from 'lucide-vue-next'
import {
  discoverWirelessDevices,
  normalizePairingCode,
  pairWirelessDevice,
  type DiscoveredWirelessService,
} from '@/services/deviceConnection'
import { toAppError } from '@/services/api'
import { useDevicesStore } from '@/stores/devices'
import { useUiStore } from '@/stores/ui'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: [] }>()

type ConnectionMode = 'connect' | 'pair'
type ConnectionPhase = 'idle' | 'discovering' | 'pairing' | 'connecting'

const devices = useDevicesStore()
const ui = useUiStore()
const endpoint = ref('')
const pairingCode = ref('')
const mode = ref<ConnectionMode>('connect')
const phase = ref<ConnectionPhase>('idle')
const discovered = ref<DiscoveredWirelessService[]>([])
const busy = computed(() => phase.value !== 'idle')
const visibleServices = computed(() => discovered.value.filter(item => item.type === (mode.value === 'pair' ? 'pairing' : 'connect')))

watch(() => props.open, (open) => {
  if (!open) return
  endpoint.value = ''
  pairingCode.value = ''
  mode.value = 'connect'
  void discover()
})

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

async function submit() {
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
        <div class="mt-4 flex rounded-lg bg-slate-100 p-1 dark:bg-white/5" role="tablist" aria-label="连接方式"><button class="h-8 flex-1 rounded-md text-xs font-medium" role="tab" :aria-selected="mode === 'connect'" :class="mode === 'connect' ? 'bg-white text-brand-700 shadow-sm dark:bg-white/10 dark:text-brand-300' : 'text-slate-500'" @click="mode='connect'; endpoint=''">直接连接</button><button class="h-8 flex-1 rounded-md text-xs font-medium" role="tab" :aria-selected="mode === 'pair'" :class="mode === 'pair' ? 'bg-white text-brand-700 shadow-sm dark:bg-white/10 dark:text-brand-300' : 'text-slate-500'" @click="mode='pair'; endpoint=''">六位码配对</button></div>
        <div v-if="visibleServices.length" class="mt-3 max-h-28 overflow-auto rounded-lg border border-slate-200 p-1 dark:border-white/9"><button v-for="service in visibleServices" :key="service.endpoint" class="flex w-full items-center rounded-md px-2 py-1.5 text-left font-mono text-[10px] transition hover:bg-slate-50 dark:hover:bg-white/5" @click="endpoint=service.endpoint"><Wifi :size="13" class="mr-2 text-brand-600" />{{ service.endpoint }}</button></div>
        <div v-else-if="phase === 'discovering'" class="mt-3 rounded-lg border border-slate-200 px-3 py-2 text-xs text-slate-400 dark:border-white/9">正在发现局域网设备…</div>
        <label class="mt-4 block text-xs font-medium">{{ mode === 'pair' ? '配对地址' : '连接地址' }}</label>
        <input v-model="endpoint" class="field mt-2 font-mono" :placeholder="mode === 'pair' ? '192.168.1.20:37123' : '192.168.1.20:5555'" autocomplete="off" @keyup.enter="submit" />
        <template v-if="mode === 'pair'"><label class="mt-4 block text-xs font-medium">六位配对码</label><input :value="pairingCode" inputmode="numeric" maxlength="6" class="field mt-2 font-mono tracking-[.3em]" placeholder="123456" autocomplete="one-time-code" @input="updatePairingCode" @keyup.enter="submit" /><p class="mt-2 text-[11px] leading-5 text-slate-400">配对码会很快过期，且仅发送给当前服务端，不会写入日志或配置。</p></template>
        <div class="mt-6 flex justify-end gap-2"><button class="btn-secondary" @click="emit('close')">取消</button><button class="btn-primary" :disabled="busy || !endpoint.trim() || (mode === 'pair' && pairingCode.length !== 6)" @click="submit">{{ phase === 'pairing' ? '正在配对…' : phase === 'connecting' ? '正在连接…' : mode === 'pair' ? '配对设备' : '连接并验证' }}</button></div>
      </section>
    </div>
  </Teleport>
</template>
