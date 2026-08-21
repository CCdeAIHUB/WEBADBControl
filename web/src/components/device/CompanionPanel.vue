<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { CheckCircle2, Download, KeyRound, Play, RefreshCw, ShieldAlert } from 'lucide-vue-next'
import { api, toAppError } from '@/services/api'
import { useUiStore } from '@/stores/ui'

const props = defineProps<{ deviceId: string }>()
const ui = useUiStore()
const capabilities = ref<Array<Record<string, any>>>([])
const permissions = ref<Array<Record<string, any>>>([])
const loading = ref(false)
const selected = ref<Record<string, any> | null>(null)
const operation = ref('')
const argumentsJson = ref('{}')

async function load() {
  loading.value = true
  try {
    ;[capabilities.value, permissions.value] = await Promise.all([
      api<Array<Record<string, any>>>(`/devices/${encodeURIComponent(props.deviceId)}/capabilities`),
      api<Array<Record<string, any>>>(`/devices/${encodeURIComponent(props.deviceId)}/permissions`),
    ])
  } catch (error) { ui.failure(toAppError(error)) }
  finally { loading.value = false }
}

function permissionFor(id: string) { return permissions.value.find(item => item.capabilityId === id) }

async function install() {
  try { await api(`/devices/${encodeURIComponent(props.deviceId)}/companion/install`, { method: 'POST', body: '{}' }); ui.notify('伴侣应用已安装', '请在设备上按提示完成权限与配对。', 'success') }
  catch (error) { ui.failure(toAppError(error)) }
}

async function invoke() {
  if (!selected.value || !operation.value) return
  try {
    const args = JSON.parse(argumentsJson.value) as Record<string, unknown>
    await api(`/devices/${encodeURIComponent(props.deviceId)}/capabilities/invoke`, { method: 'POST', body: JSON.stringify({ capabilityId: selected.value.id, operation: operation.value, args }) })
    ui.notify('能力请求已发送', `${selected.value.id} · ${operation.value}`, 'success')
  } catch (error) { ui.failure(toAppError(error)) }
}

function choose(capability: Record<string, any>) {
  selected.value = capability
  operation.value = capability.operations?.[0] ?? ''
  argumentsJson.value = '{}'
}

onMounted(load)
</script>

<template>
  <div class="grid gap-4 xl:grid-cols-[1.2fr_.8fr]">
    <section class="card overflow-hidden"><div class="flex items-center border-b border-slate-100 p-4 dark:border-white/7"><div><h3 class="section-title m-0">Android Companion 能力</h3><p class="mt-1 text-xs text-slate-400">QUIC v1 权限能力目录</p></div><button class="icon-button ml-auto" title="刷新" @click="load"><RefreshCw :size="16" :class="loading ? 'animate-spin' : ''" /></button><button class="btn-primary ml-2" @click="install"><Download :size="15" />安装伴侣应用</button></div><div class="divide-y divide-slate-100 dark:divide-white/7"><button v-for="capability in capabilities" :key="capability.id" class="flex w-full items-center gap-3 px-4 py-3 text-left hover:bg-slate-50 dark:hover:bg-white/3" @click="choose(capability)"><div class="grid size-9 place-items-center rounded-lg" :class="permissionFor(capability.id)?.granted ? 'bg-brand-50 text-brand-600 dark:bg-brand-500/10' : 'bg-amber-50 text-amber-600 dark:bg-amber-500/10'"><CheckCircle2 v-if="permissionFor(capability.id)?.granted" :size="17" /><ShieldAlert v-else :size="17" /></div><div class="min-w-0 flex-1"><div class="truncate text-sm font-semibold">{{ capability.displayName || capability.id }}</div><div class="mt-0.5 truncate font-mono text-[10px] text-slate-400">{{ capability.id }}</div></div><span class="text-[10px] font-medium" :class="permissionFor(capability.id)?.granted ? 'text-brand-600' : 'text-amber-600'">{{ permissionFor(capability.id)?.granted ? '已授权' : '待授权' }}</span></button><div v-if="!capabilities.length && !loading" class="p-10 text-center text-xs text-slate-400">伴侣会话尚未连接</div></div></section>
    <section class="card p-5"><div class="flex items-center gap-2"><KeyRound :size="17" class="text-brand-600" /><h3 class="section-title m-0">能力调用</h3></div><template v-if="selected"><div class="mt-5 rounded-lg bg-slate-50 p-3 dark:bg-white/4"><div class="text-xs font-semibold">{{ selected.displayName || selected.id }}</div><div class="mt-1 font-mono text-[10px] text-slate-400">{{ selected.id }}</div></div><label class="mt-4 block text-xs font-medium">操作</label><select v-model="operation" class="field mt-2"><option v-for="item in selected.operations" :key="item" :value="item">{{ item }}</option></select><label class="mt-4 block text-xs font-medium">参数 JSON</label><textarea v-model="argumentsJson" class="field mt-2 !h-36 py-3 font-mono text-xs" spellcheck="false" /><button class="btn-primary mt-4 w-full" @click="invoke"><Play :size="15" />发送请求</button></template><div v-else class="grid min-h-72 place-items-center text-center"><div><KeyRound :size="25" class="mx-auto mb-3 text-slate-300" /><p class="m-0 text-xs text-slate-400">从左侧选择一项能力</p></div></div></section>
  </div>
</template>
