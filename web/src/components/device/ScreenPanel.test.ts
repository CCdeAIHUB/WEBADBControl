// @vitest-environment jsdom

import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/stores/ui', () => ({ useUiStore: () => ({ failure: vi.fn() }) }))

import ScreenPanel from './ScreenPanel.vue'

describe('screen fullscreen controls', () => {
  let fullscreenElement: Element | null

  beforeEach(() => {
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
})
