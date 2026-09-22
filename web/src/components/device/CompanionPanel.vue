<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { CheckCircle2, Download, KeyRound, Play, RefreshCw, ShieldAlert, Smartphone, Wifi } from 'lucide-vue-next'
import UiSelect from '@/components/common/UiSelect.vue'
import { api, toAppError } from '@/services/api'
import { useUiStore } from '@/stores/ui'
import type { CompanionStatus } from '@/types/api'

const props = defineProps<{ deviceId: string }>()
const ui = useUiStore()
const capabilities = ref<Array<Record<string, any>>>([])
const permissions = ref<Array<Record<string, any>>>([])
const status = ref<CompanionStatus | null>(null)
const loading = ref(false)
const opening = ref(false)
const selected = ref<Record<string, any> | null>(null)
const operation = ref('')
const argumentsJson = ref('{}')
const companionError = ref('')

const quicConnected = computed(() => permissions.value.length > 0)
const adbCompatibilityMode = computed(() => status.value?.adbResponsive && capabilities.value.length > 0 && !quicConnected.value)
const statusTone = computed(() => {
  if (quicConnected.value) return 'connected'
  if (status.value?.adbResponsive) return 'adb'
  if (status.value?.installed === false) return 'missing'
  return 'warning'
})
const statusTitle = computed(() => {
  if (quicConnected.value) return '伴侣 QUIC 会话已连接'
  if (adbCompatibilityMode.value) return '伴侣 ADB 兼容通道已连接'
  if (status.value?.adbResponsive) return '伴侣 App ADB 可达'
  if (status.value?.installed === false) return '未安装伴侣应用'
  return '等待伴侣连接'
})
const statusMessage = computed(() => {
  if (quicConnected.value) return '已通过原 Core/QUIC 同步能力目录，可以使用完整伴侣能力。'
  if (adbCompatibilityMode.value) return 'QUIC 会话尚未建立；能力目录来自 Rust Core，操作将通过与 Windows 客户端一致的 ADB broadcast 通道执行并逐项校验权限。'
  if (status.value?.adbResponsive) return 'App 已安装且 broadcast 探测正常；如果能力目录为空，请在手机端确认 Companion 服务与权限。'
  return status.value?.message || '请按流程安装、打开并授权 Companion。'
})

async function load() {
  loading.value = true
  companionError.value = ''
  const [statusResult, capabilitiesResult, permissionsResult] = await Promise.allSettled([
    api<CompanionStatus>(`/devices/${encodeURIComponent(props.deviceId)}/companion/status`),
    api<Array<Record<string, any>>>(`/devices/${encodeURIComponent(props.deviceId)}/capabilities`),
    api<Array<Record<string, any>>>(`/devices/${encodeURIComponent(props.deviceId)}/permissions`),
  ])
  if (statusResult.status === 'fulfilled') status.value = statusResult.value
  else companionError.value = `${toAppError(statusResult.reason).message}（${toAppError(statusResult.reason).errorCode}）`
  capabilities.value = capabilitiesResult.status === 'fulfilled' ? capabilitiesResult.value : []
  permissions.value = permissionsResult.status === 'fulfilled' ? permissionsResult.value : []
  const syncError = capabilitiesResult.status === 'rejected' ? capabilitiesResult.reason : permissionsResult.status === 'rejected' ? permissionsResult.reason : null
  if (syncError) {
    const appError = toAppError(syncError)
    companionError.value = companionError.value || `能力同步失败：${appError.message}（${appError.errorCode}）`
  }
  loading.value = false
}

function permissionFor(id: string) { return permissions.value.find(item => item.capabilityId === id) }
function permissionLabel(id: string) {
  const permission = permissionFor(id)
  if (!permission && adbCompatibilityMode.value) return '操作时校验'
  return permission?.granted ? '已授权' : '待授权'
}

async function install() {
  try { await api(`/devices/${encodeURIComponent(props.deviceId)}/companion/install`, { method: 'POST', body: '{}' }); ui.notify('伴侣应用已安装', '请在设备上打开 ADBControl Companion 并按提示授权。', 'success') }
  catch (error) { ui.failure(toAppError(error)) }
}

