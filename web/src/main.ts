import { createApp } from 'vue'
import { createPinia } from 'pinia'

import App from './App.vue'
import { router } from './router'
import { installClientErrorReporting } from '@/services/clientErrorReporting'
import './style.css'

window.addEventListener('webadb:auth-required', () => {
  if (router.currentRoute.value.name !== 'login') {
    void router.replace({ name: 'login', query: { redirect: router.currentRoute.value.fullPath } })
  }
})

const app = createApp(App)
installClientErrorReporting(app, router)
app.use(createPinia()).use(router).mount('#app')
