<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { AlertTriangle, Bug, ClipboardList, FileSearch, RefreshCw, Search, ServerCrash } from 'lucide-vue-next'
import PageHeader from '@/components/common/PageHeader.vue'
import UiSelect from '@/components/common/UiSelect.vue'
import { getLogStats, listLogs, formatLogTime, logLevelClass, logTypeLabel } from '@/services/logs'
import { toAppError } from '@/services/api'
import { useUiStore } from '@/stores/ui'
import type { LogEvent, LogStats } from '@/types/api'

const ui = useUiStore()
const loading = ref(false)
const events = ref<LogEvent[]>([])
const stats = ref<LogStats | null>(null)
const selected = ref<LogEvent | null>(null)
const filters = ref({ type: '', level: '', q: '', traceId: '', errorCode: '', deviceId: '', limit: 200 })

const errorRate = computed(() => {
  if (!stats.value?.total) return '0%'
  return `${Math.round((stats.value.errorCount / stats.value.total) * 100)}%`
})

async function load() {
  loading.value = true
  try {
    const [nextEvents, nextStats] = await Promise.all([listLogs(filters.value), getLogStats()])
    events.value = nextEvents
    stats.value = nextStats
    selected.value = nextEvents[0] ?? null
  } catch (error) {
    ui.failure(toAppError(error))
  } finally {
    loading.value = false
  }
}

function details(event: LogEvent) {
  return JSON.stringify(event.details ?? {}, null, 2)
}

onMounted(load)
</script>

