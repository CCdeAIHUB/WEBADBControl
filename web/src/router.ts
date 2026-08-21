import { createRouter, createWebHistory } from 'vue-router'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'dashboard', component: () => import('@/pages/DashboardPage.vue'), meta: { title: '工作台' } },
    { path: '/devices', name: 'devices', component: () => import('@/pages/DevicesPage.vue'), meta: { title: '设备中心' } },
    { path: '/devices/:id', name: 'device', component: () => import('@/pages/DeviceDetailPage.vue'), meta: { title: '设备详情' } },
    { path: '/automation', name: 'automation', component: () => import('@/pages/AutomationPage.vue'), meta: { title: '自动化任务' } },
    { path: '/assistant', name: 'assistant', component: () => import('@/pages/AssistantPage.vue'), meta: { title: 'AI 助手' } },
    { path: '/settings', name: 'settings', component: () => import('@/pages/SettingsPage.vue'), meta: { title: '系统设置' } },
    { path: '/login', name: 'login', component: () => import('@/pages/LoginPage.vue'), meta: { title: '访问验证', public: true } },
  ],
  scrollBehavior: () => ({ top: 0 }),
})

router.afterEach((to) => {
  document.title = `${String(to.meta.title ?? 'ADBControl')} · ADBControl Web`
})
