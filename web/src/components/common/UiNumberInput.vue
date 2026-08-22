<script setup lang="ts">
import { Minus, Plus } from 'lucide-vue-next'

const props = withDefaults(defineProps<{ min?: number; max?: number; step?: number; ariaLabel?: string }>(), { step: 1 })
const model = defineModel<number>({ required: true })

function clamp(value: number): number {
  if (!Number.isFinite(value)) return props.min ?? 0
  return Math.min(props.max ?? Number.POSITIVE_INFINITY, Math.max(props.min ?? Number.NEGATIVE_INFINITY, value))
}

function setValue(value: number) {
  model.value = clamp(value)
}

function onInput(event: Event) {
  const value = Number((event.target as HTMLInputElement).value)
  if (Number.isFinite(value)) model.value = value
}
</script>

<template>
  <div class="flex h-10 w-full items-center overflow-hidden rounded-lg border border-slate-200 bg-white text-slate-800 transition hover:border-slate-300 focus-within:border-brand-500 focus-within:ring-3 focus-within:ring-brand-500/12 dark:border-white/10 dark:bg-white/4 dark:text-slate-100">
    <button type="button" class="grid h-full w-9 shrink-0 place-items-center border-r border-slate-100 text-slate-400 transition hover:bg-slate-50 hover:text-slate-700 focus-visible:outline-none dark:border-white/8 dark:hover:bg-white/6 dark:hover:text-white" :disabled="model <= (min ?? Number.NEGATIVE_INFINITY)" :aria-label="`${ariaLabel ?? '数值'}减小`" @click="setValue(model - step)"><Minus :size="14" /></button>
    <input :value="model" type="text" inputmode="numeric" class="h-full min-w-0 flex-1 appearance-none border-0 bg-transparent px-2 text-center text-sm outline-none" :aria-label="ariaLabel" @input="onInput" @blur="setValue(model)" />
    <button type="button" class="grid h-full w-9 shrink-0 place-items-center border-l border-slate-100 text-slate-400 transition hover:bg-slate-50 hover:text-slate-700 focus-visible:outline-none dark:border-white/8 dark:hover:bg-white/6 dark:hover:text-white" :disabled="model >= (max ?? Number.POSITIVE_INFINITY)" :aria-label="`${ariaLabel ?? '数值'}增大`" @click="setValue(model + step)"><Plus :size="14" /></button>
  </div>
</template>
