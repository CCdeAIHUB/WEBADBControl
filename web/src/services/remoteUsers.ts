import { api } from '@/services/api'
import type { RemoteAccount } from '@/types/api'

export function listRemoteUsers(): Promise<RemoteAccount[]> {
  return api<RemoteAccount[]>('/users')
}

export function createRemoteUser(username: string, password: string): Promise<RemoteAccount> {
  return api<RemoteAccount>('/users', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
}

export function deleteRemoteUser(username: string): Promise<{ deleted: boolean }> {
  return api(`/users/${encodeURIComponent(username)}`, { method: 'DELETE' })
}

export function resetRemoteUserPassword(username: string, newPassword: string): Promise<{ passwordReset: boolean }> {
  return api(`/users/${encodeURIComponent(username)}/password`, {
    method: 'PUT',
    body: JSON.stringify({ newPassword }),
  })
}

export function assignRemoteDevice(
  username: string,
  deviceId: string,
  assigned: boolean,
): Promise<RemoteAccount> {
  return api<RemoteAccount>(`/users/${encodeURIComponent(username)}/devices`, {
    method: 'PUT',
    body: JSON.stringify({ deviceId, assigned }),
  })
}
