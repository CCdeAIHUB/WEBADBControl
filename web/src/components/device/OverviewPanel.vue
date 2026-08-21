<script setup lang="ts">
import { BatteryCharging, Box, Cpu, HardDrive, Info, MemoryStick, Radio } from 'lucide-vue-next'
import type { DeviceOverview } from '@/types/api'

const props = defineProps<{ overview: DeviceOverview }>()
const property = (key: string, fallback = '—') => props.overview.properties[key] || fallback
</script>

<template>
  <div class="space-y-4">
    <section class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <div v-for="metric in [
        { label: '设备型号', value: property('ro.product.model'), icon: Box },
        { label: 'Android 版本', value: property('ro.build.version.release'), icon: Info },
        { label: '电池电量', value: `${overview.battery.level ?? '—'}%`, icon: BatteryCharging },
        { label: 'SoC 平台', value: property('ro.soc.model', property('ro.board.platform')), icon: Cpu },
      ]" :key="metric.label" class="card p-4"><div class="flex items-center gap-2 text-xs text-slate-500"><component :is="metric.icon" :size="15" class="text-brand-600" />{{ metric.label }}</div><div class="mt-3 truncate text-lg font-semibold">{{ metric.value }}</div></div>
    </section>
    <section class="grid gap-4 xl:grid-cols-2">
      <div class="card p-5"><h3 class="section-title m-0 flex items-center gap-2"><HardDrive :size="17" class="text-brand-600" />系统信息</h3><dl class="mt-4 grid gap-3 text-sm"><div v-for="[label, value] in [['制造商', property('ro.product.manufacturer')], ['产品名称', property('ro.product.name')], ['设备代号', property('ro.product.device')], ['构建版本', property('ro.build.display.id')], ['安全补丁', property('ro.build.version.security_patch')]]" :key="label" class="flex border-b border-slate-100 pb-2.5 last:border-0 dark:border-white/7"><dt class="text-slate-500">{{ label }}</dt><dd class="ml-auto max-w-[65%] truncate font-mono text-xs text-slate-800 dark:text-slate-200">{{ value }}</dd></div></dl></div>
      <div class="card p-5"><h3 class="section-title m-0 flex items-center gap-2"><MemoryStick :size="17" class="text-brand-600" />连接与硬件</h3><dl class="mt-4 grid gap-3 text-sm"><div v-for="[label, value] in [['CPU ABI', property('ro.product.cpu.abi')], ['硬件平台', property('ro.hardware')], ['序列号', property('ro.serialno', overview.deviceId)], ['启动完成', property('sys.boot_completed') === '1' ? '是' : '否'], ['电池状态', String(overview.battery.status ?? '—')]]" :key="label" class="flex border-b border-slate-100 pb-2.5 last:border-0 dark:border-white/7"><dt class="text-slate-500">{{ label }}</dt><dd class="ml-auto max-w-[65%] truncate font-mono text-xs text-slate-800 dark:text-slate-200">{{ value }}</dd></div></dl></div>
    </section>
    <div class="card flex items-center gap-3 px-4 py-3 text-xs text-slate-500 dark:text-slate-400"><Radio :size="15" class="text-brand-500" />数据采集于 {{ new Date(overview.collectedAt).toLocaleString('zh-CN') }}，刷新页面可获取最新状态。</div>
  </div>
</template>
