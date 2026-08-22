<script setup lang="ts">
import { Check } from 'lucide-vue-next'

defineProps<{ label: string; disabled?: boolean }>()
const model = defineModel<boolean>({ required: true })
</script>

<template>
  <button
    type="button"
    role="switch"
    :aria-checked="model"
    :aria-label="label"
    :disabled="disabled"
    class="group inline-flex h-10 items-center gap-2.5 rounded-lg px-1 text-xs font-medium text-slate-600 transition focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-brand-500/25 disabled:opacity-45 dark:text-slate-300"
    @click="model = !model"
  >
    <span
      class="relative inline-flex h-5 w-9 shrink-0 items-center rounded-full border transition"
      :class="model ? 'border-brand-600 bg-brand-600' : 'border-slate-300 bg-slate-200 dark:border-white/15 dark:bg-white/10'"
    >
      <span
        class="grid size-4 place-items-center rounded-full bg-white text-brand-600 shadow-sm transition-transform"
        :class="model ? 'translate-x-4' : 'translate-x-0.5'"
      ><Check v-if="model" :size="10" :stroke-width="3" /></span>
    </span>
    <span>{{ label }}</span>
  </button>
</template>
