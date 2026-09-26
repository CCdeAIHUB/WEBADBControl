// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest'
import { createDevicePresenceSync } from './devicePresenceSync'

describe('device presence synchronization', () => {
  afterEach(() => vi.useRealTimers())

  it('refreshes periodically and immediately when the page becomes active again', async () => {
    // 场景：已配对设备重新打开无线调试后，无需手动刷新；周期、窗口聚焦和网络恢复都会重新读取设备列表。
    vi.useFakeTimers()
    const refresh = vi.fn().mockResolvedValue(undefined)
    const sync = createDevicePresenceSync({ refresh, intervalSeconds: 2, windowTarget: window, documentTarget: document })

    sync.start()
    await vi.runAllTicks()
    expect(refresh).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(2000)
    expect(refresh).toHaveBeenCalledTimes(2)

    window.dispatchEvent(new Event('focus'))
    await vi.runAllTicks()
    expect(refresh).toHaveBeenCalledTimes(3)

    window.dispatchEvent(new Event('online'))
    await vi.runAllTicks()
    expect(refresh).toHaveBeenCalledTimes(4)

    sync.stop()
    await vi.advanceTimersByTimeAsync(4000)
    expect(refresh).toHaveBeenCalledTimes(4)
  })

  it('does not stack requests while a previous device refresh is pending', async () => {
    // 场景：ADB 查询较慢时只允许一个后台刷新在途，不能因定时器堆积造成 Core 阻塞。
    vi.useFakeTimers()
    let resolveRefresh: (() => void) | undefined
    const refresh = vi.fn(() => new Promise<void>((resolve) => { resolveRefresh = resolve }))
    const sync = createDevicePresenceSync({ refresh, intervalSeconds: 1, windowTarget: window, documentTarget: document })

    sync.start()
    await vi.advanceTimersByTimeAsync(3000)
    expect(refresh).toHaveBeenCalledTimes(1)

    resolveRefresh?.()
    await vi.runAllTicks()
    await vi.advanceTimersByTimeAsync(1000)
    expect(refresh).toHaveBeenCalledTimes(2)
    sync.stop()
  })
})
