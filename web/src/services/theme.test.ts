// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from 'vitest'
import { applyTheme, initializeTheme, persistTheme, type ThemePreference } from './theme'

describe('theme initialization', () => {
  beforeEach(() => {
    document.documentElement.classList.remove('dark')
    localStorage.clear()
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: true }))
  })

  it('applies the persisted server preference before the first page renders', () => {
    // 场景：首页必须在进入设置页之前使用上次同步的深色主题，不能先显示浅色再跳变。
    persistTheme('dark')
    initializeTheme()

    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })

  it.each<[ThemePreference, boolean]>([['light', false], ['dark', true], ['system', true]])(
    'applies %s consistently on every page',
    (theme, expectedDark) => {
      applyTheme(theme)
      expect(document.documentElement.classList.contains('dark')).toBe(expectedDark)
    },
  )
})
