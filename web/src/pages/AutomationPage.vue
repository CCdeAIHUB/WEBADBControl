<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Braces, CirclePause, CirclePlay, Clock3, Plus, RefreshCw, Square, Trash2, X } from 'lucide-vue-next'
import PageHeader from '@/components/common/PageHeader.vue'
import ConfirmDialog from '@/components/feedback/ConfirmDialog.vue'
import { api, toAppError } from '@/services/api'
import { useUiStore } from '@/stores/ui'
import type { AutomationRun, AutomationTask } from '@/types/api'

const ui = useUiStore()
const tasks = ref<AutomationTask[]>([])
const runs = ref<AutomationRun[]>([])
const loading = ref(false)
const editorOpen = ref(false)
const editor = ref('')
const deleting = ref<AutomationTask | null>(null)
const activeRuns = computed(() => runs.value.filter(run => ['queued','running','paused'].includes(run.status)))

const template = {
  schemaVersion: 1, id: '', name: '新自动化任务', description: '', deviceId: '', enabled: true,
  concurrencyPolicy: 'skip', permissions: { allowAdb: false, allowShell: false, allowCompanion: false, allowAi: false },
  triggers: [{ type: 'manual' }], actions: [{ type: 'log', parameters: { message: '任务开始' } }],
}

async function load() {
  loading.value = true
  try { [tasks.value, runs.value] = await Promise.all([api<AutomationTask[]>('/automation/tasks'), api<AutomationRun[]>('/automation/runs')]) }
  catch (error) { ui.failure(toAppError(error)) }
  finally { loading.value = false }
}

function createTask() { editor.value = JSON.stringify(template, null, 2); editorOpen.value = true }
function editTask(task: AutomationTask) { editor.value = JSON.stringify(task, null, 2); editorOpen.value = true }

async function save() {
  try {
    const task = JSON.parse(editor.value) as AutomationTask
    const path = task.id ? `/automation/tasks/${encodeURIComponent(task.id)}` : '/automation/tasks'
    await api(path, { method: task.id ? 'PUT' : 'POST', body: JSON.stringify(task) })
    editorOpen.value = false; ui.notify('任务已保存', task.name, 'success'); await load()
  } catch (error) { ui.failure(toAppError(error)) }
}

async function run(task: AutomationTask) {
  try { await api(`/automation/tasks/${encodeURIComponent(task.id)}/run`, { method: 'POST', body: '{}' }); ui.notify('任务已开始', task.name, 'success'); setTimeout(load, 300) }
  catch (error) { ui.failure(toAppError(error)) }
}

async function remove() {
  if (!deleting.value) return
  try { await api(`/automation/tasks/${encodeURIComponent(deleting.value.id)}`, { method: 'DELETE' }); ui.notify('任务已删除', deleting.value.name, 'success'); deleting.value = null; await load() }
  catch (error) { ui.failure(toAppError(error)) }
}

async function control(run: AutomationRun, operation: 'pause'|'resume'|'stop') {
  try { await api(`/automation/runs/${encodeURIComponent(run.id)}/${operation}`, { method: 'POST', body: '{}' }); setTimeout(load, 200) }
  catch (error) { ui.failure(toAppError(error)) }
}

