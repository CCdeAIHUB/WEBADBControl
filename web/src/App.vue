<script setup lang="ts">
import AppShell from '@/components/layout/AppShell.vue'
import ToastStack from '@/components/feedback/ToastStack.vue'
</script>

<template>
  <AppShell>
    <RouterView v-slot="{ Component }">
      <Transition name="page" mode="out-in"><component :is="Component" /></Transition>
    </RouterView>
  </AppShell>
  <ToastStack />
</template>
