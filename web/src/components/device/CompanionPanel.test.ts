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

  it('asks before uninstalling and reinstalling a companion with a conflicting signature', async () => {
    // 场景：覆盖安装遇到签名冲突时先停下，只有用户再次确认清除旧 App 后才调用重装接口。
    let replaced = false
    apiMock.mockImplementation(async (path: string) => {
      if (path.endsWith('/companion/status')) return replaced
        ? { installed: true, adbResponsive: true, state: 'adb-responsive', message: 'ready', updateRequired: false }
        : { installed: true, installedVersionCode: 11, requiredVersionCode: 13, adbResponsive: false, state: 'outdated', message: '版本过旧', updateRequired: true }
      if (path.endsWith('/companion/install')) throw { errorCode: 'COMPANION_SIGNATURE_MISMATCH', message: '签名不一致', module: 'companion.upgrade', recoverable: false, traceId: 'test' }
      if (path.endsWith('/companion/reinstall')) {
        replaced = true
        return { state: 'reinstalled', updated: true, installedVersionCode: 13, installedVersionName: '0.13.0' }
      }
      return []
    })

    const wrapper = mount(CompanionPanel, { props: { deviceId: 'phone' } })
    await flushPromises()
    const install = Array.from(document.body.querySelectorAll('button')).find(button => button.textContent?.includes('覆盖安装伴侣 App'))
    ;(install as HTMLButtonElement).click()
    await flushPromises()

    expect(document.body.textContent).toContain('无法覆盖安装伴侣应用')
    expect(document.body.textContent).toContain('清除伴侣应用数据')
    expect(apiMock.mock.calls.map(call => call[0])).not.toContain('/devices/phone/companion/reinstall')

    const replace = Array.from(document.body.querySelectorAll('button')).find(button => button.textContent?.includes('卸载旧版并重新安装'))
    ;(replace as HTMLButtonElement).click()
    await flushPromises()

    expect(apiMock.mock.calls.map(call => call[0])).toContain('/devices/phone/companion/reinstall')
    expect(notifyMock).toHaveBeenCalledWith('伴侣应用已重新安装', expect.stringContaining('需要重新授予'), 'success')
    wrapper.unmount()
  })
})
