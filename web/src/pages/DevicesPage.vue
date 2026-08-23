<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Cable, Plus, RefreshCw, Search, Smartphone, Wifi } from 'lucide-vue-next'
import PageHeader from '@/components/common/PageHeader.vue'
import DeviceConnectionDialog from '@/components/device/DeviceConnectionDialog.vue'
import StateMessage from '@/components/feedback/StateMessage.vue'
import { useDevicesStore } from '@/stores/devices'

const devices = useDevicesStore()
const query = ref('')
const connectOpen = ref(false)

onMounted(() => devices.refresh())

</script>

<template>
  <PageHeader eyebrow="Devices" title="设备中心" description="统一管理 USB、无线 ADB 与 Android Companion 设备。">
    <template #actions><button class="btn-secondary" @click="devices.refresh()"><RefreshCw :size="15" />刷新</button><button class="btn-primary" @click="connectOpen = true"><Plus :size="16" />连接设备</button></template>
  </PageHeader>
  <div class="mb-4 flex max-w-md items-center gap-2 rounded-lg border border-slate-200 bg-white px-3 dark:border-white/9 dark:bg-white/4"><Search :size="16" class="text-slate-400" /><input v-model="query" class="h-10 min-w-0 flex-1 border-0 bg-transparent text-sm outline-none" placeholder="搜索名称、型号或设备序列号" /></div>
  <StateMessage v-if="devices.status === 'loading'" state="loading" title="正在发现设备" />
  <StateMessage v-else-if="devices.status === 'error'" state="error" :error="devices.error" @retry="devices.refresh()" />
  <StateMessage v-else-if="!devices.devices.length" state="empty" title="还没有连接设备" description="使用 USB 调试连接设备，或输入无线 ADB 地址建立连接。" />
  <section v-else class="grid gap-4 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
    <RouterLink v-for="device in devices.devices.filter(d => `${d.name} ${d.model} ${d.id}`.toLowerCase().includes(query.toLowerCase()))" :key="device.id" :to="`/devices/${encodeURIComponent(device.id)}`" class="card group overflow-hidden p-5 transition hover:-translate-y-0.5 hover:border-brand-200 hover:shadow-md dark:hover:border-brand-500/25">
      <div class="flex items-start"><div class="grid size-11 place-items-center rounded-xl bg-slate-100 text-slate-600 group-hover:bg-brand-50 group-hover:text-brand-700 dark:bg-white/6 dark:text-slate-300 dark:group-hover:bg-brand-500/10 dark:group-hover:text-brand-300"><Smartphone :size="21" /></div><div class="ml-auto flex items-center gap-1.5 text-[11px] font-medium" :class="['device','online'].includes(device.state) ? 'text-brand-600' : 'text-slate-400'"><span class="size-1.5 rounded-full bg-current" />{{ ['device','online'].includes(device.state) ? '在线' : device.state }}</div></div>
      <h2 class="mt-5 mb-1 truncate text-[15px] font-semibold text-slate-900 dark:text-white">{{ device.name }}</h2><p class="m-0 truncate font-mono text-[10px] text-slate-400">{{ device.id }}</p>
      <div class="mt-4 flex items-center justify-between border-t border-slate-100 pt-3 text-xs text-slate-500 dark:border-white/7 dark:text-slate-400"><span class="flex items-center gap-1.5"><Wifi v-if="device.transport === 'wireless'" :size="14" /><Cable v-else :size="14" />{{ device.transport === 'wireless' ? '无线 ADB' : device.transport === 'companion' ? '伴侣应用' : 'USB 调试' }}</span><span>{{ device.model || device.product || 'Android' }}</span></div>
    </RouterLink>
  </section>

  <DeviceConnectionDialog :open="connectOpen" @close="connectOpen = false" />
</template>
