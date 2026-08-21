<script setup lang="ts">
defineProps<{ eyebrow?: string; title: string; description?: string }>()
</script>

<template>
  <div class="mb-6 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
    <div><div v-if="eyebrow" class="eyebrow mb-1.5">{{ eyebrow }}</div><h1 class="m-0 text-[26px] font-bold tracking-tight text-slate-950 dark:text-white">{{ title }}</h1><p v-if="description" class="mt-2 max-w-2xl text-sm leading-6 text-slate-500 dark:text-slate-400">{{ description }}</p></div>
    <div v-if="$slots.actions" class="flex shrink-0 items-center gap-2"><slot name="actions" /></div>
  </div>
</template>
