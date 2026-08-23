<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { KeyRound, Lock, LockOpen, RefreshCw } from 'lucide-vue-next'
import { api, toAppError } from '@/services/api'
import type { AppError } from '@/types/api'
import { useUiStore } from '@/stores/ui'

interface LockState { locked: boolean; awake: boolean; known: boolean }
const props = defineProps<{ deviceId: string }>()
const ui = useUiStore()
const state = ref<LockState | null>(null)
const stateError = ref<AppError | null>(null)
const expanded = ref(false)
const pin = ref('')
const busy = ref(false)
let timer = 0

async function refresh() {
  try {
    state.value = await api(`/devices/${encodeURIComponent(props.deviceId)}/lock`)
    stateError.value = null
  } catch (error) {
    state.value = null
    stateError.value = toAppError(error)
  }
}

async function unlock() {
  busy.value = true
  try { await api(`/devices/${encodeURIComponent(props.deviceId)}/unlock`, { method:'POST', body:JSON.stringify({ pin:pin.value }) }); pin.value=''; expanded.value=false; ui.notify('解锁手势已发送','正在重新确认设备锁屏状态。','success'); window.setTimeout(refresh,700) }
  catch (error) { ui.failure(toAppError(error)) }
  finally { busy.value=false }
}

onMounted(() => { void refresh(); timer=window.setInterval(refresh,5000) })
onBeforeUnmount(() => window.clearInterval(timer))
</script>

<template>
  <div class="relative"><button class="btn-secondary" @click="expanded=!expanded"><component :is="state?.locked ? Lock : LockOpen" :size="15" /><span v-if="stateError" class="text-red-600 dark:text-red-300">状态不可用</span><span v-else-if="state?.known">{{ state.locked ? '设备已锁定' : '设备已解锁' }}</span><span v-else>锁屏状态</span></button><div v-if="expanded" class="surface absolute top-11 right-0 z-30 w-72 rounded-xl p-4 shadow-xl"><div class="flex items-center"><div><div class="text-xs font-semibold">设备解锁</div><p class="mt-1 text-[10px] text-slate-400">唤醒并执行适配屏幕尺寸的上滑手势</p></div><button class="icon-button ml-auto" title="刷新状态" @click="refresh"><RefreshCw :size="14" /></button></div><label class="mt-4 block text-[10px] font-medium text-slate-500">数字 PIN（无 PIN 可留空）</label><div class="relative mt-2"><KeyRound :size="14" class="absolute top-3 left-3 text-slate-400" /><input v-model="pin" type="password" inputmode="numeric" maxlength="16" class="field pl-8" placeholder="4–16 位数字" @keyup.enter="unlock" /></div><button class="btn-primary mt-3 w-full" :disabled="busy" @click="unlock">{{ busy ? '正在执行…' : '唤醒并解锁' }}</button></div></div>
</template>
