const authenticatedKey = 'webadb.authenticated'

export function markClientAuthenticated(authenticated: boolean) {
  if (typeof window === 'undefined') return
  if (authenticated) {
    window.sessionStorage.setItem(authenticatedKey, '1')
    return
  }
  window.sessionStorage.removeItem(authenticatedKey)
}

export function isClientAuthenticated() {
  if (typeof window === 'undefined') return false
  return window.sessionStorage.getItem(authenticatedKey) === '1'
}
