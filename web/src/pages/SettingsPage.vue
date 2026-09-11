<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { AlertTriangle, Bot, KeyRound, Monitor, Plus, Save, Server, ShieldCheck, Trash2, Wifi } from 'lucide-vue-next'
import PageHeader from '@/components/common/PageHeader.vue'
import UiNumberInput from '@/components/common/UiNumberInput.vue'
import UiSelect from '@/components/common/UiSelect.vue'
import UiSwitch from '@/components/common/UiSwitch.vue'
import AccountManagement from '@/components/settings/AccountManagement.vue'
import { api, toAppError } from '@/services/api'
import { getSession, updatePassword } from '@/services/auth'
import { useUiStore } from '@/stores/ui'
import type { AIModel, AppSettings } from '@/types/api'

const route = useRoute()
const router = useRouter()
const ui = useUiStore()
const settings = ref<AppSettings>({ theme: 'system', language: 'zh-CN', refreshSeconds: 5, screenFps: 2, aiModels: [], remoteEnabled: false, remoteAddress: '0.0.0.0', remotePort: 45921 })
const currentPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const passwordBusy = ref(false)
const passwordError = ref('')
const mustChangePassword = ref(route.query.password === 'required')
const savedRemoteSignature = ref('')
const sessionLoaded = ref(false)

function remoteSignature(value: AppSettings) {
  return `${value.remoteEnabled}:${value.remoteAddress}:${value.remotePort}`
}

