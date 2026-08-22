import { api } from '@/services/api'

export interface SessionStatus {
  authenticated: boolean
  mustChangePassword: boolean
}

export function getSession(): Promise<SessionStatus> {
  return api<SessionStatus>('/session')
}

export function loginWithPassword(password: string): Promise<SessionStatus> {
  return api<SessionStatus>('/session', {
    method: 'POST',
    body: JSON.stringify({ password }),
  })
}

export function logout(): Promise<{ authenticated: boolean }> {
  return api('/session', { method: 'DELETE' })
}

export function updatePassword(currentPassword: string, newPassword: string): Promise<SessionStatus> {
  return api<SessionStatus>('/password', {
    method: 'PUT',
    body: JSON.stringify({ currentPassword, newPassword }),
  })
}
