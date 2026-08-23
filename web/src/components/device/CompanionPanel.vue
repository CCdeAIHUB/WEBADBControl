<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { CheckCircle2, Download, KeyRound, Play, RefreshCw, ShieldAlert, Smartphone, Wifi } from 'lucide-vue-next'
import UiSelect from '@/components/common/UiSelect.vue'
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
const companionError = ref('')

const connected = computed(() => capabilities.value.length > 0 || permissions.value.length > 0)

async function load() {
  loading.value = true
  companionError.value = ''
  try {
    ;[capabilities.value, permissions.value] = await Promise.all([
      api<Array<Record<string, any>>>(`/devices/${encodeURIComponent(props.deviceId)}/capabilities`),
      api<Array<Record<string, any>>>(`/devices/${encodeURIComponent(props.deviceId)}/permissions`),
    ])
  } catch (error) {
    const appError = toAppError(error)
    companionError.value = `${appError.message}（${appError.errorCode}）`
  } finally { loading.value = false }
}

function permissionFor(id: string) { return permissions.value.find(item => item.capabilityId === id) }

async function install() {
  try { await api(`/devices/${encodeURIComponent(props.deviceId)}/companion/install`, { method: 'POST', body: '{}' }); ui.notify('伴侣应用已安装', '请在设备上打开 ADBControl Companion 并按提示授权。', 'success') }
  catch (error) { ui.failure(toAppError(error)) }
}

