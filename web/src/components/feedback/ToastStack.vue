<script setup lang="ts">
import { AlertCircle, CheckCircle2, Info, X } from 'lucide-vue-next'
import { useUiStore } from '@/stores/ui'

const ui = useUiStore()
const icons = { success: CheckCircle2, warning: AlertCircle, danger: AlertCircle, info: Info }
</script>

<template>
  <div class="pointer-events-none fixed right-4 top-4 z-[100] flex w-[min(380px,calc(100vw-2rem))] flex-col gap-2" aria-live="polite">
    <TransitionGroup name="toast">
      <div v-for="toast in ui.toasts" :key="toast.id" class="surface pointer-events-auto flex gap-3 rounded-xl p-3.5 shadow-lg">
        <component :is="icons[toast.tone]" :size="18" class="mt-0.5 shrink-0" :class="toast.tone === 'danger' ? 'text-red-500' : toast.tone === 'success' ? 'text-brand-600' : 'text-amber-500'" />
        <div class="min-w-0 flex-1"><div class="text-sm font-semibold">{{ toast.title }}</div><div class="mt-0.5 text-xs leading-5 text-slate-500 dark:text-slate-400">{{ toast.message }}</div></div>
        <button class="text-slate-400 hover:text-slate-700 dark:hover:text-white" aria-label="关闭通知" @click="ui.dismiss(toast.id)"><X :size="16" /></button>
      </div>
    </TransitionGroup>
  </div>
</template>

<style scoped>
.toast-enter-active,.toast-leave-active { transition: .2s ease; }
.toast-enter-from,.toast-leave-to { opacity: 0; transform: translateY(-8px); }
</style>
