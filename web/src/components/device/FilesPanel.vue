<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Download, File, Folder, FolderUp, RefreshCw, Upload } from 'lucide-vue-next'
import { api, toAppError } from '@/services/api'
import { useUiStore } from '@/stores/ui'

const props = defineProps<{ deviceId: string }>()
const ui = useUiStore()
const path = ref('/sdcard')
const listing = ref<string[]>([])
const loading = ref(false)
const fileInput = ref<HTMLInputElement>()

async function load() {
  loading.value = true
  try { listing.value = (await api<{ listing: string[] }>(`/devices/${encodeURIComponent(props.deviceId)}/files?path=${encodeURIComponent(path.value)}`)).listing }
  catch (error) { ui.failure(toAppError(error)) }
  finally { loading.value = false }
}

async function upload(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (!file) return
  const form = new FormData(); form.append('file', file)
  try { await api(`/devices/${encodeURIComponent(props.deviceId)}/files/upload?path=${encodeURIComponent(path.value)}`, { method: 'POST', body: form }); ui.notify('上传成功', file.name, 'success'); await load() }
  catch (error) { ui.failure(toAppError(error)) }
}

function download(line: string) {
  const name = line.trim().split(/\s+/).at(-1)
  if (!name || line.startsWith('d')) return
  location.href = `/api/v1/devices/${encodeURIComponent(props.deviceId)}/files/download?path=${encodeURIComponent(`${path.value}/${name}`)}`
}

onMounted(load)
</script>

<template>
  <div class="card overflow-hidden"><div class="flex flex-col gap-3 border-b border-slate-100 p-4 dark:border-white/7 lg:flex-row lg:items-center"><div><h3 class="section-title m-0">文件管理</h3><p class="mt-1 text-xs text-slate-400">访问设备共享存储空间</p></div><div class="ml-auto flex min-w-0 flex-1 gap-2 lg:max-w-2xl"><button class="icon-button shrink-0" title="上级目录" @click="path = path.split('/').slice(0,-1).join('/') || '/'; load()"><FolderUp :size="17" /></button><input v-model="path" class="field min-w-0 font-mono text-xs" @keyup.enter="load" /><button class="icon-button shrink-0" title="刷新" @click="load"><RefreshCw :size="16" :class="loading ? 'animate-spin' : ''" /></button><button class="btn-primary shrink-0" @click="fileInput?.click()"><Upload :size="15" />上传</button><input ref="fileInput" type="file" class="hidden" @change="upload" /></div></div>
    <div class="divide-y divide-slate-100 dark:divide-white/7"><div v-for="(line, index) in listing" :key="index" class="flex items-center gap-3 px-4 py-2.5 font-mono text-[11px]"><component :is="line.startsWith('d') ? Folder : File" :size="16" :class="line.startsWith('d') ? 'text-amber-500' : 'text-slate-400'" /><span class="min-w-0 flex-1 truncate">{{ line }}</span><button v-if="!line.startsWith('d')" class="icon-button" title="下载" @click="download(line)"><Download :size="15" /></button></div><div v-if="!listing.length && !loading" class="p-10 text-center text-xs text-slate-400">目录为空</div></div>
  </div>
</template>