async function openCompanion() {
  try {
    await api(`/devices/${encodeURIComponent(props.deviceId)}/terminal`, { method: 'POST', body: JSON.stringify({ args: ['shell', 'monkey', '-p', 'com.adbcontrol.companion', '1'] }) })
    ui.notify('已尝试打开伴侣应用', '如设备未响应，请在手机上手动打开 ADBControl Companion。', 'success')
  } catch (error) { ui.failure(toAppError(error)) }
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
  <div class="grid gap-4 xl:grid-cols-[.85fr_1.15fr]">
    <section class="space-y-4">
      <div class="card p-5">
        <div class="flex items-center gap-3">
          <div class="grid size-10 place-items-center rounded-xl bg-brand-50 text-brand-700 dark:bg-brand-500/10 dark:text-brand-300"><Smartphone :size="19" /></div>
          <div><h3 class="section-title m-0">ADB Companion 连接流程</h3><p class="mt-1 text-xs text-slate-500 dark:text-slate-300">按 Windows 客户端习惯分步完成安装、打开、授权与能力同步。</p></div>
        </div>
        <ol class="mt-5 space-y-3">
          <li class="flex gap-3 rounded-lg border border-slate-200 bg-slate-50 p-3 dark:border-white/10 dark:bg-white/5"><span class="grid size-6 shrink-0 place-items-center rounded-full bg-brand-600 text-xs font-bold text-white">1</span><div><div class="text-sm font-semibold">安装伴侣应用</div><p class="mt-1 text-xs text-slate-500 dark:text-slate-300">将服务端打包的 Companion APK 安装到当前设备。</p><button class="btn-secondary mt-3" @click="install"><Download :size="15" />安装 / 覆盖安装</button></div></li>
          <li class="flex gap-3 rounded-lg border border-slate-200 bg-slate-50 p-3 dark:border-white/10 dark:bg-white/5"><span class="grid size-6 shrink-0 place-items-center rounded-full bg-brand-600 text-xs font-bold text-white">2</span><div><div class="text-sm font-semibold">打开应用并完成授权</div><p class="mt-1 text-xs text-slate-500 dark:text-slate-300">在手机端允许通知、无障碍、悬浮窗等所需权限，并保持伴侣服务在线。</p><button class="btn-secondary mt-3" @click="openCompanion"><Smartphone :size="15" />尝试打开伴侣</button></div></li>
          <li class="flex gap-3 rounded-lg border border-slate-200 bg-slate-50 p-3 dark:border-white/10 dark:bg-white/5"><span class="grid size-6 shrink-0 place-items-center rounded-full bg-brand-600 text-xs font-bold text-white">3</span><div><div class="text-sm font-semibold">同步能力与权限</div><p class="mt-1 text-xs text-slate-500 dark:text-slate-300">Web 服务通过原 Core/QUIC 获取能力目录，不绕过原核心。</p><button class="btn-primary mt-3" @click="load"><RefreshCw :size="15" :class="loading ? 'animate-spin' : ''" />刷新连接状态</button></div></li>
        </ol>
        <div class="mt-4 rounded-lg border px-3 py-2 text-xs" :class="connected ? 'border-brand-200 bg-brand-50 text-brand-800 dark:border-brand-500/20 dark:bg-brand-500/10 dark:text-brand-300' : 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-500/20 dark:bg-amber-500/10 dark:text-amber-300'">
          <span class="font-semibold">{{ connected ? '伴侣会话已连接' : '等待伴侣会话连接' }}</span>
          <span v-if="companionError" class="ml-1">{{ companionError }}</span>
        </div>
      </div>
    </section>

    <section class="grid gap-4">
      <div class="card overflow-hidden">
        <div class="flex items-center border-b border-slate-100 p-4 dark:border-white/7"><div><h3 class="section-title m-0">能力与权限</h3><p class="mt-1 text-xs text-slate-500 dark:text-slate-300">选择能力后可在右侧执行对应操作</p></div><button class="icon-button ml-auto" title="刷新" @click="load"><RefreshCw :size="16" :class="loading ? 'animate-spin' : ''" /></button></div>
        <div class="divide-y divide-slate-100 dark:divide-white/7"><button v-for="capability in capabilities" :key="capability.id" class="flex w-full items-center gap-3 px-4 py-3 text-left hover:bg-slate-50 dark:hover:bg-white/5" @click="choose(capability)"><div class="grid size-9 place-items-center rounded-lg" :class="permissionFor(capability.id)?.granted ? 'bg-brand-50 text-brand-600 dark:bg-brand-500/10 dark:text-brand-300' : 'bg-amber-50 text-amber-600 dark:bg-amber-500/10 dark:text-amber-300'"><CheckCircle2 v-if="permissionFor(capability.id)?.granted" :size="17" /><ShieldAlert v-else :size="17" /></div><div class="min-w-0 flex-1"><div class="truncate text-sm font-semibold">{{ capability.displayName || capability.id }}</div><div class="mt-0.5 truncate font-mono text-[10px] text-slate-500 dark:text-slate-300">{{ capability.id }}</div></div><span class="text-[10px] font-medium" :class="permissionFor(capability.id)?.granted ? 'text-brand-700 dark:text-brand-300' : 'text-amber-700 dark:text-amber-300'">{{ permissionFor(capability.id)?.granted ? '已授权' : '待授权' }}</span></button><div v-if="!capabilities.length && !loading" class="p-10 text-center text-xs text-slate-500 dark:text-slate-300"><Wifi :size="25" class="mx-auto mb-3" />尚未同步到 Companion 能力</div></div>
      </div>

      <section class="card p-5"><div class="flex items-center gap-2"><KeyRound :size="17" class="text-brand-600" /><h3 class="section-title m-0">能力调用</h3></div><template v-if="selected"><div class="mt-5 rounded-lg bg-slate-50 p-3 dark:bg-white/5"><div class="text-xs font-semibold">{{ selected.displayName || selected.id }}</div><div class="mt-1 font-mono text-[10px] text-slate-500 dark:text-slate-300">{{ selected.id }}</div></div><label class="mt-4 block text-xs font-medium text-slate-700 dark:text-slate-200">操作</label><UiSelect v-model="operation" class="mt-2" aria-label="能力操作"><option v-for="item in selected.operations" :key="item" :value="item">{{ item }}</option></UiSelect><label class="mt-4 block text-xs font-medium text-slate-700 dark:text-slate-200">参数 JSON</label><textarea v-model="argumentsJson" class="field mt-2 !h-36 py-3 font-mono text-xs" spellcheck="false" /><button class="btn-primary mt-4 w-full" @click="invoke"><Play :size="15" />发送请求</button></template><div v-else class="grid min-h-56 place-items-center text-center"><div><KeyRound :size="25" class="mx-auto mb-3 text-slate-400 dark:text-slate-500" /><p class="m-0 text-xs text-slate-500 dark:text-slate-300">从能力列表选择一项</p></div></div></section>
    </section>
  </div>
</template>
