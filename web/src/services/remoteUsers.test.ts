import { afterEach, describe, expect, it, vi } from 'vitest'
import { assignRemoteDevice, createRemoteUser, listRemoteUsers } from './remoteUsers'

afterEach(() => vi.unstubAllGlobals())

describe('remote user management API', () => {
  it('uses the administrator-only remote user endpoints', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: [] }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: { username: 'operator', role: 'user', devices: [] } }), { status: 201 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: { username: 'operator', role: 'user', devices: ['device-1'] } }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await listRemoteUsers()
    await createRemoteUser('operator', 'operator-password')
    await assignRemoteDevice('operator', 'device-1', true)

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/users', expect.objectContaining({ credentials: 'same-origin' }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/users', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ username: 'operator', password: 'operator-password' }),
    }))
    expect(fetchMock).toHaveBeenNthCalledWith(3, '/api/v1/users/operator/devices', expect.objectContaining({
      method: 'PUT',
      body: JSON.stringify({ deviceId: 'device-1', assigned: true }),
    }))
  })
})
