<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ArrowRight, Bot, CheckCircle2, CircleDot, Clock3, Smartphone, Workflow, Zap } from 'lucide-vue-next'
import PageHeader from '@/components/common/PageHeader.vue'
import StateMessage from '@/components/feedback/StateMessage.vue'
import { api } from '@/services/api'
import { useDevicesStore } from '@/stores/devices'
import type { AutomationRun } from '@/types/api'

const devices = useDevicesStore()
const runs = ref<AutomationRun[]>([])
const taskCount = ref(0)
const now = new Date()
const greeting = computed(() => now.getHours() < 12 ? '早上好' : now.getHours() < 18 ? '下午好' : '晚上好')

onMounted(async () => {
  await devices.refresh()
  try {
    const overview = await api<{ taskCount: number; recentRuns: AutomationRun[] }>('/overview')
    taskCount.value = overview.taskCount
    runs.value = overview.recentRuns
  } catch { /* Device store already presents the primary service error. */ }
})

function runTone(status: AutomationRun['status']) {
  return status === 'succeeded' ? 'text-brand-600 bg-brand-50 dark:bg-brand-500/10' : status === 'failed' ? 'text-red-600 bg-red-50 dark:bg-red-500/10' : 'text-amber-600 bg-amber-50 dark:bg-amber-500/10'
}
</script>

<template>
  <PageHeader eyebrow="Overview" :title="`${greeting}，欢迎回来`" description="集中查看设备状态、自动化任务与最近活动。">
    <template #actions><RouterLink to="/devices" class="btn-primary"><Zap :size="16" />开始管理</RouterLink></template>
  </PageHeader>

  <StateMessage v-if="devices.status === 'loading'" state="loading" title="正在连接设备核心" />
  <StateMessage v-else-if="devices.status === 'error'" state="error" :error="devices.error" @retry="devices.refresh()" />
  <template v-else>
    <section class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <div class="card p-5"><div class="flex items-center justify-between"><span class="text-xs font-medium text-slate-500">设备总数</span><div class="grid size-8 place-items-center rounded-lg bg-brand-50 text-brand-600 dark:bg-brand-500/10"><Smartphone :size="17" /></div></div><div class="mt-4 text-3xl font-bold tracking-tight">{{ devices.devices.length }}</div><div class="mt-1.5 text-xs text-slate-400">{{ devices.online.length }} 台在线可用</div></div>
      <div class="card p-5"><div class="flex items-center justify-between"><span class="text-xs font-medium text-slate-500">自动化任务</span><div class="grid size-8 place-items-center rounded-lg bg-blue-50 text-blue-600 dark:bg-blue-500/10"><Workflow :size="17" /></div></div><div class="mt-4 text-3xl font-bold tracking-tight">{{ taskCount }}</div><div class="mt-1.5 text-xs text-slate-400">任务由服务端持续调度</div></div>
      <div class="card p-5"><div class="flex items-center justify-between"><span class="text-xs font-medium text-slate-500">成功运行</span><div class="grid size-8 place-items-center rounded-lg bg-emerald-50 text-emerald-600 dark:bg-emerald-500/10"><CheckCircle2 :size="17" /></div></div><div class="mt-4 text-3xl font-bold tracking-tight">{{ runs.filter(r => r.status === 'succeeded').length }}</div><div class="mt-1.5 text-xs text-slate-400">最近 50 条运行记录</div></div>
      <div class="card p-5"><div class="flex items-center justify-between"><span class="text-xs font-medium text-slate-500">服务状态</span><div class="grid size-8 place-items-center rounded-lg bg-violet-50 text-violet-600 dark:bg-violet-500/10"><CircleDot :size="17" /></div></div><div class="mt-4 text-lg font-bold tracking-tight text-brand-700 dark:text-brand-400">运行正常</div><div class="mt-2 text-xs text-slate-400">Core 与 Web 服务已就绪</div></div>
    </section>

    <section class="mt-5 grid gap-5 xl:grid-cols-[1.4fr_.8fr]">
      <div class="card overflow-hidden">
        <div class="flex items-center border-b border-slate-100 px-5 py-4 dark:border-white/7"><div><h2 class="section-title m-0">我的设备</h2><p class="mt-1 text-xs text-slate-400">快速进入常用设备</p></div><RouterLink to="/devices" class="ml-auto flex items-center gap-1 text-xs font-semibold text-brand-700 hover:text-brand-600 dark:text-brand-400">查看全部<ArrowRight :size="14" /></RouterLink></div>
        <div v-if="devices.devices.length" class="divide-y divide-slate-100 dark:divide-white/7">
          <RouterLink v-for="device in devices.devices.slice(0, 5)" :key="device.id" :to="`/devices/${encodeURIComponent(device.id)}`" class="flex items-center gap-3 px-5 py-3.5 transition hover:bg-slate-50 dark:hover:bg-white/3">
            <div class="grid size-10 place-items-center rounded-lg bg-slate-100 text-slate-600 dark:bg-white/6 dark:text-slate-300"><Smartphone :size="19" /></div>
            <div class="min-w-0"><div class="truncate text-sm font-semibold">{{ device.name }}</div><div class="mt-0.5 truncate font-mono text-[10px] text-slate-400">{{ device.id }}</div></div>
            <div class="ml-auto flex items-center gap-1.5 text-xs" :class="['device','online'].includes(device.state) ? 'text-brand-600' : 'text-slate-400'"><span class="size-1.5 rounded-full bg-current" />{{ ['device','online'].includes(device.state) ? '在线' : device.state }}</div>
          </RouterLink>
        </div>
        <div v-else class="p-8 text-center text-sm text-slate-400">尚未发现设备</div>
      </div>

      <div class="card overflow-hidden">
        <div class="border-b border-slate-100 px-5 py-4 dark:border-white/7"><h2 class="section-title m-0">最近运行</h2><p class="mt-1 text-xs text-slate-400">自动化任务状态</p></div>
        <div v-if="runs.length" class="divide-y divide-slate-100 dark:divide-white/7">
          <div v-for="run in runs.slice(0, 5)" :key="run.id" class="flex items-center gap-3 px-5 py-3.5"><div class="grid size-8 place-items-center rounded-lg" :class="runTone(run.status)"><Clock3 :size="15" /></div><div class="min-w-0 flex-1"><div class="truncate text-xs font-semibold">{{ run.taskId }}</div><div class="mt-1 h-1 overflow-hidden rounded-full bg-slate-100 dark:bg-white/8"><div class="h-full bg-brand-500" :style="{ width: `${run.progress * 100}%` }" /></div></div><span class="text-[10px] font-medium text-slate-400">{{ run.status }}</span></div>
        </div>
        <div v-else class="grid min-h-48 place-items-center p-6 text-center"><div><Bot :size="24" class="mx-auto mb-3 text-slate-300" /><p class="m-0 text-xs text-slate-400">暂无运行记录</p></div></div>
      </div>
    </section>
  </template>
</template>
