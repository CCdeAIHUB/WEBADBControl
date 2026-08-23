import { api } from '@/services/api'
import { markClientAuthenticated } from '@/services/clientAuthState'

export interface SessionStatus {
  authenticated: boolean
  mustChangePassword: boolean
}

export async function getSession(): Promise<SessionStatus> {
  const session = await api<SessionStatus>('/session')
  markClientAuthenticated(session.authenticated)
  return session
}

export async function loginWithPassword(password: string): Promise<SessionStatus> {
  const session = await api<SessionStatus>('/session', {
    method: 'POST',
    body: JSON.stringify({ password }),
  })
  markClientAuthenticated(session.authenticated)
  return session
}

export async function logout(): Promise<{ authenticated: boolean }> {
  try {
    return await api('/session', { method: 'DELETE' })
  } finally {
    markClientAuthenticated(false)
  }
}

export function updatePassword(currentPassword: string, newPassword: string): Promise<SessionStatus> {
  return api<SessionStatus>('/password', {
    method: 'PUT',
    body: JSON.stringify({ currentPassword, newPassword }),
  })
}
