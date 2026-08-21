<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { Activity, Cpu, Database, Download, HardDrive, RefreshCw, Thermometer } from 'lucide-vue-next'
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

const props = defineProps<{ deviceId: string }>()
const ui = useUiStore()
const samples = ref<Snapshot[]>([])
const loading = ref(false)
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

async function collect() {
  loading.value = true
  try {
    const snapshot = await api<Snapshot>(`/devices/${encodeURIComponent(props.deviceId)}/hardware`)
    samples.value = [...samples.value.slice(-59), snapshot]
    await nextTick(); draw()
  } catch (error) { ui.failure(toAppError(error)); stop() }
  finally { loading.value = false }
}

function start() { stop(); void collect(); timer = window.setInterval(collect, 3000) }
function stop() { if (timer) window.clearInterval(timer); timer = 0 }

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

onMounted(() => { start(); if (canvas.value) { resizeObserver = new ResizeObserver(draw); resizeObserver.observe(canvas.value) } })
onBeforeUnmount(() => { stop(); resizeObserver?.disconnect() })
</script>

<template>
  <div class="space-y-4">
    <section class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <div v-for="metric in [
        { label:'CPU 平均频率', value:`${averageCPU.toFixed(2)} GHz`, icon:Cpu, color:'text-blue-600 bg-blue-50 dark:bg-blue-500/10' },
        { label:'内存占用', value:`${memoryUsage.toFixed(1)}%`, icon:Database, color:'text-brand-600 bg-brand-50 dark:bg-brand-500/10' },
        { label:'数据分区', value:`${storageUsage.toFixed(1)}%`, icon:HardDrive, color:'text-violet-600 bg-violet-50 dark:bg-violet-500/10' },
        { label:'最高温度', value:`${maximumTemperature.toFixed(1)} °C`, icon:Thermometer, color:'text-amber-600 bg-amber-50 dark:bg-amber-500/10' },
      ]" :key="metric.label" class="card p-4"><div class="flex items-center"><div class="grid size-8 place-items-center rounded-lg" :class="metric.color"><component :is="metric.icon" :size="16" /></div><span class="ml-2 text-xs text-slate-500">{{ metric.label }}</span></div><div class="mt-4 text-xl font-semibold">{{ metric.value }}</div></div>
    </section>
    <section class="card overflow-hidden"><div class="flex items-center border-b border-slate-100 px-5 py-4 dark:border-white/7"><Activity :size="17" class="mr-2 text-brand-600" /><div><h3 class="section-title m-0">实时性能趋势</h3><p class="mt-1 text-xs text-slate-400">每 3 秒采集 · 绿色为内存占用 · 橙色为最高温度</p></div><button class="btn-secondary ml-auto" :disabled="!samples.length" @click="exportCSV"><Download :size="15" />导出 CSV</button><button class="icon-button ml-1" title="立即刷新" @click="collect"><RefreshCw :size="16" :class="loading ? 'animate-spin' : ''" /></button></div><div class="h-72 p-5"><canvas ref="canvas" class="size-full" aria-label="设备性能趋势图" /></div></section>
    <section v-if="latest" class="grid gap-4 xl:grid-cols-2"><div class="card p-5"><h3 class="section-title m-0">处理器核心</h3><div class="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-4"><div v-for="(frequency,index) in latest.cpuFrequenciesKHz" :key="index" class="rounded-lg bg-slate-50 p-3 dark:bg-white/4"><div class="text-[10px] text-slate-400">CPU {{ index }}</div><div class="mt-1 text-sm font-semibold">{{ (frequency/1_000_000).toFixed(2) }} GHz</div></div><div v-if="!latest.cpuFrequenciesKHz.length" class="text-xs text-slate-400">设备未公开核心频率</div></div></div><div class="card p-5"><h3 class="section-title m-0">资源明细</h3><dl class="mt-4 space-y-3 text-xs"><div class="flex"><dt class="text-slate-500">可用内存</dt><dd class="ml-auto font-medium">{{ formatStorage(latest.memoryAvailableKb) }}</dd></div><div class="flex"><dt class="text-slate-500">总内存</dt><dd class="ml-auto font-medium">{{ formatStorage(latest.memoryTotalKb) }}</dd></div><div class="flex"><dt class="text-slate-500">数据分区已用</dt><dd class="ml-auto font-medium">{{ formatStorage(latest.storageUsedKb) }} / {{ formatStorage(latest.storageTotalKb) }}</dd></div><div class="flex"><dt class="text-slate-500">运行时长</dt><dd class="ml-auto font-medium">{{ (latest.uptimeSeconds/3600).toFixed(1) }} 小时</dd></div></dl></div></section>
  </div>
</template>
