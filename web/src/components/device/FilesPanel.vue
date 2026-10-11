<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Download, File, Folder, FolderOpen, FolderPlus, FolderUp, HardDrive, RefreshCw, Trash2, Upload, X } from 'lucide-vue-next'
import ConfirmDialog from '@/components/feedback/ConfirmDialog.vue'
import { api, toAppError } from '@/services/api'
import { canOpenEntry, formatFileSize, joinRemotePath, normalizeRemotePath, parentRemotePath, pathSegments, resolveOpenPath, sortFileEntries } from '@/services/deviceFiles'
import { useUiStore } from '@/stores/ui'
import type { DeviceFileEntry } from '@/types/api'

const props = defineProps<{ deviceId: string }>()
const ui = useUiStore()
const currentPath = ref('/sdcard')
const entries = ref<DeviceFileEntry[]>([])
const loading = ref(false)
const creating = ref(false)
const newFolderName = ref('')
const pendingDelete = ref<DeviceFileEntry | null>(null)
const fileInput = ref<HTMLInputElement>()
interface DownloadJob { id: string; fileName: string; state: 'queued'|'running'|'completed'|'failed'; bytesTotal: number; bytesTransferred: number; progress: number; error?: string }
const downloadJob = ref<DownloadJob | null>(null)
let downloadTimer = 0
let downloadAbort: AbortController | undefined

const sortedEntries = computed(() => sortFileEntries(entries.value))
const breadcrumbs = computed(() => pathSegments(currentPath.value))

async function load(path = currentPath.value) {
  loading.value = true
  try {
    const payload = await api<{ path: string; entries: DeviceFileEntry[] }>(`/devices/${encodeURIComponent(props.deviceId)}/files?path=${encodeURIComponent(normalizeRemotePath(path))}`)
    currentPath.value = normalizeRemotePath(payload.path)
    entries.value = payload.entries
  } catch (error) {
    ui.failure(toAppError(error))
  } finally {
    loading.value = false
  }
}

function openEntry(entry: DeviceFileEntry) {
  if (canOpenEntry(entry) && !loading.value) void load(resolveOpenPath(currentPath.value, entry))
}

async function upload(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  const form = new FormData()
  form.append('file', file)
  try {
    await api(`/devices/${encodeURIComponent(props.deviceId)}/files/upload?path=${encodeURIComponent(currentPath.value)}`, { method: 'POST', body: form })
    ui.notify('上传成功', file.name, 'success')
    await load()
  } catch (error) {
    ui.failure(toAppError(error))
  } finally {
    input.value = ''
  }
}

async function createFolder() {
  const name = newFolderName.value.trim()
  if (!name || name.includes('/') || name.includes('\\')) {
    ui.notify('文件夹名称无效', '名称不能包含路径分隔符。', 'warning')
    return
  }
  creating.value = true
  try {
    await api(`/devices/${encodeURIComponent(props.deviceId)}/files/mkdir`, { method: 'POST', body: JSON.stringify({ path: joinRemotePath(currentPath.value, name) }) })
    ui.notify('文件夹已创建', name, 'success')
    newFolderName.value = ''
    await load()
  } catch (error) {
    ui.failure(toAppError(error))
  } finally {
    creating.value = false
  }
}

async function removePending() {
  if (!pendingDelete.value) return
  const target = pendingDelete.value
  pendingDelete.value = null
  try {
    await api(`/devices/${encodeURIComponent(props.deviceId)}/files?path=${encodeURIComponent(target.path)}`, { method: 'DELETE' })
    ui.notify('已删除', target.name, 'success')
    await load()
  } catch (error) {
    ui.failure(toAppError(error))
  }
}

async function download(entry: DeviceFileEntry) {
  if (entry.type === 'directory') return
  cancelDownload(false)
  try {
    downloadJob.value = await api<DownloadJob>(`/devices/${encodeURIComponent(props.deviceId)}/files/downloads`, { method: 'POST', body: JSON.stringify({ path: entry.path }) })
    downloadTimer = window.setInterval(pollDownload, 500)
    await pollDownload()
  } catch (error) { ui.failure(toAppError(error)) }
}

async function pollDownload() {
  const job = downloadJob.value
  if (!job || job.state === 'completed' || job.state === 'failed') return
  try {
    const current = await api<DownloadJob>(`/devices/${encodeURIComponent(props.deviceId)}/files/downloads/${job.id}`)
    downloadJob.value = current
    if (current.state === 'completed') { window.clearInterval(downloadTimer); await saveCompletedDownload(current) }
    else if (current.state === 'failed') { window.clearInterval(downloadTimer); ui.notify('下载失败', current.error || '设备文件传输失败', 'danger') }
  } catch (error) { window.clearInterval(downloadTimer); ui.failure(toAppError(error)) }
}

