// @vitest-environment jsdom

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiMock = vi.fn()
const notifyMock = vi.fn()

vi.mock('@/services/api', () => ({
  api: (...args: unknown[]) => apiMock(...args),
  toAppError: (error: any) => error,
}))
vi.mock('@/stores/ui', () => ({
  useUiStore: () => ({ notify: notifyMock, failure: vi.fn() }),
}))

import CompanionPanel from './CompanionPanel.vue'

describe('companion lifecycle preflight', () => {
  beforeEach(() => {
    apiMock.mockReset()
    notifyMock.mockReset()
  })

  it('upgrades before loading companion capabilities', async () => {
    // 场景：进入伴侣页时先完成版本能力检查，旧版覆盖升级成功后才读取状态与能力。
    const calls: string[] = []
    apiMock.mockImplementation(async (path: string) => {
      calls.push(path)
      if (path.endsWith('/companion/ensure')) {
        return { state: 'updated', updated: true, previousVersionName: '0.11.0', installedVersionName: '0.13.0' }
      }
      if (path.endsWith('/companion/status')) return { installed: true, adbResponsive: true, state: 'adb-responsive', message: 'ready', updateRequired: false }
      return []
    })

    mount(CompanionPanel, { props: { deviceId: 'phone' } })
    await flushPromises()

    expect(calls[0]).toBe('/devices/phone/companion/ensure')
    expect(calls).toContain('/devices/phone/capabilities')
    expect(calls).toContain('/devices/phone/permissions')
    expect(notifyMock).toHaveBeenCalledWith('伴侣已自动升级', '0.11.0 → 0.13.0', 'success')
  })

  it('does not retry capability requests after a failed upgrade', async () => {
    // 场景：签名冲突等升级失败时只读取诊断状态，避免能力与权限请求再次触发重复安装。
    apiMock.mockImplementation(async (path: string) => {
      if (path.endsWith('/companion/ensure')) {
        throw { errorCode: 'COMPANION_SIGNATURE_MISMATCH', message: '签名不一致', module: 'companion.upgrade', recoverable: false, traceId: 'test' }
      }
      if (path.endsWith('/companion/status')) return { installed: true, adbResponsive: false, state: 'outdated', message: '版本过旧', updateRequired: true }
      return []
    })

    mount(CompanionPanel, { props: { deviceId: 'phone' } })
    await flushPromises()

    const paths = apiMock.mock.calls.map(call => call[0])
    expect(paths).toEqual(['/devices/phone/companion/ensure', '/devices/phone/companion/status'])
  })
})
