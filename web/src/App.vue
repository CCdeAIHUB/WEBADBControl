<script setup lang="ts">
import AppShell from '@/components/layout/AppShell.vue'
import ToastStack from '@/components/feedback/ToastStack.vue'
</script>

<template>
  <AppShell>
    <RouterView />
  </AppShell>
  <ToastStack />
</template>
