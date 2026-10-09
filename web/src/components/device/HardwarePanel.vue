<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { Activity, Cpu, Database, Download, HardDrive, RefreshCw, Thermometer, X } from 'lucide-vue-next'
import { api, toAppError } from '@/services/api'
import { useUiStore } from '@/stores/ui'

interface Snapshot {
  capturedAt: string
  cpuFrequenciesKHz: number[]
  memoryTotalKb: number
  memoryAvailableKb: number
  storageTotalKb: number
  storageUsedKb: number
  temperaturesC: Record<string, number>
  uptimeSeconds: number
}

interface MonitorStatus {
  deviceId: string
  running: boolean
  startedAt?: string
  intervalMillis: number
  metrics: string[]
  sampleCount: number
  samples: Snapshot[]
  lastError?: string
  lastErrorCode?: string
}

const props = defineProps<{ deviceId: string }>()
const ui = useUiStore()
const samples = ref<Snapshot[]>([])
const loading = ref(false)
const monitor = ref<MonitorStatus | null>(null)
const monitorOpen = ref(false)
const selectedMetrics = ref(['cpu', 'memory', 'storage', 'temperature'])
const canvas = ref<HTMLCanvasElement>()
let timer = 0
let resizeObserver: ResizeObserver | undefined

const latest = computed(() => samples.value.at(-1))
const memoryUsage = computed(() => latest.value?.memoryTotalKb ? (latest.value.memoryTotalKb - latest.value.memoryAvailableKb) / latest.value.memoryTotalKb * 100 : 0)
const storageUsage = computed(() => latest.value?.storageTotalKb ? latest.value.storageUsedKb / latest.value.storageTotalKb * 100 : 0)
const averageCPU = computed(() => {
  const values = latest.value?.cpuFrequenciesKHz ?? []
  return values.length ? values.reduce((sum, value) => sum + value, 0) / values.length / 1_000_000 : 0
})
const maximumTemperature = computed(() => Math.max(0, ...Object.values(latest.value?.temperaturesC ?? {})))

async function collectOnce() {
  loading.value = true
  try {
    const snapshot = await api<Snapshot>(`/devices/${encodeURIComponent(props.deviceId)}/hardware`)
    samples.value = [...samples.value.slice(-59), snapshot]
    await nextTick(); draw()
  } catch (error) { ui.failure(toAppError(error)) }
  finally { loading.value = false }
}

async function refreshMonitor() {
  try {
    monitor.value = await api<MonitorStatus>(`/devices/${encodeURIComponent(props.deviceId)}/hardware-monitor`)
    if (monitor.value.samples.length) samples.value = monitor.value.samples.slice(-60)
    await nextTick(); draw()
  } catch (error) { ui.failure(toAppError(error)) }
}

async function toggleRecording() {
  loading.value = true
  try {
    const operation = monitor.value?.running ? 'stop' : 'start'
    monitor.value = await api<MonitorStatus>(`/devices/${encodeURIComponent(props.deviceId)}/hardware-monitor/${operation}`, {
      method: 'POST', body: JSON.stringify({ metrics: selectedMetrics.value }),
    })
    if (monitor.value.samples.length) samples.value = monitor.value.samples.slice(-60)
  } catch (error) { ui.failure(toAppError(error)) }
  finally { loading.value = false }
}

function startPolling() { stopPolling(); void refreshMonitor(); timer = window.setInterval(refreshMonitor, 3000) }
function stopPolling() { if (timer) window.clearInterval(timer); timer = 0 }

function draw() {
  const element = canvas.value
  if (!element) return
  const ratio = window.devicePixelRatio || 1
  const rect = element.getBoundingClientRect()
  element.width = Math.max(1, rect.width * ratio)
  element.height = Math.max(1, rect.height * ratio)
  const context = element.getContext('2d')
  if (!context) return
  context.scale(ratio, ratio)
  context.clearRect(0, 0, rect.width, rect.height)
  context.strokeStyle = document.documentElement.classList.contains('dark') ? 'rgba(255,255,255,.07)' : 'rgba(15,23,42,.07)'
  context.lineWidth = 1
  for (let row = 1; row < 4; row++) {
    context.beginPath(); context.moveTo(0, rect.height * row / 4); context.lineTo(rect.width, rect.height * row / 4); context.stroke()
  }
  const drawSeries = (values: number[], color: string) => {
    if (values.length < 2) return
    context.beginPath(); context.strokeStyle = color; context.lineWidth = 2
    values.forEach((value, index) => {
      const x = index / Math.max(1, values.length - 1) * rect.width
      const y = rect.height - Math.max(0, Math.min(100, value)) / 100 * rect.height
      if (index === 0) context.moveTo(x, y); else context.lineTo(x, y)
    })
    context.stroke()
  }
  drawSeries(samples.value.map(item => item.memoryTotalKb ? (item.memoryTotalKb - item.memoryAvailableKb) / item.memoryTotalKb * 100 : 0), '#22c55e')
  drawSeries(samples.value.map(item => Math.min(100, Math.max(0, ...Object.values(item.temperaturesC)) || 0)), '#f59e0b')
}

