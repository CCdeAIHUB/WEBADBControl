import { afterEach, describe, expect, it, vi } from 'vitest'
import { getSession, loginWithPassword, updatePassword } from './auth'

afterEach(() => vi.unstubAllGlobals())

describe('password authentication API', () => {
  it('submits a password and returns bootstrap state', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      data: { authenticated: true, mustChangePassword: true },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    const session = await loginWithPassword('admin')

    expect(session.mustChangePassword).toBe(true)
    expect(fetchMock).toHaveBeenCalledWith('/api/v1/session', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ password: 'admin' }),
      credentials: 'same-origin',
    }))
  })

  it('checks sessions and changes passwords through dedicated endpoints', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: { authenticated: false, mustChangePassword: false } }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: { authenticated: true, mustChangePassword: false } }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    expect((await getSession()).authenticated).toBe(false)
    await updatePassword('admin', 'new-password-123')

    expect(fetchMock).toHaveBeenLastCalledWith('/api/v1/password', expect.objectContaining({
      method: 'PUT',
      body: JSON.stringify({ currentPassword: 'admin', newPassword: 'new-password-123' }),
    }))
  })
})
