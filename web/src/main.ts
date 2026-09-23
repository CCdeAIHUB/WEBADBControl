import { createApp } from 'vue'
import { createPinia } from 'pinia'

import App from './App.vue'
import { router } from './router'
import { installClientErrorReporting } from '@/services/clientErrorReporting'
import { initializeTheme } from '@/services/theme'
import './style.css'

// Apply the last server-synchronized preference before Vue paints the first route.
initializeTheme()

window.addEventListener('webadb:auth-required', () => {
  if (router.currentRoute.value.name !== 'login') {
    void router.replace({ name: 'login', query: { redirect: router.currentRoute.value.fullPath } })
  }
})

const app = createApp(App)
installClientErrorReporting(app, router)
app.use(createPinia()).use(router).mount('#app')