function formatStorage(valueKb = 0) { return valueKb ? `${(valueKb / 1024 / 1024).toFixed(1)} GB` : '—' }

function exportCSV() {
  const rows = ['capturedAt,cpuAverageGHz,memoryUsagePercent,storageUsagePercent,maxTemperatureC']
  for (const sample of samples.value) {
    const cpu = sample.cpuFrequenciesKHz.length ? sample.cpuFrequenciesKHz.reduce((sum,value)=>sum+value,0)/sample.cpuFrequenciesKHz.length/1_000_000 : 0
    const memory = sample.memoryTotalKb ? (sample.memoryTotalKb-sample.memoryAvailableKb)/sample.memoryTotalKb*100 : 0
    const storage = sample.storageTotalKb ? sample.storageUsedKb/sample.storageTotalKb*100 : 0
    rows.push(`${sample.capturedAt},${cpu.toFixed(3)},${memory.toFixed(2)},${storage.toFixed(2)},${Math.max(0,...Object.values(sample.temperaturesC)).toFixed(1)}`)
  }
  const url = URL.createObjectURL(new Blob([`\ufeff${rows.join('\n')}`], { type: 'text/csv;charset=utf-8' }))
  const link = document.createElement('a'); link.href = url; link.download = `hardware-${props.deviceId}-${Date.now()}.csv`; link.click(); URL.revokeObjectURL(url)
}

onMounted(async () => {
  await refreshMonitor()
  if (!monitor.value?.running && !monitor.value?.samples.length) await collectOnce()
  startPolling()
  if (canvas.value) { resizeObserver = new ResizeObserver(draw); resizeObserver.observe(canvas.value) }
})
// 仅停止页面状态轮询；后台记录归 Go 服务所有，离开页面绝不能隐式停止。
onBeforeUnmount(() => { stopPolling(); resizeObserver?.disconnect() })
</script>