async function saveCompletedDownload(job: DownloadJob) {
  downloadAbort = new AbortController()
  const response = await fetch(`/api/v1/devices/${encodeURIComponent(props.deviceId)}/files/downloads/${job.id}/content`, { credentials: 'same-origin', signal: downloadAbort.signal })
  if (!response.ok || !response.body) throw new Error(`下载响应失败（${response.status}）`)
  const total = Number(response.headers.get('content-length')) || job.bytesTotal
  const reader = response.body.getReader(); const chunks: Uint8Array[] = []; let received = 0
  while (true) {
    const { done, value } = await reader.read(); if (done) break
    chunks.push(value); received += value.byteLength
    downloadJob.value = { ...job, bytesTotal: total, bytesTransferred: received, progress: total ? received / total * 100 : 100 }
  }
  const url = URL.createObjectURL(new Blob(chunks as BlobPart[])); const link = document.createElement('a')
  link.href = url; link.download = job.fileName; link.click(); URL.revokeObjectURL(url)
  ui.notify('下载完成', job.fileName, 'success')
}

function cancelDownload(removeRemote = true) {
  window.clearInterval(downloadTimer); downloadAbort?.abort(); downloadAbort = undefined
  const id = downloadJob.value?.id; downloadJob.value = null
  if (removeRemote && id) void api(`/devices/${encodeURIComponent(props.deviceId)}/files/downloads/${id}`, { method: 'DELETE' }).catch(() => undefined)
}

onMounted(() => load())
onBeforeUnmount(() => cancelDownload())
</script>

