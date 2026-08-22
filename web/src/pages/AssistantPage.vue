<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { Bot, Image, LoaderCircle, Send, Sparkles, User } from 'lucide-vue-next'
import PageHeader from '@/components/common/PageHeader.vue'
import UiSelect from '@/components/common/UiSelect.vue'
import { api, toAppError } from '@/services/api'
import { useDevicesStore } from '@/stores/devices'
import { useUiStore } from '@/stores/ui'
import type { AIModel, AppSettings } from '@/types/api'

interface Message { role: 'user'|'assistant'; content: string; image?: string }
const ui = useUiStore()
const devices = useDevicesStore()
const models = ref<AIModel[]>([])
const modelId = ref('')
const deviceId = ref('')
const input = ref('')
const messages = ref<Message[]>([])
const sending = ref(false)
const attachScreen = ref(false)
const scroller = ref<HTMLElement>()
const selectedModel = computed(() => models.value.find(model => model.id === modelId.value))

onMounted(async () => {
  await devices.refresh()
  try { const settings = await api<AppSettings>('/settings'); models.value = settings.aiModels; modelId.value = settings.defaultModelId || models.value[0]?.id || '' }
  catch (error) { ui.failure(toAppError(error)) }
})

async function send() {
  const text = input.value.trim()
  if (!text || sending.value || !modelId.value) return
  let image: string | undefined
  if (attachScreen.value && deviceId.value) {
    try { const response = await fetch(`/api/v1/devices/${encodeURIComponent(deviceId.value)}/screenshot`); if (!response.ok) throw new Error('截图失败'); image = await blobToDataURL(await response.blob()) }
    catch (error) { ui.failure(toAppError(error)); return }
  }
  messages.value.push({ role: 'user', content: text, image }); input.value = ''; attachScreen.value = false; sending.value = true
  const assistant: Message = { role: 'assistant', content: '' }; messages.value.push(assistant)
  try {
    const payloadMessages = messages.value.slice(0,-1).map(message => ({ role: message.role, content: message.image ? [{ type:'text', text: message.content }, { type:'image_url', image_url:{ url: message.image } }] : message.content }))
    const response = await fetch('/api/v1/ai/chat', { method:'POST', credentials:'same-origin', headers:{'Content-Type':'application/json'}, body:JSON.stringify({ modelId:modelId.value, messages:payloadMessages }) })
    if (!response.ok) { const payload = await response.json(); throw payload.error }
    const reader = response.body?.getReader(); const decoder = new TextDecoder(); let buffer = ''
    while (reader) {
      const { done, value } = await reader.read(); if (done) break
      buffer += decoder.decode(value, { stream:true })
      const events = buffer.split('\n\n'); buffer = events.pop() || ''
      for (const event of events) for (const line of event.split('\n')) if (line.startsWith('data: ') && line !== 'data: [DONE]') {
        try { const chunk = JSON.parse(line.slice(6)); assistant.content += chunk.choices?.[0]?.delta?.content ?? '' } catch { /* Incomplete provider event stays buffered by SSE framing. */ }
      }
      await nextTick(); scroller.value?.scrollTo({ top: scroller.value.scrollHeight })
    }
  } catch (error) { messages.value.pop(); ui.failure(toAppError(error)) }
  finally { sending.value = false }
}

function blobToDataURL(blob: Blob) { return new Promise<string>((resolve,reject)=>{ const reader=new FileReader(); reader.onload=()=>resolve(String(reader.result)); reader.onerror=reject; reader.readAsDataURL(blob) }) }
</script>