async function openCompanion() {
  if (opening.value) return
  opening.value = true
  try {
    await api(`/devices/${encodeURIComponent(props.deviceId)}/terminal`, { method: 'POST', body: JSON.stringify({ args: ['shell', 'monkey', '-p', 'com.adbcontrol.companion', '1'] }) })
    ui.notify('已尝试打开伴侣应用', '如设备未响应，请在手机上手动打开 ADBControl Companion。', 'success')
  } catch (error) { ui.failure(toAppError(error)) }
  finally { opening.value = false }
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
          <li class="flex gap-3 rounded-lg border border-slate-200 bg-slate-50 p-3 dark:border-white/10 dark:bg-white/5"><span class="grid size-6 shrink-0 place-items-center rounded-full bg-brand-600 text-xs font-bold text-white">2</span><div><div class="text-sm font-semibold">打开应用并完成授权</div><p class="mt-1 text-xs text-slate-500 dark:text-slate-300">在手机端允许通知、无障碍、悬浮窗等所需权限，并保持伴侣服务在线；系统不会在控制过程中自动反复调起 App。</p><button class="btn-secondary mt-3 disabled:cursor-not-allowed disabled:opacity-60" :disabled="opening" @click="openCompanion"><Smartphone :size="15" />{{ opening ? '正在打开…' : '手动打开伴侣' }}</button></div></li>
          <li class="flex gap-3 rounded-lg border border-slate-200 bg-slate-50 p-3 dark:border-white/10 dark:bg-white/5"><span class="grid size-6 shrink-0 place-items-center rounded-full bg-brand-600 text-xs font-bold text-white">3</span><div><div class="text-sm font-semibold">同步能力与权限</div><p class="mt-1 text-xs text-slate-500 dark:text-slate-300">Web 服务通过原 Core/QUIC 获取能力目录，不绕过原核心。</p><button class="btn-primary mt-3" @click="load"><RefreshCw :size="15" :class="loading ? 'animate-spin' : ''" />刷新连接状态</button></div></li>
        </ol>
        <div class="mt-4 rounded-lg border px-3 py-2 text-xs" :class="statusTone === 'connected' ? 'border-brand-200 bg-brand-50 text-brand-800 dark:border-brand-500/20 dark:bg-brand-500/10 dark:text-brand-300' : statusTone === 'adb' ? 'border-sky-200 bg-sky-50 text-sky-800 dark:border-sky-500/20 dark:bg-sky-500/10 dark:text-sky-200' : statusTone === 'missing' ? 'border-slate-200 bg-slate-50 text-slate-700 dark:border-white/10 dark:bg-white/5 dark:text-slate-200' : 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-500/20 dark:bg-amber-500/10 dark:text-amber-300'">
          <span class="font-semibold">{{ statusTitle }}</span>
          <span class="ml-1">{{ statusMessage }}</span>
          <span v-if="companionError" class="ml-1">诊断：{{ companionError }}</span>
        </div>
      </div>
    </section>

    <section class="grid gap-4">
      <div class="card overflow-hidden">
        <div class="flex items-center border-b border-slate-100 p-4 dark:border-white/7"><div><h3 class="section-title m-0">能力与权限</h3><p class="mt-1 text-xs text-slate-500 dark:text-slate-300">选择能力后可在右侧执行对应操作</p></div><button class="icon-button ml-auto" title="刷新" @click="load"><RefreshCw :size="16" :class="loading ? 'animate-spin' : ''" /></button></div>
        <div class="divide-y divide-slate-100 dark:divide-white/7"><button v-for="capability in capabilities" :key="capability.id" class="flex w-full items-center gap-3 px-4 py-3 text-left hover:bg-slate-50 dark:hover:bg-white/5" @click="choose(capability)"><div class="grid size-9 place-items-center rounded-lg" :class="permissionFor(capability.id)?.granted ? 'bg-brand-50 text-brand-600 dark:bg-brand-500/10 dark:text-brand-300' : adbCompatibilityMode ? 'bg-sky-50 text-sky-600 dark:bg-sky-500/10 dark:text-sky-300' : 'bg-amber-50 text-amber-600 dark:bg-amber-500/10 dark:text-amber-300'"><CheckCircle2 v-if="permissionFor(capability.id)?.granted" :size="17" /><Wifi v-else-if="adbCompatibilityMode" :size="17" /><ShieldAlert v-else :size="17" /></div><div class="min-w-0 flex-1"><div class="truncate text-sm font-semibold">{{ capability.displayName || capability.id }}</div><div class="mt-0.5 truncate font-mono text-[10px] text-slate-500 dark:text-slate-300">{{ capability.id }}</div></div><span class="text-[10px] font-medium" :class="permissionFor(capability.id)?.granted ? 'text-brand-700 dark:text-brand-300' : adbCompatibilityMode ? 'text-sky-700 dark:text-sky-300' : 'text-amber-700 dark:text-amber-300'">{{ permissionLabel(capability.id) }}</span></button><div v-if="!capabilities.length && !loading" class="p-10 text-center text-xs text-slate-500 dark:text-slate-300"><Wifi :size="25" class="mx-auto mb-3" />尚未同步到 Companion 能力</div></div>
      </div>

      <section class="card p-5"><div class="flex items-center gap-2"><KeyRound :size="17" class="text-brand-600" /><h3 class="section-title m-0">能力调用</h3></div><template v-if="selected"><div class="mt-5 rounded-lg bg-slate-50 p-3 dark:bg-white/5"><div class="text-xs font-semibold">{{ selected.displayName || selected.id }}</div><div class="mt-1 font-mono text-[10px] text-slate-500 dark:text-slate-300">{{ selected.id }}</div></div><label class="mt-4 block text-xs font-medium text-slate-700 dark:text-slate-200">操作</label><UiSelect v-model="operation" class="mt-2" aria-label="能力操作"><option v-for="item in selected.operations" :key="item" :value="item">{{ item }}</option></UiSelect><label class="mt-4 block text-xs font-medium text-slate-700 dark:text-slate-200">参数 JSON</label><textarea v-model="argumentsJson" class="field mt-2 !h-36 py-3 font-mono text-xs" spellcheck="false" /><button class="btn-primary mt-4 w-full" @click="invoke"><Play :size="15" />发送请求</button></template><div v-else class="grid min-h-56 place-items-center text-center"><div><KeyRound :size="25" class="mx-auto mb-3 text-slate-400 dark:text-slate-500" /><p class="m-0 text-xs text-slate-500 dark:text-slate-300">从能力列表选择一项</p></div></div></section>
    </section>
  </div>
</template>
