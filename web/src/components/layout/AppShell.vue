<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Bot, Boxes, ChevronLeft, ClipboardList, LayoutDashboard, LogOut, Menu, Settings, Smartphone,
  Workflow, X, Zap,
} from 'lucide-vue-next'
import { logout } from '@/services/auth'
import { api, toAppError } from '@/services/api'
import { synchronizeTheme } from '@/services/theme'
import { useUiStore } from '@/stores/ui'
import type { AppSettings } from '@/types/api'

const route = useRoute()
const router = useRouter()
const ui = useUiStore()
const isPublic = computed(() => route.meta.public === true)
let themeSynchronized = false

watch(() => route.name, async () => {
  if (!route.name || isPublic.value || themeSynchronized) return
  try {
    const settings = await api<AppSettings>('/settings')
    synchronizeTheme(settings.theme)
    themeSynchronized = true
  } catch (error) {
    ui.failure(toAppError(error))
  }
}, { immediate: true })

async function signOut() {
  try { await logout() } finally { await router.replace('/login') }
}

const navigation = [
	{ to: '/', label: '工作台', icon: LayoutDashboard },
	{ to: '/devices', label: '设备中心', icon: Smartphone },
	{ to: '/automation', label: '自动化任务', icon: Workflow },
	{ to: '/assistant', label: 'AI 助手', icon: Bot },
	{ to: '/logs', label: '系统日志', icon: ClipboardList },
	{ to: '/settings', label: '系统设置', icon: Settings },
]
</script>

<template>
  <div v-if="isPublic" class="min-h-screen"><slot /></div>
  <div v-else class="min-h-screen bg-[#f4f7f5] text-slate-800 dark:bg-[#0b100e] dark:text-slate-100">
    <div v-if="ui.sidebarOpen" class="fixed inset-0 z-40 bg-black/30 backdrop-blur-[2px] lg:hidden" @click="ui.sidebarOpen = false" />
    <aside
      class="fixed inset-y-0 left-0 z-50 flex w-[244px] flex-col border-r border-slate-200/80 bg-[#fbfcfb] transition-transform duration-200 dark:border-white/8 dark:bg-[#0e1411] lg:translate-x-0"
      :class="ui.sidebarOpen ? 'translate-x-0' : '-translate-x-full'"
    >
      <div class="flex h-17 items-center gap-3 px-5">
        <div class="grid size-9 place-items-center rounded-lg bg-brand-600 text-white shadow-[0_6px_18px_rgba(22,163,74,.2)]">
          <Zap :size="19" :stroke-width="2.3" />
        </div>
        <div class="min-w-0">
          <div class="truncate text-[15px] font-bold tracking-tight text-slate-900 dark:text-white">ADBControl</div>
          <div class="text-[10px] font-semibold tracking-[.16em] text-slate-400 uppercase">Web Console</div>
        </div>
        <button class="icon-button ml-auto lg:hidden" aria-label="关闭导航" @click="ui.sidebarOpen = false"><X :size="18" /></button>
      </div>

      <nav class="flex-1 space-y-1 px-3 py-5" aria-label="主导航">
        <div class="mb-2 px-3 text-[10px] font-semibold tracking-[.15em] text-slate-400 uppercase">管理中心</div>
        <RouterLink
		v-for="item in navigation" :key="item.to" :to="item.to"
          class="group flex h-10 items-center gap-3 rounded-lg px-3 text-sm font-medium text-slate-600 transition hover:bg-slate-100 hover:text-slate-950 dark:text-slate-400 dark:hover:bg-white/5 dark:hover:text-white"
          :class="route.path === item.to || (item.to !== '/' && route.path.startsWith(item.to)) ? '!bg-brand-50 !text-brand-700 dark:!bg-brand-500/10 dark:!text-brand-300' : ''"
          @click="ui.sidebarOpen = false"
        >
          <component :is="item.icon" :size="18" :stroke-width="1.9" />
          <span>{{ item.label }}</span>
          <ChevronLeft v-if="route.path === item.to || (item.to !== '/' && route.path.startsWith(item.to))" :size="14" class="ml-auto rotate-180" />
        </RouterLink>
      </nav>

      <div class="p-3">
        <div class="mb-3 rounded-lg border border-brand-100 bg-brand-50/70 p-3 dark:border-brand-500/15 dark:bg-brand-500/7">
          <div class="mb-1 flex items-center gap-2 text-xs font-semibold text-brand-800 dark:text-brand-300"><Boxes :size="14" /> 原生核心已连接</div>
          <p class="m-0 text-[11px] leading-5 text-brand-700/75 dark:text-brand-300/65">Rust Core · Go Service · Web UI</p>
        </div>
        <button class="flex h-10 w-full items-center gap-3 rounded-lg px-3 text-sm font-medium text-slate-500 hover:bg-red-50 hover:text-red-600 dark:text-slate-400 dark:hover:bg-red-500/10 dark:hover:text-red-300" @click="signOut">
          <LogOut :size="18" />退出登录
        </button>
      </div>
    </aside>

    <div class="lg:pl-[244px]">
      <header class="sticky top-0 z-30 flex h-15 items-center border-b border-slate-200/75 bg-[#f4f7f5]/90 px-4 backdrop-blur-xl dark:border-white/8 dark:bg-[#0b100e]/88 sm:px-6 lg:px-8">
        <button class="icon-button mr-2 lg:hidden" aria-label="打开导航" @click="ui.sidebarOpen = true"><Menu :size="20" /></button>
        <div>
          <div class="text-[13px] font-semibold text-slate-800 dark:text-slate-100">{{ route.meta.title }}</div>
          <div class="hidden text-[10px] text-slate-400 sm:block">安全、稳定地管理每一台 Android 设备</div>
        </div>
        <div class="ml-auto flex items-center gap-2">
          <span class="hidden items-center gap-1.5 text-xs text-slate-500 sm:flex"><span class="size-1.5 rounded-full bg-brand-500 shadow-[0_0_0_3px_rgba(34,197,94,.12)]" />服务正常</span>
          <div class="ml-2 grid size-8 place-items-center rounded-lg bg-slate-900 text-xs font-semibold text-white dark:bg-white dark:text-slate-900">AD</div>
        </div>
      </header>
      <main class="mx-auto w-full max-w-[1560px] px-4 py-5 sm:px-6 lg:px-8 lg:py-7"><slot /></main>
    </div>
  </div>
</template>
