// @vitest-environment jsdom

import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getSession } = vi.hoisted(() => ({ getSession: vi.fn() }))
vi.mock('@/services/auth', () => ({ getSession }))

import { router } from './router'

describe('authenticated navigation', () => {
  beforeEach(async () => {
    window.scrollTo = vi.fn()
    getSession.mockReset()
	getSession.mockResolvedValue({ authenticated: true, mustChangePassword: true, localBypass: true, role: 'admin' })
    await router.replace('/login')
  })

	it('allows loopback management while the Core default password is unchanged', async () => {
    await router.push('/devices')

		expect(router.currentRoute.value.name).toBe('devices')
  })

	it('requires a non-loopback session to change the Core default password', async () => {
		getSession.mockResolvedValue({ authenticated: true, mustChangePassword: true, localBypass: false, role: 'admin' })
		await router.push('/devices')
		expect(router.currentRoute.value.name).toBe('change-password')
	})

  it('still redirects anonymous visitors to login', async () => {
    getSession.mockResolvedValue({ authenticated: false, mustChangePassword: false })
    await router.push('/automation')

    expect(router.currentRoute.value.name).toBe('login')
    expect(router.currentRoute.value.query.redirect).toBe('/automation')
  })
})