<template>
  <div class="card overflow-hidden">
    <div class="border-b border-slate-100 p-4 dark:border-white/7">
      <div class="flex flex-col gap-3 xl:flex-row xl:items-center">
        <div class="min-w-0">
          <div class="flex items-center gap-2">
            <HardDrive :size="18" class="text-brand-600" />
            <h3 class="section-title m-0">文件管理</h3>
          </div>
          <p class="mt-1 text-xs text-slate-400">共享存储浏览、上传、下载、建目录与删除</p>
        </div>
        <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2 xl:justify-end">
          <button class="btn-secondary" :disabled="loading" @click="load(parentRemotePath(currentPath))"><FolderUp :size="15" />上级</button>
          <button class="btn-secondary" :disabled="loading" @click="load()"><RefreshCw :size="15" :class="loading ? 'animate-spin' : ''" />刷新</button>
          <button class="btn-primary" @click="fileInput?.click()"><Upload :size="15" />上传文件</button>
          <input ref="fileInput" type="file" class="hidden" @change="upload" />
        </div>
      </div>
      <div v-if="downloadJob" class="mt-4 rounded-xl border border-brand-200 bg-brand-50/70 p-3 dark:border-brand-500/20 dark:bg-brand-500/8">
        <div class="flex items-center gap-3 text-xs"><Download :size="15" class="text-brand-600"/><span class="min-w-0 flex-1 truncate font-semibold">{{ downloadJob.fileName }}</span><span class="tabular-nums text-slate-500">{{ formatFileSize(downloadJob.bytesTransferred) }}<template v-if="downloadJob.bytesTotal"> / {{ formatFileSize(downloadJob.bytesTotal) }}</template> · {{ Math.min(100, downloadJob.progress).toFixed(0) }}%</span><button class="icon-button !size-7" title="取消并清理下载" @click="cancelDownload()"><X :size="14"/></button></div>
        <div class="mt-2 h-1.5 overflow-hidden rounded-full bg-slate-200 dark:bg-white/10"><div class="h-full rounded-full bg-brand-600 transition-[width] duration-300" :style="{ width: `${Math.max(2, Math.min(100, downloadJob.progress))}%` }"/></div>
        <p class="mt-2 text-[11px] text-slate-500">设备到服务器使用独立 ADB 传输通道，不会阻塞设备发现与控制。</p>
      </div>
      <div class="mt-4 flex flex-col gap-3 lg:flex-row lg:items-center">
        <div class="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto rounded-lg border border-slate-200 bg-slate-50 px-2 py-1.5 dark:border-white/10 dark:bg-white/4">
          <button v-for="segment in breadcrumbs" :key="segment.path" class="shrink-0 rounded-md px-2 py-1 font-mono text-[11px] text-slate-500 transition hover:bg-white hover:text-brand-700 dark:text-slate-400 dark:hover:bg-white/7 dark:hover:text-brand-300" @click="load(segment.path)">
            {{ segment.label }}
          </button>
        </div>
        <form class="flex min-w-0 gap-2" @submit.prevent="createFolder">
          <input v-model="newFolderName" class="field h-9 min-w-0 lg:w-56" placeholder="新建文件夹名称" />
          <button class="btn-secondary shrink-0" :disabled="creating" type="submit"><FolderPlus :size="15" />新建</button>
        </form>
      </div>
    </div>

    <div class="overflow-x-auto">
      <div class="grid min-w-[760px] grid-cols-[minmax(260px,1fr)_120px_140px_180px_96px] border-b border-slate-100 bg-slate-50 px-4 py-2 text-[11px] font-semibold text-slate-400 dark:border-white/7 dark:bg-white/3">
        <span>名称</span>
        <span>大小</span>
        <span>权限</span>
        <span>修改时间</span>
        <span class="text-right">操作</span>
      </div>
      <div v-for="entry in sortedEntries" :key="entry.path || `${currentPath}/${entry.name}`" class="grid min-w-[760px] grid-cols-[minmax(260px,1fr)_120px_140px_180px_96px] items-center border-b border-slate-100 px-4 py-3 text-left transition hover:bg-slate-50 dark:border-white/7 dark:hover:bg-white/3" :class="entry.type === 'directory' ? 'cursor-pointer' : ''" @click="openEntry(entry)">
        <button class="flex min-w-0 items-center gap-3 rounded-lg text-left focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-brand-500/25" :class="entry.type === 'directory' ? 'cursor-pointer' : 'cursor-default'" @click.stop="openEntry(entry)">
          <span class="grid size-9 shrink-0 place-items-center rounded-lg" :class="entry.type === 'directory' ? 'bg-amber-50 text-amber-600 dark:bg-amber-500/10 dark:text-amber-300' : entry.type === 'link' ? 'bg-sky-50 text-sky-600 dark:bg-sky-500/10 dark:text-sky-300' : 'bg-slate-100 text-slate-500 dark:bg-white/7 dark:text-slate-300'">
            <FolderOpen v-if="entry.type === 'directory'" :size="18" />
            <Folder v-else-if="entry.type === 'link'" :size="18" />
            <File v-else :size="18" />
          </span>
          <span class="min-w-0">
            <span class="block truncate text-sm font-semibold text-slate-800 dark:text-slate-100">{{ entry.name }}</span>
            <span v-if="entry.target" class="mt-0.5 block truncate font-mono text-[10px] text-slate-400">→ {{ entry.target }}</span>
          </span>
        </button>
        <span class="text-xs text-slate-500">{{ entry.type === 'directory' ? '文件夹' : formatFileSize(entry.size) }}</span>
        <span class="font-mono text-[11px] text-slate-400">{{ entry.permissions }}</span>
        <span class="font-mono text-[11px] text-slate-400">{{ entry.modified || '—' }}</span>
        <span class="flex justify-end gap-1">
          <button v-if="entry.type !== 'directory'" class="icon-button" title="下载" @click.stop="download(entry)"><Download :size="15" /></button>
          <button class="icon-button hover:!border-red-200 hover:!bg-red-50 hover:!text-red-600 dark:hover:!border-red-500/20 dark:hover:!bg-red-500/10 dark:hover:!text-red-300" title="删除" @click.stop="pendingDelete = entry"><Trash2 :size="15" /></button>
        </span>
      </div>
      <div v-if="!sortedEntries.length && !loading" class="grid min-h-72 place-items-center p-10 text-center">
        <div>
          <FolderOpen :size="30" class="mx-auto mb-3 text-slate-300" />
          <p class="m-0 text-sm font-medium text-slate-500">当前目录为空</p>
          <p class="mt-1 text-xs text-slate-400">可上传文件或创建新文件夹</p>
        </div>
      </div>
      <div v-if="loading" class="p-4 text-center text-xs text-slate-400">正在读取目录…</div>
    </div>
  </div>

  <ConfirmDialog
    :open="!!pendingDelete"
    title="确认删除"
    :description="`将从设备删除“${pendingDelete?.name ?? ''}”。此操作不会进入电脑回收站。`"
    confirm-text="删除"
    destructive
    @confirm="removePending"
    @cancel="pendingDelete = null"
  />
</template>
