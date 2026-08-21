<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Cable, Plus, RefreshCw, Search, Smartphone, Wifi, X } from 'lucide-vue-next'
import PageHeader from '@/components/common/PageHeader.vue'
import StateMessage from '@/components/feedback/StateMessage.vue'
import { useDevicesStore } from '@/stores/devices'
import { api, toAppError } from '@/services/api'
import { useUiStore } from '@/stores/ui'

const devices = useDevicesStore()
const ui = useUiStore()
const query = ref('')
const connectOpen = ref(false)
const endpoint = ref('')
const pairingCode = ref('')
const mode = ref<'connect'|'pair'>('connect')
const discovered = ref<Array<{name:string;type:string;endpoint:string}>>([])
const connecting = ref(false)

onMounted(() => devices.refresh())

async function discover() {
  try { discovered.value = await api('/devices/discover') }
  catch (error) { ui.failure(toAppError(error)) }
}

async function connect() {
  connecting.value = true
  try {
    if (mode.value === 'pair') {
      await api('/devices/pair', { method:'POST', body:JSON.stringify({ endpoint:endpoint.value.trim(), code:pairingCode.value }) })
      ui.notify('配对成功', '请切换到“直接连接”并选择设备的连接端口。', 'success')
      mode.value = 'connect'; endpoint.value = ''; pairingCode.value = ''; await discover()
      return
    }
    await devices.connect(endpoint.value.trim())
    connectOpen.value = false
    endpoint.value = ''
    ui.notify('连接成功', '无线设备已通过 ADB 验证在线。', 'success')
  } catch (error) { ui.failure(toAppError(error)) }
  finally { connecting.value = false }
}
</script>

<template>
  <PageHeader eyebrow="Devices" title="设备中心" description="统一管理 USB、无线 ADB 与 Android Companion 设备。">
    <template #actions><button class="btn-secondary" @click="devices.refresh()"><RefreshCw :size="15" />刷新</button><button class="btn-primary" @click="connectOpen = true; discover()"><Plus :size="16" />连接设备</button></template>
  </PageHeader>
  <div class="mb-4 flex max-w-md items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 dark:border-white/9 dark:bg-white/4"><Search :size="16" class="text-slate-400" /><input v-model="query" class="h-10 min-w-0 flex-1 border-0 bg-transparent text-sm outline-none" placeholder="搜索名称、型号或设备序列号" /></div>
  <StateMessage v-if="devices.status === 'loading'" state="loading" title="正在发现设备" />
  <StateMessage v-else-if="devices.status === 'error'" state="error" :error="devices.error" @retry="devices.refresh()" />
  <StateMessage v-else-if="!devices.devices.length" state="empty" title="还没有连接设备" description="使用 USB 调试连接设备，或输入无线 ADB 地址建立连接。" />
  <section v-else class="grid gap-4 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
    <RouterLink v-for="device in devices.devices.filter(d => `${d.name} ${d.model} ${d.id}`.toLowerCase().includes(query.toLowerCase()))" :key="device.id" :to="`/devices/${encodeURIComponent(device.id)}`" class="card group overflow-hidden p-5 transition hover:-translate-y-0.5 hover:border-brand-200 hover:shadow-md dark:hover:border-brand-500/25">
      <div class="flex items-start"><div class="grid size-11 place-items-center rounded-xl bg-slate-100 text-slate-600 group-hover:bg-brand-50 group-hover:text-brand-700 dark:bg-white/6 dark:text-slate-300 dark:group-hover:bg-brand-500/10 dark:group-hover:text-brand-300"><Smartphone :size="21" /></div><div class="ml-auto flex items-center gap-1.5 text-[11px] font-medium" :class="['device','online'].includes(device.state) ? 'text-brand-600' : 'text-slate-400'"><span class="size-1.5 rounded-full bg-current" />{{ ['device','online'].includes(device.state) ? '在线' : device.state }}</div></div>
      <h2 class="mt-5 mb-1 truncate text-[15px] font-semibold text-slate-900 dark:text-white">{{ device.name }}</h2><p class="m-0 truncate font-mono text-[10px] text-slate-400">{{ device.id }}</p>
      <div class="mt-4 flex items-center justify-between border-t border-slate-100 pt-3 text-xs text-slate-500 dark:border-white/7 dark:text-slate-400"><span class="flex items-center gap-1.5"><Wifi v-if="device.transport === 'wireless'" :size="14" /><Cable v-else :size="14" />{{ device.transport === 'wireless' ? '无线 ADB' : device.transport === 'companion' ? '伴侣应用' : 'USB 调试' }}</span><span>{{ device.model || device.product || 'Android' }}</span></div>
    </RouterLink>
  </section>

  <Teleport to="body"><div v-if="connectOpen" class="fixed inset-0 z-[80] grid place-items-center bg-black/35 p-4 backdrop-blur-[2px]" @click.self="connectOpen = false"><section class="surface w-full max-w-lg rounded-xl p-5 shadow-2xl"><div class="flex items-center"><div><div class="eyebrow">Wireless ADB</div><h2 class="mt-1 text-lg font-semibold">连接无线设备</h2></div><button class="icon-button ml-auto" @click="connectOpen = false"><X :size="18" /></button></div><p class="mt-3 text-sm leading-6 text-slate-500 dark:text-slate-400">确保设备与服务器处于同一网络，并已在开发者选项中启用无线调试。</p><div class="mt-4 flex rounded-lg bg-slate-100 p-1 dark:bg-white/5"><button class="h-8 flex-1 rounded-md text-xs font-medium" :class="mode === 'connect' ? 'bg-white text-brand-700 shadow-sm dark:bg-white/10 dark:text-brand-300' : 'text-slate-500'" @click="mode='connect'">直接连接</button><button class="h-8 flex-1 rounded-md text-xs font-medium" :class="mode === 'pair' ? 'bg-white text-brand-700 shadow-sm dark:bg-white/10 dark:text-brand-300' : 'text-slate-500'" @click="mode='pair'">六位码配对</button></div><div v-if="discovered.length" class="mt-3 max-h-28 overflow-auto rounded-lg border border-slate-200 p-1 dark:border-white/9"><button v-for="service in discovered.filter(item => mode === 'pair' ? item.type === 'pairing' : item.type === 'connect')" :key="service.endpoint" class="flex w-full items-center rounded-md px-2 py-1.5 text-left font-mono text-[10px] hover:bg-slate-50 dark:hover:bg-white/5" @click="endpoint=service.endpoint"><Wifi :size="13" class="mr-2 text-brand-600" />{{ service.endpoint }}</button></div><label class="mt-4 block text-xs font-medium">{{ mode === 'pair' ? '配对地址' : '设备地址' }}</label><input v-model="endpoint" class="field mt-2 font-mono" :placeholder="mode === 'pair' ? '192.168.1.20:37123' : '192.168.1.20:5555'" @keyup.enter="connect" /><template v-if="mode === 'pair'"><label class="mt-4 block text-xs font-medium">六位配对码</label><input v-model="pairingCode" inputmode="numeric" maxlength="6" class="field mt-2 font-mono tracking-[.3em]" placeholder="123456" @keyup.enter="connect" /></template><div class="mt-6 flex justify-end gap-2"><button class="btn-secondary" @click="connectOpen = false">取消</button><button class="btn-primary" :disabled="!endpoint.trim() || (mode === 'pair' && pairingCode.length !== 6) || connecting" @click="connect">{{ connecting ? '正在验证…' : mode === 'pair' ? '配对设备' : '连接并验证' }}</button></div></section></div></Teleport>
</template>
