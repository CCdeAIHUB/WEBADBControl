import { createRouter, createWebHistory } from 'vue-router'
import { getSession } from '@/services/auth'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
	{ path: '/', name: 'dashboard', component: () => import('@/pages/DashboardPage.vue'), meta: { title: '工作台' } },
    { path: '/devices', name: 'devices', component: () => import('@/pages/DevicesPage.vue'), meta: { title: '设备中心' } },
    { path: '/devices/:id', name: 'device', component: () => import('@/pages/DeviceDetailPage.vue'), meta: { title: '设备详情' } },
	{ path: '/automation', name: 'automation', component: () => import('@/pages/AutomationPage.vue'), meta: { title: '自动化任务' } },
	{ path: '/assistant', name: 'assistant', component: () => import('@/pages/AssistantPage.vue'), meta: { title: 'AI 助手' } },
	{ path: '/logs', name: 'logs', component: () => import('@/pages/LogsPage.vue'), meta: { title: '系统日志' } },
	{ path: '/settings', name: 'settings', component: () => import('@/pages/SettingsPage.vue'), meta: { title: '系统设置' } },
	{ path: '/change-password', name: 'change-password', component: () => import('@/pages/ChangePasswordPage.vue'), meta: { title: '修改密码' } },
    { path: '/login', name: 'login', component: () => import('@/pages/LoginPage.vue'), meta: { title: '密码登录', public: true } },
  ],
  scrollBehavior: () => ({ top: 0 }),
})

function safeRedirect(value: unknown): string {
  return typeof value === 'string' && value.startsWith('/') && !value.startsWith('//') ? value : '/'
}

router.beforeEach(async (to) => {
  try {
    const session = await getSession()
    if (!session.authenticated) {
      if (to.meta.public === true) return true
      return { name: 'login', query: { redirect: to.fullPath } }
    }
	if (session.mustChangePassword && !session.localBypass && to.name !== 'change-password') return { name: 'change-password' }
	if ((!session.mustChangePassword || session.localBypass) && to.name === 'change-password') return '/'
    if (to.name === 'login') return safeRedirect(to.query.redirect)
    return true
  } catch {
    if (to.meta.public === true) return true
    return { name: 'login', query: { redirect: to.fullPath, service: 'unavailable' } }
  }
})

router.afterEach((to) => {
  document.title = `${String(to.meta.title ?? 'ADBControl')} · ADBControl Web`
})
