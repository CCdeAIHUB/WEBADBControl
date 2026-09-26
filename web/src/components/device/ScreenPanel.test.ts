// @vitest-environment jsdom

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiMock = vi.fn()
const notifyMock = vi.fn()
vi.mock('@/services/api', () => ({
  api: (...args: unknown[]) => apiMock(...args),
  toAppError: (error: any) => error,
}))
vi.mock('@/stores/ui', () => ({ useUiStore: () => ({ failure: vi.fn(), notify: notifyMock }) }))
vi.mock('@/services/logs', () => ({ reportClientError: vi.fn() }))

import ScreenPanel from './ScreenPanel.vue'

describe('screen fullscreen controls', () => {
  let fullscreenElement: Element | null

  beforeEach(() => {
	apiMock.mockReset()
	notifyMock.mockReset()
    fullscreenElement = null
    Object.defineProperty(document, 'fullscreenElement', { configurable: true, get: () => fullscreenElement })
    HTMLElement.prototype.requestFullscreen = vi.fn(async function (this: HTMLElement) {
      fullscreenElement = this
      document.dispatchEvent(new Event('fullscreenchange'))
    })
    document.exitFullscreen = vi.fn(async () => {
      fullscreenElement = null
      document.dispatchEvent(new Event('fullscreenchange'))
    })
  })

  it('toggles between enter and exit fullscreen states', async () => {
    // 场景：进入全屏后同一个按钮必须切换为“退出全屏”，再次点击能够真正退出。
    const wrapper = mount(ScreenPanel, { props: { deviceId: 'phone', active: false } })
    await wrapper.get('button[aria-label="进入全屏"]').trigger('click')
    expect(wrapper.find('button[aria-label="退出全屏"]').exists()).toBe(true)

    await wrapper.get('button[aria-label="退出全屏"]').trigger('click')
    expect(document.exitFullscreen).toHaveBeenCalledOnce()
    expect(wrapper.find('button[aria-label="进入全屏"]').exists()).toBe(true)
  })

  it('keeps the frame-rate control readable inside the dark screen panel', () => {
    const wrapper = mount(ScreenPanel, { props: { deviceId: 'phone', active: false } })
    const select = wrapper.get('select[aria-label="投屏刷新帧率"]')
    expect(select.classes()).toContain('ui-select-dark')
    expect(select.classes()).toContain('!text-slate-100')
  })

  it('asks before installing companion for a denied adb control fallback', async () => {
    // 场景：直接 ADB 控制被设备拒绝且需要伴侣时，必须经内建确认框授权后才安装并重试。
    let actionAttempts = 0
    apiMock.mockImplementation(async (path: string) => {
      if (path.endsWith('/actions') && actionAttempts++ === 0) {
        throw { errorCode: 'COMPANION_INSTALL_REQUIRED', message: '需要伴侣', module: 'companion.requirement', recoverable: true, traceId: 'test' }
      }
      if (path.endsWith('/companion/install')) return { state: 'installed', installedVersionCode: 13, installedVersionName: '0.13.0' }
      return { accepted: true }
    })
    const wrapper = mount(ScreenPanel, { props: { deviceId: 'phone', active: false } })

    await wrapper.get('button[title="返回"]').trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('该控制操作需要伴侣能力支持')
    expect(apiMock.mock.calls.map(call => call[0])).not.toContain('/devices/phone/companion/install')

    const confirm = Array.from(document.body.querySelectorAll('button')).find(button => button.textContent?.includes('安装 / 覆盖安装'))
    ;(confirm as HTMLButtonElement).click()
    await flushPromises()

    expect(apiMock.mock.calls.map(call => call[0])).toContain('/devices/phone/companion/install')
    expect(actionAttempts).toBe(2)
    wrapper.unmount()
  })

  it('preserves the pending control action while confirming a signature-conflict reinstall', async () => {
    // 场景：自动能力补齐遇到签名冲突时，以第二个破坏性确认框接续流程，重装后再重试原控制操作。
    let actionAttempts = 0
    apiMock.mockImplementation(async (path: string) => {
      if (path.endsWith('/actions') && actionAttempts++ === 0) {
        throw { errorCode: 'COMPANION_UPGRADE_REQUIRED', message: '需要新版伴侣', module: 'companion.requirement', recoverable: true, traceId: 'test' }
      }
      if (path.endsWith('/companion/install')) {
        throw { errorCode: 'COMPANION_SIGNATURE_MISMATCH', message: '签名不一致', module: 'companion.upgrade', recoverable: false, traceId: 'test' }
      }
      if (path.endsWith('/companion/reinstall')) return { state: 'reinstalled', installedVersionCode: 13, installedVersionName: '0.13.0' }
      return { accepted: true }
    })
    const wrapper = mount(ScreenPanel, { props: { deviceId: 'phone', active: false } })

    await wrapper.get('button[title="主页"]').trigger('click')
    await flushPromises()
    const install = Array.from(document.body.querySelectorAll('button')).find(button => button.textContent?.includes('安装 / 覆盖安装'))
    ;(install as HTMLButtonElement).click()
    await flushPromises()

    expect(document.body.textContent).toContain('无法覆盖安装伴侣应用')
    expect(apiMock.mock.calls.map(call => call[0])).not.toContain('/devices/phone/companion/reinstall')
    const replace = Array.from(document.body.querySelectorAll('button')).find(button => button.textContent?.includes('卸载旧版并重新安装'))
    ;(replace as HTMLButtonElement).click()
    await flushPromises()

    expect(apiMock.mock.calls.map(call => call[0])).toContain('/devices/phone/companion/reinstall')
    expect(actionAttempts).toBe(2)
    wrapper.unmount()
  })
})