function applyTheme(theme: AppSettings['theme']) {
  const dark = theme === 'dark' || (theme === 'system' && matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', dark)
}
watch(() => settings.value.theme, applyTheme)

function addModel() {
  const model: AIModel = { id: crypto.randomUUID(), name: '新模型', baseUrl: 'https://api.openai.com/v1', model: 'gpt-4.1', apiKey: '', vision: true }
  settings.value.aiModels.push(model)
}

async function load() {
  try {
    const [savedSettings, session] = await Promise.all([api<AppSettings>('/settings'), getSession()])
    settings.value = { ...savedSettings, aiModels: savedSettings.aiModels ?? [] }
    savedRemoteSignature.value = remoteSignature(settings.value)
    mustChangePassword.value = session.mustChangePassword
    sessionLoaded.value = true
    applyTheme(settings.value.theme)
  } catch (error) {
    ui.failure(toAppError(error))
  }
}

async function save() {
  try {
    const remoteChanged = remoteSignature(settings.value) !== savedRemoteSignature.value
    settings.value = await api('/settings', { method: 'PUT', body: JSON.stringify(settings.value) })
    savedRemoteSignature.value = remoteSignature(settings.value)
    ui.notify('设置已保存', remoteChanged ? '界面设置已生效；网络监听设置会在服务重启后生效。' : '新的配置已立即生效。', 'success')
  } catch (error) {
    ui.failure(toAppError(error))
  }
}

async function changeAdminPassword() {
  passwordError.value = ''
  const passwordBytes = new TextEncoder().encode(newPassword.value).length
  if (passwordBytes < 8) {
    passwordError.value = '新密码至少需要 8 个字节'
    return
  }
  if (passwordBytes > 1024) {
    passwordError.value = '新密码不能超过 1024 个字节'
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    passwordError.value = '两次输入的新密码不一致'
    return
  }
  passwordBusy.value = true
  try {
    await updatePassword(currentPassword.value, newPassword.value)
    currentPassword.value = ''
    newPassword.value = ''
    confirmPassword.value = ''
    mustChangePassword.value = false
    const query = { ...route.query }
    delete query.password
    await router.replace({ query })
	ui.notify('管理员密码已更新', 'Core 管理员密码已同步更新，其他旧会话已失效。', 'success')
  } catch (error) {
    passwordError.value = toAppError(error).message
  } finally {
    passwordBusy.value = false
  }
}

onMounted(load)
</script>

<template>
  <PageHeader eyebrow="Settings" title="系统设置" description="管理登录密码、界面偏好、屏幕刷新与服务端 AI 模型。"><template #actions><button class="btn-primary" @click="save"><Save :size="15" />保存设置</button></template></PageHeader>
  <div class="mx-auto grid max-w-5xl gap-4">
    <section class="card p-5" :class="mustChangePassword ? '!border-amber-300 bg-amber-50/60 dark:!border-amber-500/30 dark:bg-amber-500/5' : ''">
      <div class="flex items-start gap-3">
        <div class="grid size-9 shrink-0 place-items-center rounded-lg" :class="mustChangePassword ? 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300' : 'bg-brand-50 text-brand-700 dark:bg-brand-500/10 dark:text-brand-300'"><AlertTriangle v-if="mustChangePassword" :size="18" /><KeyRound v-else :size="18" /></div>
        <div><h2 class="section-title m-0">管理密码</h2><p class="mt-1 text-xs leading-5" :class="mustChangePassword ? 'text-amber-700 dark:text-amber-300' : 'text-slate-400'">{{ mustChangePassword ? '当前仍在使用默认密码 admin，建议尽快修改以保护管理后台。' : '修改登录管理后台所使用的密码，修改后其他旧会话会立即失效。' }}</p></div>
      </div>
      <div class="mt-5 grid gap-4 md:grid-cols-3">
        <label class="text-xs font-medium">当前密码<input v-model="currentPassword" type="password" autocomplete="current-password" class="field mt-2" placeholder="输入当前密码" /></label>
        <label class="text-xs font-medium">新密码<input v-model="newPassword" type="password" autocomplete="new-password" class="field mt-2" placeholder="8–1024 字节" /></label>
        <label class="text-xs font-medium">确认新密码<input v-model="confirmPassword" type="password" autocomplete="new-password" class="field mt-2" placeholder="再次输入新密码" @keyup.enter="changeAdminPassword" /></label>
      </div>
      <div class="mt-4 flex flex-wrap items-center gap-3"><button class="btn-primary" :disabled="passwordBusy || !currentPassword || !newPassword || !confirmPassword" @click="changeAdminPassword"><KeyRound :size="15" />{{ passwordBusy ? '正在更新…' : '修改管理密码' }}</button><p v-if="passwordError" class="text-xs text-red-600" role="alert">{{ passwordError }}</p></div>
    </section>

    <AccountManagement v-if="sessionLoaded && !mustChangePassword" />
    <section v-else-if="sessionLoaded" class="card border-amber-200 p-5 dark:border-amber-500/20">
      <h2 class="section-title m-0">远程用户与设备权限</h2>
      <p class="mt-2 text-xs leading-5 text-amber-700 dark:text-amber-300">请先修改默认管理员密码，再创建远程用户或分配设备。</p>
    </section>

    <section class="card p-5">
	  <div class="flex flex-col items-start gap-3 sm:flex-row"><div class="grid size-9 shrink-0 place-items-center rounded-lg bg-sky-50 text-sky-700 dark:bg-sky-500/10 dark:text-sky-300"><Wifi :size="18" /></div><div><h2 class="section-title m-0">远程控制服务</h2><p class="mt-1 text-xs leading-5 text-slate-400">启用 Rust Core 的 QUIC/TLS 1.3 远程入口；保存后重启服务生效。</p></div><UiSwitch v-model="settings.remoteEnabled" class="sm:ml-auto" label="启用远程控制" :disabled="mustChangePassword" /></div>
      <div class="mt-5 grid gap-4 sm:grid-cols-2"><label class="text-xs font-medium">监听 IP<input v-model="settings.remoteAddress" class="field mt-2 font-mono" :disabled="!settings.remoteEnabled" placeholder="0.0.0.0" /></label><label class="text-xs font-medium">监听端口<UiNumberInput v-model="settings.remotePort" :min="1024" :max="65535" class="mt-2" aria-label="远程监听端口" /></label></div>
	  <div class="mt-4 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800 dark:border-amber-500/20 dark:bg-amber-500/7 dark:text-amber-300">该端口使用 UDP，不是 Web HTTP 端口。请放行对应 UDP 防火墙规则；内置管理员不能通过远程入口登录，远程客户端仅接受下方创建的用户。</div>
    </section>

    <section class="card p-5"><div class="flex items-center gap-3"><div class="grid size-9 place-items-center rounded-lg bg-brand-50 text-brand-700 dark:bg-brand-500/10 dark:text-brand-300"><Monitor :size="18" /></div><div><h2 class="section-title m-0">外观与刷新</h2><p class="mt-1 text-xs text-slate-400">所有浏览器设备共享服务端配置</p></div></div><div class="mt-5 grid gap-4 sm:grid-cols-3"><label class="text-xs font-medium">主题<UiSelect v-model="settings.theme" class="mt-2" aria-label="主题"><option value="system">跟随系统</option><option value="light">浅色</option><option value="dark">深色</option></UiSelect></label><label class="text-xs font-medium">设备刷新间隔（秒）<UiNumberInput v-model="settings.refreshSeconds" :min="1" :max="120" class="mt-2" aria-label="设备刷新间隔" /></label><label class="text-xs font-medium">屏幕刷新帧率<UiNumberInput v-model="settings.screenFps" :min="1" :max="10" class="mt-2" aria-label="屏幕刷新帧率" /></label></div></section>
    <section class="card overflow-hidden"><div class="flex items-center border-b border-slate-100 p-5 dark:border-white/7"><div class="grid size-9 place-items-center rounded-lg bg-violet-50 text-violet-700 dark:bg-violet-500/10 dark:text-violet-300"><Bot :size="18" /></div><div class="ml-3"><h2 class="section-title m-0">AI 模型</h2><p class="mt-1 text-xs text-slate-400">支持 OpenAI 兼容接口，密钥只保存在服务端</p></div><button class="btn-secondary ml-auto" @click="addModel"><Plus :size="15" />添加模型</button></div><div v-if="settings.aiModels.length" class="divide-y divide-slate-100 dark:divide-white/7"><div v-for="(model,index) in settings.aiModels" :key="model.id" class="grid gap-3 p-5 lg:grid-cols-[1fr_1.4fr_1fr_1fr_auto]"><label class="text-[10px] font-medium text-slate-500">显示名称<input v-model="model.name" class="field mt-1.5" /></label><label class="text-[10px] font-medium text-slate-500">接口地址<input v-model="model.baseUrl" class="field mt-1.5 font-mono text-xs" /></label><label class="text-[10px] font-medium text-slate-500">模型 ID<input v-model="model.model" class="field mt-1.5 font-mono text-xs" /></label><label class="text-[10px] font-medium text-slate-500">API Key<input v-model="model.apiKey" type="password" class="field mt-1.5 font-mono text-xs" /></label><div class="flex items-end gap-2 pb-0.5"><UiSwitch v-model="model.vision" label="视觉" /><button class="icon-button hover:!text-red-600" @click="settings.aiModels.splice(index,1)"><Trash2 :size="16" /></button></div></div></div><div v-else class="p-10 text-center text-xs text-slate-400">尚未配置 AI 模型</div><div v-if="settings.aiModels.length" class="border-t border-slate-100 p-5 dark:border-white/7"><label class="block max-w-sm text-xs font-medium">默认模型<UiSelect v-model="settings.defaultModelId" class="mt-2" aria-label="默认模型"><option v-for="model in settings.aiModels" :key="model.id" :value="model.id">{{ model.name }}</option></UiSelect></label></div></section>
	<section class="grid gap-4 md:grid-cols-2"><div class="card flex gap-3 p-5"><Server :size="19" class="mt-0.5 text-brand-600" /><div><h3 class="m-0 text-sm font-semibold">服务架构</h3><p class="mt-2 text-xs leading-6 text-slate-500 dark:text-slate-400">Vue 3 Web 客户端通过 Go 网络层访问原 Rust Core，设备命令不会绕过核心安全边界。</p></div></div><div class="card flex gap-3 p-5"><ShieldCheck :size="19" class="mt-0.5 text-brand-600" /><div><h3 class="m-0 text-sm font-semibold">访问安全</h3><p class="mt-2 text-xs leading-6 text-slate-500 dark:text-slate-400">管理员与远程用户密码均由 Core 使用 Argon2id 保存；Go 不维护第二套账户库，浏览器仅持有 HttpOnly 会话。</p></div></div></section>
  </div>
</template>
