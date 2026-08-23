<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { AppWindow, ArrowLeft, Boxes, Cpu, Files, LayoutDashboard, MonitorSmartphone, RefreshCw, TerminalSquare } from 'lucide-vue-next'
import AppsPanel from '@/components/device/AppsPanel.vue'
import CompanionPanel from '@/components/device/CompanionPanel.vue'
import ConnectionControl from '@/components/device/ConnectionControl.vue'
import FilesPanel from '@/components/device/FilesPanel.vue'
import HardwarePanel from '@/components/device/HardwarePanel.vue'
import LockControl from '@/components/device/LockControl.vue'
import OverviewPanel from '@/components/device/OverviewPanel.vue'
import ScreenPanel from '@/components/device/ScreenPanel.vue'
import TerminalPanel from '@/components/device/TerminalPanel.vue'
import StateMessage from '@/components/feedback/StateMessage.vue'
import { api, toAppError } from '@/services/api'
import type { AppError, DeviceOverview } from '@/types/api'

const route = useRoute()
const router = useRouter()
const deviceId = computed(() => String(route.params.id))
const activeTab = ref(String(route.query.tab ?? 'overview'))
const overview = ref<DeviceOverview | null>(null)
const error = ref<AppError | null>(null)
const loading = ref(false)
const tabs = [
  { id: 'overview', label: '概览', icon: LayoutDashboard },
  { id: 'screen', label: '实时控制', icon: MonitorSmartphone },
  { id: 'hardware', label: '硬件信息', icon: Cpu },
  { id: 'apps', label: '应用管理', icon: AppWindow },
  { id: 'files', label: '文件管理', icon: Files },
  { id: 'terminal', label: 'ADB 终端', icon: TerminalSquare },
  { id: 'companion', label: '伴侣能力', icon: Boxes },
]

async function load() {
  loading.value = true; error.value = null
  try { overview.value = await api(`/devices/${encodeURIComponent(deviceId.value)}/overview`) }
  catch (value) { error.value = toAppError(value) }
  finally { loading.value = false }
}

function selectTab(tab: string) { activeTab.value = tab; router.replace({ query: { ...route.query, tab } }) }
watch(() => route.query.tab, value => { if (value) activeTab.value = String(value) })
onMounted(load)
</script>

<template>
  <div class="mb-5 flex flex-wrap items-center gap-3"><RouterLink to="/devices" class="icon-button" aria-label="返回设备中心"><ArrowLeft :size="18" /></RouterLink><div class="min-w-0"><div class="eyebrow">Device Console</div><h1 class="mt-1 mb-0 truncate text-xl font-bold tracking-tight">{{ overview?.properties['ro.product.model'] || deviceId }}</h1><p class="mt-1 mb-0 truncate font-mono text-[10px] text-slate-400">{{ deviceId }}</p></div><div class="ml-auto flex items-center gap-2"><LockControl :device-id="deviceId" /><ConnectionControl :device-id="deviceId" /><span class="flex items-center gap-1.5 rounded-md bg-brand-50 px-2 py-1 text-[11px] font-semibold text-brand-700 dark:bg-brand-500/10 dark:text-brand-300"><span class="size-1.5 rounded-full bg-brand-500" />设备在线</span><button class="icon-button" title="刷新" @click="load"><RefreshCw :size="16" :class="loading ? 'animate-spin' : ''" /></button></div></div>
  <div class="mb-5 overflow-x-auto border-b border-slate-200 dark:border-white/9"><div class="flex min-w-max gap-1"><button v-for="tab in tabs" :key="tab.id" class="relative flex h-11 items-center gap-2 px-3 text-xs font-medium text-slate-500 transition hover:text-slate-900 dark:text-slate-400 dark:hover:text-white" :class="activeTab === tab.id ? '!text-brand-700 dark:!text-brand-400' : ''" @click="selectTab(tab.id)"><component :is="tab.icon" :size="15" />{{ tab.label }}<span v-if="activeTab === tab.id" class="absolute inset-x-2 bottom-0 h-0.5 bg-brand-500" /></button></div></div>
  <StateMessage v-if="loading && !overview" state="loading" title="正在读取设备信息" />
  <StateMessage v-else-if="error" state="error" :error="error" @retry="load" />
  <template v-else-if="overview">
    <OverviewPanel v-if="activeTab === 'overview'" :overview="overview" />
    <HardwarePanel v-if="activeTab === 'hardware'" :device-id="deviceId" />
    <ScreenPanel v-if="activeTab === 'screen'" :device-id="deviceId" active />
    <AppsPanel v-if="activeTab === 'apps'" :device-id="deviceId" />
    <FilesPanel v-if="activeTab === 'files'" :device-id="deviceId" />
    <TerminalPanel v-if="activeTab === 'terminal'" :device-id="deviceId" />
    <CompanionPanel v-if="activeTab === 'companion'" :device-id="deviceId" />
  </template>
</template>