<template>
  <div class="space-y-4">
    <section class="card flex flex-wrap items-center gap-3 p-5"><div><h3 class="section-title m-0">分类硬件信息</h3><p class="mt-1 text-xs text-slate-500 dark:text-slate-300">处理器、内存与存储、电池与温度分类展示；采样与记录在独立监控窗口中配置。</p></div><button class="btn-primary ml-auto" @click="monitorOpen = true"><Activity :size="16"/>打开硬件监控</button></section>
    <section v-if="monitor?.running" class="flex items-center gap-3 rounded-xl border border-amber-300 bg-amber-50 px-4 py-3 text-amber-900 shadow-sm dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-200">
      <span class="relative flex size-3"><span class="absolute inline-flex size-full animate-ping rounded-full bg-amber-500 opacity-60"/><span class="relative inline-flex size-3 rounded-full bg-amber-500"/></span>
      <div><p class="text-sm font-semibold">后台性能记录正在运行</p><p class="text-xs opacity-80">每 {{ Math.round(monitor.intervalMillis / 1000) }} 秒串行采样；离开本页后仍会继续，当前已记录 {{ monitor.sampleCount }} 条，只有手动停止才会结束。</p></div>
      <button class="btn-secondary ml-auto" :disabled="loading" @click="toggleRecording">停止记录</button>
    </section>
    <section class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <div v-for="metric in [
        { label:'CPU 平均频率', value:`${averageCPU.toFixed(2)} GHz`, icon:Cpu, color:'text-blue-600 bg-blue-50 dark:bg-blue-500/10' },
        { label:'内存占用', value:`${memoryUsage.toFixed(1)}%`, icon:Database, color:'text-brand-600 bg-brand-50 dark:bg-brand-500/10' },
        { label:'数据分区', value:`${storageUsage.toFixed(1)}%`, icon:HardDrive, color:'text-violet-600 bg-violet-50 dark:bg-violet-500/10' },
        { label:'最高温度', value:`${maximumTemperature.toFixed(1)} °C`, icon:Thermometer, color:'text-amber-600 bg-amber-50 dark:bg-amber-500/10' },
      ]" :key="metric.label" class="card p-4"><div class="flex items-center"><div class="grid size-8 place-items-center rounded-lg" :class="metric.color"><component :is="metric.icon" :size="16" /></div><span class="ml-2 text-xs text-slate-500">{{ metric.label }}</span></div><div class="mt-4 text-xl font-semibold">{{ metric.value }}</div></div>
    </section>
    <section v-if="latest" class="grid gap-4 xl:grid-cols-2"><div class="card p-5"><h3 class="section-title m-0">处理器核心</h3><div class="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-4"><div v-for="(frequency,index) in latest.cpuFrequenciesKHz" :key="index" class="rounded-lg bg-slate-50 p-3 dark:bg-white/4"><div class="text-[10px] text-slate-400">CPU {{ index }}</div><div class="mt-1 text-sm font-semibold">{{ (frequency/1_000_000).toFixed(2) }} GHz</div></div><div v-if="!latest.cpuFrequenciesKHz.length" class="text-xs text-slate-400">设备未公开核心频率</div></div></div><div class="card p-5"><h3 class="section-title m-0">资源明细</h3><dl class="mt-4 space-y-3 text-xs"><div class="flex"><dt class="text-slate-500">可用内存</dt><dd class="ml-auto font-medium">{{ formatStorage(latest.memoryAvailableKb) }}</dd></div><div class="flex"><dt class="text-slate-500">总内存</dt><dd class="ml-auto font-medium">{{ formatStorage(latest.memoryTotalKb) }}</dd></div><div class="flex"><dt class="text-slate-500">数据分区已用</dt><dd class="ml-auto font-medium">{{ formatStorage(latest.storageUsedKb) }} / {{ formatStorage(latest.storageTotalKb) }}</dd></div><div class="flex"><dt class="text-slate-500">运行时长</dt><dd class="ml-auto font-medium">{{ (latest.uptimeSeconds/3600).toFixed(1) }} 小时</dd></div></dl></div></section>
    <div v-if="monitorOpen" class="fixed inset-0 z-[90] grid place-items-center bg-slate-950/55 p-4 backdrop-blur-sm" @click.self="monitorOpen = false"><section class="card flex max-h-[90vh] w-full max-w-5xl flex-col overflow-hidden shadow-2xl"><div class="flex flex-wrap items-center gap-2 border-b border-slate-100 px-5 py-4 dark:border-white/7"><Activity :size="17" class="mr-1 text-brand-600" /><div><h3 class="section-title m-0">硬件监控</h3><p class="mt-1 text-xs text-slate-400">关闭窗口不会停止 Core 后台记录任务</p></div><button class="btn-primary ml-auto" :disabled="loading || !selectedMetrics.length" @click="toggleRecording">{{ monitor?.running ? '停止记录' : '开始后台记录' }}</button><button class="btn-secondary" :disabled="!samples.length" @click="exportCSV"><Download :size="15" />导出 CSV</button><button class="icon-button" title="关闭" @click="monitorOpen=false"><X :size="17"/></button></div><div class="flex flex-wrap gap-2 border-b border-slate-100 px-5 py-3 dark:border-white/7"><label v-for="metric in [{id:'cpu',label:'CPU'},{id:'memory',label:'内存'},{id:'storage',label:'存储'},{id:'temperature',label:'温度'}]" :key="metric.id" class="flex items-center gap-2 rounded-lg border border-slate-200 px-3 py-2 text-xs dark:border-white/10"><input v-model="selectedMetrics" type="checkbox" :value="metric.id" :disabled="monitor?.running" class="accent-brand-600"/>{{ metric.label }}</label></div><div class="min-h-0 flex-1 overflow-auto"><div v-if="loading && !samples.length" class="grid h-72 place-items-center text-sm text-slate-400"><RefreshCw :size="20" class="mb-2 animate-spin"/>正在读取硬件信息…</div><div v-else class="h-96 p-5"><canvas ref="canvas" class="size-full" aria-label="设备性能趋势图" /></div><p v-if="monitor?.lastError" class="border-t border-rose-200 bg-rose-50 px-5 py-3 text-xs text-rose-700 dark:border-rose-500/20 dark:bg-rose-500/10 dark:text-rose-300">最近一次采样失败：{{ monitor.lastError }}（{{ monitor.lastErrorCode }}），后台任务会在下一周期继续。</p></div></section></div>
  </div>
</template>
