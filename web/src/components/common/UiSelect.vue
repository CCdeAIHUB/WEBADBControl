<script setup lang="ts">
import { ChevronDown } from 'lucide-vue-next'

defineProps<{ disabled?: boolean; ariaLabel?: string }>()
const model = defineModel<string | undefined>()
</script>

<template>
  <div class="relative">
    <select
      v-model="model"
      class="field appearance-none pr-9"
      :disabled="disabled"
      :aria-label="ariaLabel"
    >
      <slot />
    </select>
    <ChevronDown
      :size="15"
      aria-hidden="true"
      class="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-slate-400"
    />
  </div>
</template>
