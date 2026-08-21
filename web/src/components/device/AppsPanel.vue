<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Box, ExternalLink, Search, Trash2, Upload, XCircle } from 'lucide-vue-next'
import ConfirmDialog from '@/components/feedback/ConfirmDialog.vue'
import StateMessage from '@/components/feedback/StateMessage.vue'
import { api, toAppError } from '@/services/api'
import { useUiStore } from '@/stores/ui'

const props = defineProps<{ deviceId: string }>()
const ui = useUiStore()
const packages = ref<string[]>([])
const loading = ref(true)
const query = ref('')
const pending = ref<{ operation: string; package: string } | null>(null)
const fileInput = ref<HTMLInputElement>()
const filtered = computed(() => packages.value.filter(name => name.includes(query.value.trim().toLowerCase())))

async function load() {
  loading.value = true
  try { packages.value = await api(`/devices/${encodeURIComponent(props.deviceId)}/packages`) }
  catch (error) { ui.failure(toAppError(error)) }
  finally { loading.value = false }
}

async function action(operation: string, packageName: string) {
  if (['clear', 'uninstall'].includes(operation)) { pending.value = { operation, package: packageName }; return }
  await execute(operation, packageName)
}

async function execute(operation: string, packageName: string) {
  try {
    await api(`/devices/${encodeURIComponent(props.deviceId)}/packages/action`, { method: 'POST', body: JSON.stringify({ operation, package: packageName }) })
    ui.notify('操作已完成', `${packageName} · ${operation}`, 'success')
    if (operation === 'uninstall') await load()
  } catch (error) { ui.failure(toAppError(error)) }
  finally { pending.value = null }
}

async function install(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (!file) return
  const form = new FormData(); form.append('file', file)
  try {
    await api(`/devices/${encodeURIComponent(props.deviceId)}/packages/install`, { method: 'POST', body: form })
    ui.notify('安装成功', file.name, 'success'); await load()
  } catch (error) { ui.failure(toAppError(error)) }
  if (fileInput.value) fileInput.value.value = ''
}

onMounted(load)
</script>

<template>
  <div class="card overflow-hidden">
    <div class="flex flex-col gap-3 border-b border-slate-100 p-4 dark:border-white/7 sm:flex-row sm:items-center"><div><h3 class="section-title m-0">应用管理</h3><p class="mt-1 text-xs text-slate-400">已安装的第三方应用</p></div><div class="ml-auto flex gap-2"><div class="flex items-center rounded-lg border border-slate-200 px-2.5 dark:border-white/10"><Search :size="15" class="text-slate-400" /><input v-model="query" class="h-9 w-40 border-0 bg-transparent px-2 text-xs outline-none" placeholder="搜索包名" /></div><button class="btn-primary" @click="fileInput?.click()"><Upload :size="15" />安装 APK</button><input ref="fileInput" type="file" accept=".apk,application/vnd.android.package-archive" class="hidden" @change="install" /></div></div>
    <StateMessage v-if="loading" state="loading" title="正在读取应用目录" class="!border-0 !shadow-none" />
    <div v-else-if="filtered.length" class="divide-y divide-slate-100 dark:divide-white/7"><div v-for="name in filtered" :key="name" class="flex flex-wrap items-center gap-3 px-4 py-3"><div class="grid size-9 place-items-center rounded-lg bg-slate-100 text-slate-500 dark:bg-white/6"><Box :size="17" /></div><div class="min-w-0 flex-1 truncate font-mono text-xs">{{ name }}</div><div class="flex gap-1"><button class="icon-button" title="启动" @click="action('open', name)"><ExternalLink :size="15" /></button><button class="icon-button" title="强制停止" @click="action('stop', name)"><XCircle :size="15" /></button><button class="icon-button hover:!text-red-600" title="清除数据" @click="action('clear', name)"><Trash2 :size="15" /></button><button class="btn-danger !h-8 !px-2.5 !text-xs" @click="action('uninstall', name)">卸载</button></div></div></div>
    <StateMessage v-else state="empty" title="没有匹配的应用" class="!border-0 !shadow-none" />
  </div>
  <ConfirmDialog :open="!!pending" destructive :title="pending?.operation === 'uninstall' ? '确认卸载应用' : '确认清除应用数据'" :description="pending?.operation === 'uninstall' ? `${pending?.package} 及其数据将被移除。` : `${pending?.package} 的登录状态、设置和缓存将永久清除。`" confirm-text="确认执行" @cancel="pending = null" @confirm="pending && execute(pending.operation, pending.package)" />
</template>