<template>
  <PageHeader eyebrow="AI Assistant" title="AI 设备助手" description="结合全局设备上下文、屏幕观察与受控操作，协助诊断和管理 Android 设备。" />
  <div class="grid min-h-[calc(100vh-190px)] gap-4 xl:grid-cols-[1fr_280px]">
    <section class="card flex min-h-[620px] flex-col overflow-hidden"><div ref="scroller" class="flex-1 overflow-auto p-5 sm:p-7"><div v-if="!messages.length" class="grid h-full min-h-[420px] place-items-center text-center"><div class="max-w-md"><div class="mx-auto grid size-12 place-items-center rounded-xl bg-brand-50 text-brand-700 dark:bg-brand-500/10 dark:text-brand-300"><Sparkles :size="22" /></div><h2 class="mt-5 text-lg font-semibold">有什么可以帮你？</h2><p class="mt-2 text-sm leading-6 text-slate-500 dark:text-slate-400">可以询问设备状态、排查连接问题，或附加当前屏幕进行视觉分析。</p><div class="mt-5 flex flex-wrap justify-center gap-2"><button v-for="prompt in ['检查所有设备状态','分析设备连接问题','解释当前屏幕内容']" :key="prompt" class="btn-secondary !h-8 !text-xs" @click="input=prompt">{{ prompt }}</button></div></div></div><div v-else class="mx-auto max-w-3xl space-y-5"><div v-for="(message,index) in messages" :key="index" class="flex gap-3" :class="message.role==='user' ? 'flex-row-reverse' : ''"><div class="grid size-8 shrink-0 place-items-center rounded-lg" :class="message.role==='user' ? 'bg-slate-900 text-white dark:bg-white dark:text-slate-900' : 'bg-brand-50 text-brand-700 dark:bg-brand-500/10 dark:text-brand-300'"><User v-if="message.role==='user'" :size="15" /><Bot v-else :size="16" /></div><div class="max-w-[82%] rounded-xl px-4 py-3 text-sm leading-7" :class="message.role==='user' ? 'bg-slate-900 text-white dark:bg-white dark:text-slate-900' : 'bg-slate-50 text-slate-700 dark:bg-white/5 dark:text-slate-200'"><img v-if="message.image" :src="message.image" alt="随消息发送的设备屏幕" class="mb-3 max-h-72 rounded-lg" /><div class="whitespace-pre-wrap">{{ message.content }}<LoaderCircle v-if="sending && index===messages.length-1 && !message.content" :size="16" class="animate-spin text-brand-500" /></div></div></div></div></div><div class="border-t border-slate-100 p-3 dark:border-white/7 sm:p-4"><div class="mx-auto max-w-3xl"><div v-if="attachScreen" class="mb-2 flex items-center gap-2 rounded-lg border border-brand-200 bg-brand-50 px-3 py-2 text-xs text-brand-700 dark:border-brand-500/20 dark:bg-brand-500/8 dark:text-brand-300"><Image :size="15" />将附加所选设备的当前屏幕<button class="ml-auto" @click="attachScreen=false">取消</button></div><div class="flex items-end gap-2 rounded-xl border border-slate-200 bg-white p-2 focus-within:border-brand-500 dark:border-white/10 dark:bg-white/3"><textarea v-model="input" class="max-h-40 min-h-10 flex-1 resize-none border-0 bg-transparent px-2 py-2 text-sm outline-none" rows="1" placeholder="输入消息，Enter 发送，Shift+Enter 换行" @keydown.enter.exact.prevent="send" /><button class="icon-button" :class="attachScreen ? '!bg-brand-50 !text-brand-700 dark:!bg-brand-500/10' : ''" title="附加设备屏幕" :disabled="!deviceId" @click="attachScreen=!attachScreen"><Image :size="18" /></button><button class="btn-primary !size-10 !px-0" :disabled="!input.trim() || sending || !modelId" @click="send"><Send :size="17" /></button></div></div></div></section>
    <aside class="space-y-4"><div class="card p-4"><h3 class="section-title m-0">对话配置</h3><label class="mt-4 block text-xs font-medium">AI 模型</label><UiSelect v-model="modelId" class="mt-2" aria-label="AI 模型"><option v-if="!models.length" value="">请先添加模型</option><option v-for="model in models" :key="model.id" :value="model.id">{{ model.name }}</option></UiSelect><p v-if="selectedModel" class="mt-2 text-[10px] text-slate-400">{{ selectedModel.model }} · {{ selectedModel.vision ? '支持视觉' : '仅文本' }}</p><label class="mt-4 block text-xs font-medium">目标设备</label><UiSelect v-model="deviceId" class="mt-2" aria-label="目标设备"><option value="">暂不指定</option><option v-for="device in devices.online" :key="device.id" :value="device.id">{{ device.name }}</option></UiSelect></div><div class="card p-4 text-xs leading-6 text-slate-500 dark:text-slate-400"><div class="mb-2 flex items-center gap-2 font-semibold text-slate-800 dark:text-slate-200"><Bot :size="15" class="text-brand-600" />安全边界</div>屏幕只在你明确附加时发送。敏感或破坏性设备操作仍需二次确认，不会由普通对话自动执行。</div></aside>
  </div>
</template>
