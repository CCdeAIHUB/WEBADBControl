import { describe, expect, it, vi } from 'vitest'
import { listLogs, logLevelClass, logTypeLabel } from './logs'

describe('logs service', () => {
  it('serializes query parameters for log search', async () => {
    // 场景：日志中心必须能按 traceId、错误码和类型组合检索。
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ data: [] }), { status: 200 }))
    await listLogs({ type: 'audit', traceId: 'trace-1', errorCode: 'ADB_FAILED', limit: 50 })
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/logs?type=audit&traceId=trace-1&errorCode=ADB_FAILED&limit=50', expect.any(Object))
    fetchMock.mockRestore()
  })

  it('maps log labels and levels for UI badges', () => {
    expect(logTypeLabel('client_error')).toBe('前端错误')
    expect(logTypeLabel('custom')).toBe('custom')
    expect(logLevelClass('error')).toContain('red')
    expect(logLevelClass('warn')).toContain('amber')
    expect(logLevelClass('info')).toContain('brand')
  })
})
