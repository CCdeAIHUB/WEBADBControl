import type { App } from 'vue'
import type { Router } from 'vue-router'
import { reportClientError } from '@/services/logs'

export function installClientErrorReporting(app: App, router: Router) {
  app.config.errorHandler = (error, instance, info) => {
    const message = error instanceof Error ? error.message : String(error)
    reportClientError({
      message,
      source: 'vue',
      stack: error instanceof Error ? error.stack : undefined,
      component: instance?.$options?.name ?? 'anonymous',
      route: router.currentRoute.value.fullPath,
      errorCode: 'WEB_COMPONENT_ERROR',
      details: { info },
    })
  }

  window.addEventListener('error', (event) => {
    reportClientError({
      message: event.message || '未捕获的浏览器异常',
      source: 'window.error',
      stack: event.error instanceof Error ? event.error.stack : undefined,
      route: router.currentRoute.value.fullPath,
      errorCode: 'WEB_UNCAUGHT_ERROR',
      details: { filename: event.filename, lineno: event.lineno, colno: event.colno },
    })
  })

  window.addEventListener('unhandledrejection', (event) => {
    const reason = event.reason
    reportClientError({
      message: reason instanceof Error ? reason.message : String(reason ?? '未处理的 Promise 异常'),
      source: 'window.unhandledrejection',
      stack: reason instanceof Error ? reason.stack : undefined,
      route: router.currentRoute.value.fullPath,
      errorCode: 'WEB_UNHANDLED_REJECTION',
    })
  })
}
