<script setup lang="ts">
import { AlertTriangle, X } from 'lucide-vue-next'

defineProps<{ open: boolean; title: string; description: string; confirmText?: string; destructive?: boolean }>()
defineEmits<{ confirm: []; cancel: [] }>()
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="fixed inset-0 z-[90] grid place-items-center bg-black/35 p-4 backdrop-blur-[2px]" @click.self="$emit('cancel')">
      <section class="surface w-full max-w-md rounded-xl p-5 shadow-2xl" role="alertdialog" aria-modal="true">
        <div class="flex items-start gap-3">
          <div class="grid size-9 shrink-0 place-items-center rounded-lg" :class="destructive ? 'bg-red-50 text-red-600 dark:bg-red-500/10' : 'bg-amber-50 text-amber-600 dark:bg-amber-500/10'"><AlertTriangle :size="19" /></div>
          <div class="min-w-0 flex-1"><h2 class="m-0 text-base font-semibold">{{ title }}</h2><p class="mt-2 text-sm leading-6 text-slate-500 dark:text-slate-400">{{ description }}</p></div>
          <button class="icon-button -mt-1 -mr-1" aria-label="关闭" @click="$emit('cancel')"><X :size="17" /></button>
        </div>
        <div class="mt-6 flex justify-end gap-2"><button class="btn-secondary" @click="$emit('cancel')">取消</button><button :class="destructive ? 'btn-danger' : 'btn-primary'" @click="$emit('confirm')">{{ confirmText ?? '确认继续' }}</button></div>
      </section>
    </div>
  </Teleport>
</template>
