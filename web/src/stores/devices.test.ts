import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

import { useDevicesStore } from './devices'

describe('devices store', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('keeps a structured visible failure when refresh fails', async () => {
    // 场景：刷新设备失败时不得伪造空列表成功，界面必须能展示结构化错误。
    const store = useDevicesStore()
    const request = vi.fn().mockRejectedValue({
      errorCode: 'CORE_UNAVAILABLE',
      message: '核心服务不可用',
      module: 'api.client',
      recoverable: true,
      traceId: 'trace-test',
    })

    await store.refresh(request)

    expect(store.status).toBe('error')
    expect(store.error?.errorCode).toBe('CORE_UNAVAILABLE')
    expect(store.devices).toEqual([])
  })

  it('reconciles devices by id without retaining stale connection state', async () => {
    // 场景：离线设备重新在线后必须采用服务端新状态，不能保留旧卡片快照。
    const store = useDevicesStore()
    store.devices = [{ id: 'one', name: 'Phone', state: 'offline', transport: 'adb' }]
    await store.refresh(async () => [{ id: 'one', name: 'Phone', state: 'online', transport: 'wireless' }])
    expect(store.devices[0]?.state).toBe('online')
    expect(store.devices[0]?.transport).toBe('wireless')
  })
})
