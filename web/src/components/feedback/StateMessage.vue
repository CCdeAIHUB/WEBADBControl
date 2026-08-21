<script setup lang="ts">
import { AlertTriangle, Inbox, LoaderCircle } from 'lucide-vue-next'
import type { AppError } from '@/types/api'

defineProps<{ state: 'loading' | 'empty' | 'error'; title?: string; description?: string; error?: AppError | null }>()
defineEmits<{ retry: [] }>()
</script>

<template>
  <div class="card flex min-h-56 flex-col items-center justify-center p-8 text-center">
    <LoaderCircle v-if="state === 'loading'" :size="28" class="mb-4 animate-spin text-brand-600" />
    <AlertTriangle v-else-if="state === 'error'" :size="28" class="mb-4 text-amber-500" />
    <Inbox v-else :size="28" class="mb-4 text-slate-400" />
    <h3 class="m-0 text-sm font-semibold text-slate-900 dark:text-white">{{ title ?? (state === 'loading' ? '正在加载' : state === 'error' ? error?.message ?? '加载失败' : '暂无内容') }}</h3>
    <p class="mt-2 max-w-md text-xs leading-5 text-slate-500 dark:text-slate-400">{{ description ?? error?.suggestion ?? (error ? `错误码：${error.errorCode} · ${error.traceId}` : '连接设备后，内容会显示在这里。') }}</p>
    <button v-if="state === 'error' && error?.recoverable" class="btn-secondary mt-4" @click="$emit('retry')">重新加载</button>
  </div>
</template>