function statusClass(status: string) { return status === 'succeeded' ? 'bg-brand-50 text-brand-700 dark:bg-brand-500/10 dark:text-brand-300' : status === 'failed' ? 'bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-300' : 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300' }
onMounted(load)
</script>

<template>
  <PageHeader eyebrow="Automation" title="自动化任务" description="使用受控 JSON DSL 编排设备操作、定时触发与 Companion 能力。">
    <template #actions><button class="btn-secondary" @click="load"><RefreshCw :size="15" :class="loading ? 'animate-spin' : ''" />刷新</button><button class="btn-primary" @click="createTask"><Plus :size="16" />新建任务</button></template>
  </PageHeader>
  <section v-if="activeRuns.length" class="mb-5 rounded-xl border border-amber-200 bg-amber-50/60 p-4 dark:border-amber-500/15 dark:bg-amber-500/7"><div class="mb-3 flex items-center gap-2 text-sm font-semibold text-amber-800 dark:text-amber-300"><Clock3 :size="16" />正在运行</div><div class="grid gap-2 md:grid-cols-2"><div v-for="runItem in activeRuns" :key="runItem.id" class="flex items-center gap-3 rounded-lg bg-white p-3 dark:bg-white/5"><div class="min-w-0 flex-1"><div class="truncate text-xs font-semibold">{{ tasks.find(t => t.id === runItem.taskId)?.name || runItem.taskId }}</div><div class="mt-2 h-1 overflow-hidden rounded bg-slate-100 dark:bg-white/10"><div class="h-full bg-amber-500" :style="{width:`${runItem.progress*100}%`}" /></div></div><button v-if="runItem.status === 'paused'" class="icon-button" title="继续" @click="control(runItem,'resume')"><CirclePlay :size="16" /></button><button v-else class="icon-button" title="暂停" @click="control(runItem,'pause')"><CirclePause :size="16" /></button><button class="icon-button hover:!text-red-600" title="停止" @click="control(runItem,'stop')"><Square :size="14" /></button></div></div></section>
  <div class="grid gap-5 xl:grid-cols-[1.25fr_.75fr]">
    <section class="card overflow-hidden"><div class="border-b border-slate-100 px-5 py-4 dark:border-white/7"><h2 class="section-title m-0">任务列表</h2><p class="mt-1 text-xs text-slate-400">{{ tasks.length }} 个任务</p></div><div v-if="tasks.length" class="divide-y divide-slate-100 dark:divide-white/7"><div v-for="task in tasks" :key="task.id" class="flex flex-wrap items-center gap-3 px-5 py-4"><div class="grid size-10 place-items-center rounded-lg bg-blue-50 text-blue-600 dark:bg-blue-500/10 dark:text-blue-300"><Braces :size="18" /></div><button class="min-w-0 flex-1 text-left" @click="editTask(task)"><div class="truncate text-sm font-semibold">{{ task.name }}</div><div class="mt-1 flex flex-wrap gap-2 text-[10px] text-slate-400"><span>{{ task.triggers.map(t=>t.type).join(' · ') }}</span><span>{{ task.actions.length }} 个步骤</span><span>{{ task.enabled ? '已启用' : '已停用' }}</span></div></button><button class="btn-secondary !h-8 !px-2.5 !text-xs" @click="run(task)"><CirclePlay :size="14" />运行</button><button class="icon-button hover:!text-red-600" title="删除" @click="deleting = task"><Trash2 :size="15" /></button></div></div><div v-else class="p-12 text-center"><Braces :size="28" class="mx-auto mb-3 text-slate-300" /><p class="text-sm font-semibold">还没有自动化任务</p><p class="text-xs text-slate-400">从手动触发任务开始建立你的工作流。</p><button class="btn-primary mt-3" @click="createTask">创建第一个任务</button></div></section>
    <section class="card overflow-hidden"><div class="border-b border-slate-100 px-5 py-4 dark:border-white/7"><h2 class="section-title m-0">运行记录</h2><p class="mt-1 text-xs text-slate-400">状态、进度与失败诊断</p></div><div class="divide-y divide-slate-100 dark:divide-white/7"><div v-for="runItem in runs.slice(0,12)" :key="runItem.id" class="px-5 py-3.5"><div class="flex items-center"><span class="truncate text-xs font-semibold">{{ tasks.find(t => t.id === runItem.taskId)?.name || runItem.taskId }}</span><span class="ml-auto rounded-md px-2 py-1 text-[10px] font-semibold" :class="statusClass(runItem.status)">{{ runItem.status }}</span></div><div class="mt-2 flex items-center gap-2 text-[10px] text-slate-400"><span>{{ runItem.completedSteps }}/{{ runItem.totalSteps }} 步</span><span v-if="runItem.errorCode">{{ runItem.errorCode }}</span></div></div><div v-if="!runs.length" class="p-10 text-center text-xs text-slate-400">暂无运行记录</div></div></section>
  </div>
  <Teleport to="body"><div v-if="editorOpen" class="fixed inset-0 z-[80] flex justify-end bg-black/30 backdrop-blur-[2px]" @click.self="editorOpen=false"><section class="flex h-full w-full max-w-2xl flex-col border-l border-slate-200 bg-[#fbfcfb] shadow-2xl dark:border-white/9 dark:bg-[#0e1411]"><header class="flex h-16 items-center border-b border-slate-200 px-5 dark:border-white/9"><div><div class="eyebrow">JSON DSL</div><h2 class="mt-1 text-base font-semibold">任务定义</h2></div><button class="icon-button ml-auto" @click="editorOpen=false"><X :size="18" /></button></header><div class="flex-1 overflow-hidden p-5"><textarea v-model="editor" class="h-full w-full resize-none rounded-xl border border-slate-200 bg-white p-4 font-mono text-xs leading-6 outline-none focus:border-brand-500 dark:border-white/9 dark:bg-black/25" spellcheck="false" /></div><footer class="flex justify-end gap-2 border-t border-slate-200 p-4 dark:border-white/9"><button class="btn-secondary" @click="editorOpen=false">取消</button><button class="btn-primary" @click="save">校验并保存</button></footer></section></div></Teleport>
  <ConfirmDialog :open="!!deleting" destructive title="删除自动化任务" :description="`任务“${deleting?.name}”及其配置将被删除，历史运行记录会保留。`" confirm-text="删除任务" @cancel="deleting=null" @confirm="remove" />
</template>
