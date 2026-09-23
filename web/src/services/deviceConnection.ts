import { api } from '@/services/api'

export interface DiscoveredWirelessService {
  name: string
  type: 'pairing' | 'connect'
  endpoint: string
}

export interface PairingResult {
  paired: boolean
  endpoint: string
}

export interface QRPairingSession {
  sessionId: string
  serviceName: string
  qrSvg: string
  mimeType: 'image/svg+xml'
  expiresAt: number
}

export interface QRPairingResult extends PairingResult {
  serviceName: string
}

export function normalizePairingCode(value: string): string {
  return value.replace(/\D/g, '').slice(0, 6)
}

export function discoverWirelessDevices(): Promise<DiscoveredWirelessService[]> {
  return api('/devices/discover')
}

export function pairWirelessDevice(endpoint: string, code: string): Promise<PairingResult> {
  return api('/devices/pair', {
    method: 'POST',
    body: JSON.stringify({ endpoint: endpoint.trim(), code: code.trim() }),
  })
}

export function createQRPairing(): Promise<QRPairingSession> {
  return api('/wireless/qr-pairings', { method: 'POST' })
}

export function pairQRDevice(sessionId: string, signal?: AbortSignal): Promise<QRPairingResult> {
  return api(`/wireless/qr-pairings/${encodeURIComponent(sessionId)}/pair`, { method: 'POST', signal })
}

export function cancelQRPairing(sessionId: string): Promise<{ cancelled: boolean }> {
  return api(`/wireless/qr-pairings/${encodeURIComponent(sessionId)}`, { method: 'DELETE' })
}
