<script setup lang="ts">
import { nextTick, ref } from 'vue'
import { Play, Terminal, Trash2 } from 'lucide-vue-next'
import { api, toAppError } from '@/services/api'
import { useUiStore } from '@/stores/ui'

const props = defineProps<{ deviceId: string }>()
const ui = useUiStore()
const command = ref('shell getprop ro.product.model')
const lines = ref<Array<{ type: 'command' | 'stdout' | 'stderr'; text: string }>>([])
const running = ref(false)
const output = ref<HTMLElement>()

function tokenize(value: string): string[] {
  const result: string[] = []
  const pattern = /"([^"]*)"|'([^']*)'|([^\s]+)/g
  for (const match of value.matchAll(pattern)) result.push(match[1] ?? match[2] ?? match[3])
  return result
}

async function run() {
  const args = tokenize(command.value.trim())
  if (!args.length) return
  lines.value.push({ type: 'command', text: `$ adb -s ${props.deviceId} ${args.join(' ')}` })
  running.value = true
  try {
    const result = await api<{ stdout: string; stderr: string }>(`/devices/${encodeURIComponent(props.deviceId)}/terminal`, { method: 'POST', body: JSON.stringify({ args }) })
    if (result.stdout) lines.value.push({ type: 'stdout', text: result.stdout })
    if (result.stderr) lines.value.push({ type: 'stderr', text: result.stderr })
  } catch (error) { const appError = toAppError(error); lines.value.push({ type: 'stderr', text: `${appError.errorCode}: ${appError.message}\n${appError.suggestion ?? ''}` }); ui.failure(appError) }
  finally { running.value = false; await nextTick(); output.value?.scrollTo({ top: output.value.scrollHeight }) }
}
</script>

<template>
  <div class="overflow-hidden rounded-xl border border-[#233028] bg-[#09110d] shadow-sm"><div class="flex h-11 items-center border-b border-white/8 px-4 text-slate-300"><Terminal :size="16" class="mr-2 text-brand-500" /><span class="text-xs font-semibold">ADB 终端</span><button class="ml-auto grid size-7 place-items-center rounded text-slate-500 hover:bg-white/8 hover:text-white" title="清空" @click="lines = []"><Trash2 :size="14" /></button></div><div ref="output" class="h-[420px] overflow-auto p-4 font-mono text-xs leading-6"><div v-if="!lines.length" class="text-slate-600">ADBControl 安全终端已就绪。命令以参数数组发送，不经过主机 Shell。</div><pre v-for="(line, index) in lines" :key="index" class="m-0 whitespace-pre-wrap break-all" :class="line.type === 'command' ? 'mt-3 text-brand-400 first:mt-0' : line.type === 'stderr' ? 'text-red-400' : 'text-slate-300'">{{ line.text }}</pre></div><div class="flex gap-2 border-t border-white/8 p-3"><input v-model="command" class="h-10 min-w-0 flex-1 rounded-lg border border-white/9 bg-white/5 px-3 font-mono text-xs text-slate-200 outline-none placeholder:text-slate-600 focus:border-brand-500/60" placeholder="shell getprop ro.product.model" @keyup.enter="run" /><button class="btn-primary" :disabled="running || !command.trim()" @click="run"><Play :size="15" />{{ running ? '执行中' : '执行' }}</button></div></div>
</template>
