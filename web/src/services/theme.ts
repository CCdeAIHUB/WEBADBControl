export type ThemePreference = 'system' | 'light' | 'dark'

const storageKey = 'adbcontrol.theme'
let activeTheme: ThemePreference = 'system'
let mediaListenerInstalled = false

function isThemePreference(value: string | null): value is ThemePreference {
  return value === 'system' || value === 'light' || value === 'dark'
}

function systemPrefersDark() {
  return matchMedia('(prefers-color-scheme: dark)').matches
}

export function applyTheme(theme: ThemePreference) {
  activeTheme = theme
  document.documentElement.classList.toggle('dark', theme === 'dark' || (theme === 'system' && systemPrefersDark()))
  if (!mediaListenerInstalled) {
    matchMedia('(prefers-color-scheme: dark)').addEventListener?.('change', () => {
      if (activeTheme === 'system') applyTheme('system')
    })
    mediaListenerInstalled = true
  }
}

export function persistTheme(theme: ThemePreference) {
  localStorage.setItem(storageKey, theme)
}

export function initializeTheme() {
  const stored = localStorage.getItem(storageKey)
  applyTheme(isThemePreference(stored) ? stored : 'system')
}

export function synchronizeTheme(theme: ThemePreference) {
  persistTheme(theme)
  applyTheme(theme)
}
