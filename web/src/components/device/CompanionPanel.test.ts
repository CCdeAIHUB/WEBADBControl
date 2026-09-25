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

  it('loads capabilities only after the installed version satisfies requirements', async () => {
	// 场景：伴侣版本满足能力基线时，读取状态后直接同步能力，不执行安装。
    const calls: string[] = []
    apiMock.mockImplementation(async (path: string) => {
      calls.push(path)
      if (path.endsWith('/companion/status')) return { installed: true, adbResponsive: true, state: 'adb-responsive', message: 'ready', updateRequired: false }
      return []
    })

    mount(CompanionPanel, { props: { deviceId: 'phone' } })
    await flushPromises()

    expect(calls[0]).toBe('/devices/phone/companion/status')
    expect(calls).toContain('/devices/phone/capabilities')
    expect(calls).toContain('/devices/phone/permissions')
    expect(calls).not.toContain('/devices/phone/companion/install')
  })

  it('asks for confirmation before installing an outdated companion', async () => {
	// 场景：旧版伴侣不能自动覆盖；先展示内建对话框，用户确认后才安装并同步能力。
    let upgraded = false
    apiMock.mockImplementation(async (path: string) => {
      if (path.endsWith('/companion/status')) return upgraded
        ? { installed: true, adbResponsive: true, state: 'adb-responsive', message: 'ready', updateRequired: false }
        : { installed: true, installedVersionCode: 11, installedVersionName: '0.11.0', requiredVersionCode: 13, requiredVersionName: '0.13.0', adbResponsive: false, state: 'outdated', message: '版本过旧', updateRequired: true }
      if (path.endsWith('/companion/install')) {
        upgraded = true
        return { state: 'updated', updated: true, installedVersionCode: 13, installedVersionName: '0.13.0' }
      }
      return []
    })

    const wrapper = mount(CompanionPanel, { props: { deviceId: 'phone' } })
    await flushPromises()

    expect(document.body.textContent).toContain('该功能需要伴侣能力支持')
    expect(apiMock.mock.calls.map(call => call[0])).toEqual(['/devices/phone/companion/status'])

    const confirm = Array.from(document.body.querySelectorAll('button')).find(button => button.textContent?.includes('覆盖安装伴侣 App'))
    expect(confirm).toBeTruthy()
    ;(confirm as HTMLButtonElement).click()
    await flushPromises()

    const paths = apiMock.mock.calls.map(call => call[0])
    expect(paths).toContain('/devices/phone/companion/install')
    expect(paths).toContain('/devices/phone/capabilities')
    expect(notifyMock).toHaveBeenCalledWith('伴侣应用已覆盖安装', '设备版本：0.13.0', 'success')
    wrapper.unmount()
  })
})
