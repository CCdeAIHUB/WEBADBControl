// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getSession } = vi.hoisted(() => ({ getSession: vi.fn() }))
vi.mock('@/services/auth', () => ({ getSession }))

import { router } from './router'

describe('authenticated navigation', () => {
  beforeEach(async () => {
    window.scrollTo = vi.fn()
    getSession.mockReset()
    getSession.mockResolvedValue({ authenticated: true, mustChangePassword: true })
    await router.replace('/login')
  })

  it('allows navigation while the default password is still active', async () => {
    await router.push('/devices')

    expect(router.currentRoute.value.name).toBe('devices')
  })

  it('still redirects anonymous visitors to login', async () => {
    getSession.mockResolvedValue({ authenticated: false, mustChangePassword: false })
    await router.push('/automation')

    expect(router.currentRoute.value.name).toBe('login')
    expect(router.currentRoute.value.query.redirect).toBe('/automation')
  })
})
