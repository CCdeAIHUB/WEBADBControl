import { api } from '@/services/api'
import { isClientAuthenticated } from '@/services/clientAuthState'
import type { LogEvent, LogStats } from '@/types/api'

export interface LogQuery {
  type?: string
  level?: string
  module?: string
  traceId?: string
  errorCode?: string
  deviceId?: string
  q?: string
  limit?: number
}

export function listLogs(query: LogQuery = {}): Promise<LogEvent[]> {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined && value !== '') params.set(key, String(value))
  }
  const suffix = params.toString() ? `?${params}` : ''
  return api(`/logs${suffix}`)
}

export function getLogStats(): Promise<LogStats> {
  return api('/logs/stats')
}

export function reportClientError(payload: {
  message: string
  source: string
  stack?: string
  component?: string
  route?: string
  errorCode?: string
  traceId?: string
  details?: Record<string, unknown>
}) {
  if (!isClientAuthenticated()) return
  const body = JSON.stringify(payload)
  if (navigator.sendBeacon) {
    const blob = new Blob([body], { type: 'application/json' })
    if (navigator.sendBeacon('/api/v1/logs/client-error', blob)) return
  }
  void fetch('/api/v1/logs/client-error', {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
    body,
    keepalive: true,
  }).catch(() => {
    // Client-side logging must never break the UI path that is already failing.
  })
}

export function logTypeLabel(type: string) {
  return ({ request: '请求', audit: '审计', client_error: '前端错误', system: '系统' } as Record<string, string>)[type] ?? type
}

export function logLevelClass(level: string) {
  if (level === 'error') return 'bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-300'
  if (level === 'warn') return 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300'
  return 'bg-brand-50 text-brand-700 dark:bg-brand-500/10 dark:text-brand-300'
}

export function formatLogTime(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', { hour12: false })
}