<template>
  <PageHeader eyebrow="Observability" title="系统日志" description="按 traceId、错误码、设备、模块和关键字查询请求、审计与前端错误。">
    <template #actions>
      <button class="btn-primary" :disabled="loading" @click="load"><RefreshCw :size="15" :class="loading ? 'animate-spin' : ''" />刷新</button>
    </template>
  </PageHeader>

  <div class="grid gap-4 lg:grid-cols-4">
    <section class="card p-5">
      <div class="flex items-center gap-3">
        <div class="grid size-9 place-items-center rounded-lg bg-brand-50 text-brand-700 dark:bg-brand-500/10 dark:text-brand-300"><ClipboardList :size="18" /></div>
        <div><div class="text-xs text-slate-400">总日志</div><div class="mt-1 text-2xl font-bold">{{ stats?.total ?? 0 }}</div></div>
      </div>
    </section>
    <section class="card p-5">
      <div class="flex items-center gap-3">
        <div class="grid size-9 place-items-center rounded-lg bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-300"><ServerCrash :size="18" /></div>
        <div><div class="text-xs text-slate-400">错误</div><div class="mt-1 text-2xl font-bold">{{ stats?.errorCount ?? 0 }}</div></div>
      </div>
    </section>
    <section class="card p-5">
      <div class="flex items-center gap-3">
        <div class="grid size-9 place-items-center rounded-lg bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300"><AlertTriangle :size="18" /></div>
        <div><div class="text-xs text-slate-400">警告</div><div class="mt-1 text-2xl font-bold">{{ stats?.warnCount ?? 0 }}</div></div>
      </div>
    </section>
    <section class="card p-5">
      <div class="flex items-center gap-3">
        <div class="grid size-9 place-items-center rounded-lg bg-sky-50 text-sky-700 dark:bg-sky-500/10 dark:text-sky-300"><Bug :size="18" /></div>
        <div><div class="text-xs text-slate-400">错误占比</div><div class="mt-1 text-2xl font-bold">{{ errorRate }}</div></div>
      </div>
    </section>
  </div>

  <section class="card mt-4 p-4">
    <div class="grid gap-3 md:grid-cols-3 xl:grid-cols-7">
      <label class="text-[11px] font-medium text-slate-500">类型<UiSelect v-model="filters.type" class="mt-1.5" aria-label="日志类型"><option value="">全部</option><option value="request">请求</option><option value="audit">审计</option><option value="client_error">前端错误</option><option value="system">系统</option></UiSelect></label>
      <label class="text-[11px] font-medium text-slate-500">级别<UiSelect v-model="filters.level" class="mt-1.5" aria-label="日志级别"><option value="">全部</option><option value="error">错误</option><option value="warn">警告</option><option value="info">信息</option></UiSelect></label>
      <label class="text-[11px] font-medium text-slate-500">Trace ID<input v-model="filters.traceId" class="field mt-1.5 font-mono text-xs" placeholder="traceId" /></label>
      <label class="text-[11px] font-medium text-slate-500">错误码<input v-model="filters.errorCode" class="field mt-1.5 font-mono text-xs" placeholder="errorCode" /></label>
      <label class="text-[11px] font-medium text-slate-500">设备 ID<input v-model="filters.deviceId" class="field mt-1.5 font-mono text-xs" placeholder="deviceId" /></label>
      <label class="text-[11px] font-medium text-slate-500 xl:col-span-2">关键字<div class="relative mt-1.5"><Search :size="15" class="pointer-events-none absolute top-2.5 left-3 text-slate-400" /><input v-model="filters.q" class="field pl-9" placeholder="搜索消息、路径、详情" @keyup.enter="load" /></div></label>
    </div>
    <div class="mt-3 flex justify-end"><button class="btn-secondary" @click="load"><FileSearch :size="15" />查询日志</button></div>
  </section>

  <div class="mt-4 grid gap-4 xl:grid-cols-[minmax(0,1fr)_420px]">
    <section class="card overflow-hidden">
      <div class="grid min-w-[980px] grid-cols-[150px_86px_100px_minmax(240px,1fr)_120px_120px_90px] border-b border-slate-100 bg-slate-50 px-4 py-2 text-[11px] font-semibold text-slate-400 dark:border-white/7 dark:bg-white/3">
        <span>时间</span><span>级别</span><span>类型</span><span>消息</span><span>错误码</span><span>Trace</span><span class="text-right">耗时</span>
      </div>
      <div class="max-h-[680px] overflow-auto">
        <button v-for="event in events" :key="event.id" class="grid min-w-[980px] grid-cols-[150px_86px_100px_minmax(240px,1fr)_120px_120px_90px] items-center border-b border-slate-100 px-4 py-3 text-left transition hover:bg-slate-50 dark:border-white/7 dark:hover:bg-white/3" :class="selected?.id === event.id ? 'bg-brand-50/70 dark:bg-brand-500/7' : ''" @click="selected = event">
          <span class="font-mono text-[11px] text-slate-400">{{ formatLogTime(event.createdAt) }}</span>
          <span><span class="rounded-full px-2 py-1 text-[10px] font-semibold" :class="logLevelClass(event.level)">{{ event.level }}</span></span>
          <span class="text-xs text-slate-500">{{ logTypeLabel(event.type) }}</span>
          <span class="min-w-0"><span class="block truncate text-sm font-semibold text-slate-800 dark:text-slate-100">{{ event.message }}</span><span class="mt-0.5 block truncate font-mono text-[10px] text-slate-400">{{ event.method }} {{ event.path || event.module }}</span></span>
          <span class="truncate font-mono text-[10px] text-red-500">{{ event.errorCode || '—' }}</span>
          <span class="truncate font-mono text-[10px] text-slate-400">{{ event.traceId }}</span>
          <span class="text-right font-mono text-[11px] text-slate-400">{{ event.durationMs ? `${event.durationMs}ms` : '—' }}</span>
        </button>
        <div v-if="!events.length && !loading" class="p-12 text-center text-xs text-slate-400">没有匹配的日志</div>
        <div v-if="loading" class="p-4 text-center text-xs text-slate-400">正在加载日志…</div>
      </div>
    </section>

    <aside class="card overflow-hidden">
      <div class="border-b border-slate-100 p-4 dark:border-white/7">
        <h2 class="section-title m-0">日志详情</h2>
        <p class="mt-1 text-xs text-slate-400">复制 traceId 或错误码即可继续定位</p>
      </div>
      <div v-if="selected" class="space-y-4 p-4">
        <div class="grid gap-3 text-xs">
          <div><span class="text-slate-400">消息</span><div class="mt-1 font-semibold">{{ selected.message }}</div></div>
          <div><span class="text-slate-400">Trace ID</span><div class="mt-1 break-all font-mono text-[11px]">{{ selected.traceId }}</div></div>
          <div><span class="text-slate-400">模块 / 动作</span><div class="mt-1 font-mono text-[11px]">{{ selected.module }} · {{ selected.action }}</div></div>
          <div v-if="selected.deviceId"><span class="text-slate-400">设备</span><div class="mt-1 break-all font-mono text-[11px]">{{ selected.deviceId }}</div></div>
          <div v-if="selected.errorCode"><span class="text-slate-400">错误码</span><div class="mt-1 font-mono text-[11px] text-red-500">{{ selected.errorCode }}</div></div>
          <div><span class="text-slate-400">请求</span><div class="mt-1 break-all font-mono text-[11px]">{{ selected.method || '—' }} {{ selected.path || '' }} · {{ selected.status || '—' }}</div></div>
          <div><span class="text-slate-400">来源</span><div class="mt-1 break-all font-mono text-[11px]">{{ selected.actor || '—' }} · {{ selected.ipAddress || '—' }}</div></div>
        </div>
        <pre class="max-h-72 overflow-auto rounded-lg bg-slate-950 p-3 text-[11px] leading-5 text-slate-100">{{ details(selected) }}</pre>
      </div>
      <div v-else class="grid min-h-72 place-items-center text-center text-xs text-slate-400">选择一条日志查看详情</div>
    </aside>
  </div>
</template>
