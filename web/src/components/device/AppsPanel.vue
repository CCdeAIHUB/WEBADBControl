<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ExternalLink, PackageOpen, Search, Trash2, Upload, XCircle } from 'lucide-vue-next'
import ConfirmDialog from '@/components/feedback/ConfirmDialog.vue'
import StateMessage from '@/components/feedback/StateMessage.vue'
import { api, toAppError } from '@/services/api'
import { useUiStore } from '@/stores/ui'
import type { PackageInfo } from '@/types/api'

const props = defineProps<{ deviceId: string }>()
const ui = useUiStore()
const packages = ref<PackageInfo[]>([])
const loading = ref(true)
const query = ref('')
const pending = ref<{ operation: string; package: string } | null>(null)
const fileInput = ref<HTMLInputElement>()

const filtered = computed(() => {
  const keyword = query.value.trim().toLowerCase()
  if (!keyword) return packages.value
  return packages.value.filter(app => app.package.toLowerCase().includes(keyword) || app.displayName.toLowerCase().includes(keyword))
})

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

function iconClass(color: string) {
  return ({
    emerald: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/12 dark:text-emerald-300',
    sky: 'bg-sky-50 text-sky-700 dark:bg-sky-500/12 dark:text-sky-300',
    violet: 'bg-violet-50 text-violet-700 dark:bg-violet-500/12 dark:text-violet-300',
    amber: 'bg-amber-50 text-amber-700 dark:bg-amber-500/12 dark:text-amber-300',
    rose: 'bg-rose-50 text-rose-700 dark:bg-rose-500/12 dark:text-rose-300',
    cyan: 'bg-cyan-50 text-cyan-700 dark:bg-cyan-500/12 dark:text-cyan-300',
    lime: 'bg-lime-50 text-lime-700 dark:bg-lime-500/12 dark:text-lime-300',
    indigo: 'bg-indigo-50 text-indigo-700 dark:bg-indigo-500/12 dark:text-indigo-300',
  } as Record<string, string>)[color] ?? 'bg-slate-100 text-slate-700 dark:bg-white/8 dark:text-slate-200'
}

onMounted(load)
</script>

<template>
  <div class="card overflow-hidden">
    <div class="flex flex-col gap-3 border-b border-slate-100 p-4 dark:border-white/7 sm:flex-row sm:items-center">
      <div><h3 class="section-title m-0">应用管理</h3><p class="mt-1 text-xs text-slate-500 dark:text-slate-300">第三方应用、APK 安装与应用控制</p></div>
      <div class="ml-auto flex gap-2">
        <div class="flex items-center rounded-lg border border-slate-200 bg-white px-2.5 dark:border-white/10 dark:bg-white/5"><Search :size="15" class="text-slate-500 dark:text-slate-300" /><input v-model="query" class="h-9 w-48 border-0 bg-transparent px-2 text-xs text-slate-800 outline-none placeholder:text-slate-500 dark:text-slate-100 dark:placeholder:text-slate-400" placeholder="搜索名称或包名" /></div>
        <button class="btn-primary" @click="fileInput?.click()"><Upload :size="15" />安装 APK</button>
        <input ref="fileInput" type="file" accept=".apk,application/vnd.android.package-archive" class="hidden" @change="install" />
      </div>
    </div>
    <StateMessage v-if="loading" state="loading" title="正在读取应用目录" class="!border-0 !shadow-none" />
    <div v-else-if="filtered.length" class="divide-y divide-slate-100 dark:divide-white/7">
      <div v-for="app in filtered" :key="app.package" class="flex flex-wrap items-center gap-3 px-4 py-3">
        <div class="grid size-11 place-items-center rounded-xl text-xs font-bold shadow-sm" :class="iconClass(app.iconColor)">{{ app.iconText }}</div>
        <div class="min-w-0 flex-1">
          <div class="truncate text-sm font-semibold text-slate-900 dark:text-white">{{ app.displayName }}</div>
          <div class="mt-0.5 truncate font-mono text-[11px] text-slate-500 dark:text-slate-300">{{ app.package }}</div>
          <div class="mt-1 flex flex-wrap gap-2 text-[10px] text-slate-500 dark:text-slate-400">
            <span v-if="app.versionCode">versionCode {{ app.versionCode }}</span>
            <span v-if="app.apkPath" class="max-w-xl truncate font-mono">{{ app.apkPath }}</span>
          </div>
        </div>
        <div class="flex gap-1">
          <button class="icon-button" title="启动" @click="action('open', app.package)"><ExternalLink :size="15" /></button>
          <button class="icon-button" title="强制停止" @click="action('stop', app.package)"><XCircle :size="15" /></button>
          <button class="icon-button hover:!text-red-600" title="清除数据" @click="action('clear', app.package)"><Trash2 :size="15" /></button>
          <button class="btn-danger !h-8 !px-2.5 !text-xs" @click="action('uninstall', app.package)">卸载</button>
        </div>
      </div>
    </div>
    <StateMessage v-else state="empty" title="没有匹配的应用" class="!border-0 !shadow-none"><template #default><PackageOpen /></template></StateMessage>
  </div>
  <ConfirmDialog :open="!!pending" destructive :title="pending?.operation === 'uninstall' ? '确认卸载应用' : '确认清除应用数据'" :description="pending?.operation === 'uninstall' ? `${pending?.package} 及其数据将被移除。` : `${pending?.package} 的登录状态、设置和缓存将永久清除。`" confirm-text="确认执行" @cancel="pending = null" @confirm="pending && execute(pending.operation, pending.package)" />
</template>
