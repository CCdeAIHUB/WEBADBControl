<script setup lang="ts">
import { ref } from 'vue'
import { Cable, LogOut, Network, X } from 'lucide-vue-next'
import { useRouter } from 'vue-router'
import ConfirmDialog from '@/components/feedback/ConfirmDialog.vue'
import { api, toAppError } from '@/services/api'
import { useUiStore } from '@/stores/ui'

const props = defineProps<{ deviceId: string }>()
const router = useRouter()
const ui = useUiStore()
const open = ref(false)
const disconnectConfirm = ref(false)
const port = ref(5555)
const busy = ref(false)

async function enableTCPIP() {
  busy.value = true
  try { await api(`/devices/${encodeURIComponent(props.deviceId)}/tcpip`, { method:'POST', body:JSON.stringify({ port:port.value }) }); ui.notify('无线调试已启用', `设备已切换到 TCP/IP ${port.value} 端口。`, 'success') }
  catch (error) { ui.failure(toAppError(error)) }
  finally { busy.value=false }
}

async function disconnect() {
  try { await api(`/devices/${encodeURIComponent(props.deviceId)}/disconnect`, { method:'POST', body:'{}' }); ui.notify('设备已断开', props.deviceId, 'success'); await router.replace('/devices') }
  catch (error) { ui.failure(toAppError(error)) }
}
</script>

<template>
  <div class="relative"><button class="icon-button" title="连接管理" @click="open=!open"><Network :size="16" /></button><div v-if="open" class="surface absolute top-11 right-0 z-30 w-72 rounded-xl p-4 shadow-xl"><div class="flex items-start"><div><div class="text-xs font-semibold">连接管理</div><p class="mt-1 text-[10px] text-slate-400">切换无线 ADB 或断开当前设备</p></div><button class="icon-button ml-auto -mt-1" @click="open=false"><X :size="14" /></button></div><label class="mt-4 block text-[10px] font-medium text-slate-500">TCP/IP 端口</label><div class="mt-2 flex gap-2"><input v-model.number="port" type="number" min="1024" max="65535" class="field" /><button class="btn-secondary shrink-0" :disabled="busy" @click="enableTCPIP"><Cable :size="14" />启用</button></div><button class="btn-danger mt-3 w-full" @click="disconnectConfirm=true"><LogOut :size="14" />断开此设备</button></div></div>
  <ConfirmDialog :open="disconnectConfirm" title="断开设备连接" description="当前投屏、文件传输和任务操作将立即中止。设备稍后仍可重新连接。" confirm-text="确认断开" @cancel="disconnectConfirm=false" @confirm="disconnect" />
</template>
