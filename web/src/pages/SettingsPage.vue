<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { Bot, Monitor, Plus, Save, Server, ShieldCheck, Trash2 } from 'lucide-vue-next'
import PageHeader from '@/components/common/PageHeader.vue'
import { api, toAppError } from '@/services/api'
import { useUiStore } from '@/stores/ui'
import type { AIModel, AppSettings } from '@/types/api'

const ui = useUiStore()
const settings = ref<AppSettings>({ theme:'system', language:'zh-CN', refreshSeconds:5, screenFps:2, aiModels:[] })

function applyTheme(theme: AppSettings['theme']) {
  const dark = theme === 'dark' || (theme === 'system' && matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', dark)
}
watch(() => settings.value.theme, applyTheme)

function addModel() {
  const model: AIModel = { id: crypto.randomUUID(), name:'新模型', baseUrl:'https://api.openai.com/v1', model:'gpt-4.1', apiKey:'', vision:true }
  settings.value.aiModels.push(model)
}

async function load() { try { settings.value = await api('/settings'); applyTheme(settings.value.theme) } catch (error) { ui.failure(toAppError(error)) } }
async function save() { try { settings.value = await api('/settings', { method:'PUT', body:JSON.stringify(settings.value) }); ui.notify('设置已保存','新的配置已立即生效。','success') } catch (error) { ui.failure(toAppError(error)) } }
onMounted(load)
</script>

<template>
  <PageHeader eyebrow="Settings" title="系统设置" description="管理界面偏好、屏幕刷新与服务端 AI 模型。"><template #actions><button class="btn-primary" @click="save"><Save :size="15" />保存设置</button></template></PageHeader>
  <div class="mx-auto grid max-w-5xl gap-4">
    <section class="card p-5"><div class="flex items-center gap-3"><div class="grid size-9 place-items-center rounded-lg bg-brand-50 text-brand-700 dark:bg-brand-500/10 dark:text-brand-300"><Monitor :size="18" /></div><div><h2 class="section-title m-0">外观与刷新</h2><p class="mt-1 text-xs text-slate-400">所有浏览器设备共享服务端配置</p></div></div><div class="mt-5 grid gap-4 sm:grid-cols-3"><label class="text-xs font-medium">主题<select v-model="settings.theme" class="field mt-2"><option value="system">跟随系统</option><option value="light">浅色</option><option value="dark">深色</option></select></label><label class="text-xs font-medium">设备刷新间隔（秒）<input v-model.number="settings.refreshSeconds" type="number" min="1" max="120" class="field mt-2" /></label><label class="text-xs font-medium">屏幕刷新帧率<input v-model.number="settings.screenFps" type="number" min="1" max="10" class="field mt-2" /></label></div></section>
    <section class="card overflow-hidden"><div class="flex items-center border-b border-slate-100 p-5 dark:border-white/7"><div class="grid size-9 place-items-center rounded-lg bg-violet-50 text-violet-700 dark:bg-violet-500/10 dark:text-violet-300"><Bot :size="18" /></div><div class="ml-3"><h2 class="section-title m-0">AI 模型</h2><p class="mt-1 text-xs text-slate-400">支持 OpenAI 兼容接口，密钥只保存在服务端</p></div><button class="btn-secondary ml-auto" @click="addModel"><Plus :size="15" />添加模型</button></div><div v-if="settings.aiModels.length" class="divide-y divide-slate-100 dark:divide-white/7"><div v-for="(model,index) in settings.aiModels" :key="model.id" class="grid gap-3 p-5 lg:grid-cols-[1fr_1.4fr_1fr_1fr_auto]"><label class="text-[10px] font-medium text-slate-500">显示名称<input v-model="model.name" class="field mt-1.5" /></label><label class="text-[10px] font-medium text-slate-500">接口地址<input v-model="model.baseUrl" class="field mt-1.5 font-mono text-xs" /></label><label class="text-[10px] font-medium text-slate-500">模型 ID<input v-model="model.model" class="field mt-1.5 font-mono text-xs" /></label><label class="text-[10px] font-medium text-slate-500">API Key<input v-model="model.apiKey" type="password" class="field mt-1.5 font-mono text-xs" /></label><div class="flex items-end gap-2 pb-0.5"><label class="flex h-10 items-center gap-2 text-xs"><input v-model="model.vision" type="checkbox" class="accent-brand-600" />视觉</label><button class="icon-button hover:!text-red-600" @click="settings.aiModels.splice(index,1)"><Trash2 :size="16" /></button></div></div></div><div v-else class="p-10 text-center text-xs text-slate-400">尚未配置 AI 模型</div><div v-if="settings.aiModels.length" class="border-t border-slate-100 p-5 dark:border-white/7"><label class="block max-w-sm text-xs font-medium">默认模型<select v-model="settings.defaultModelId" class="field mt-2"><option v-for="model in settings.aiModels" :key="model.id" :value="model.id">{{ model.name }}</option></select></label></div></section>
    <section class="grid gap-4 md:grid-cols-2"><div class="card flex gap-3 p-5"><Server :size="19" class="mt-0.5 text-brand-600" /><div><h3 class="m-0 text-sm font-semibold">服务架构</h3><p class="mt-2 text-xs leading-6 text-slate-500 dark:text-slate-400">Vue 3 Web 客户端通过 Go 网络层访问原 Rust Core，设备命令不会绕过核心安全边界。</p></div></div><div class="card flex gap-3 p-5"><ShieldCheck :size="19" class="mt-0.5 text-brand-600" /><div><h3 class="m-0 text-sm font-semibold">访问安全</h3><p class="mt-2 text-xs leading-6 text-slate-500 dark:text-slate-400">非本机监听必须配置访问令牌；密钥和设备隐私数据不会写入浏览器持久存储。</p></div></div></section>
  </div>
</template>
