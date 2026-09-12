import { afterEach, describe, expect, it, vi } from 'vitest'
import { createQRPairing, normalizePairingCode, pairQRDevice, pairWirelessDevice } from './deviceConnection'

afterEach(() => vi.unstubAllGlobals())

describe('wireless ADB pairing', () => {
  it('keeps only the first six digits from pasted pairing text', () => {
    // 场景：中文输入法或复制文本带空格时，页面只保留 Android 显示的六位数字。
    expect(normalizePairingCode(' 12a34 5678 ')).toBe('123456')
  })

  it('normalizes endpoint and code before calling the pairing API', async () => {
    // 场景：发送到服务端的 endpoint/code 必须规范化，且配对码不能进入 URL 或日志。
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      data: { paired: true, endpoint: '192.168.3.20:37123' },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    await pairWirelessDevice(' 192.168.3.20:37123 ', ' 123456 ')

    expect(fetchMock).toHaveBeenCalledWith('/api/v1/devices/pair', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ endpoint: '192.168.3.20:37123', code: '123456' }),
    }))
  })

  it('uses an opaque session id for QR pairing and never posts a password', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: { sessionId: 'qr-1', serviceName: 'studio-x', qrSvg: '<svg/>', mimeType: 'image/svg+xml', expiresAt: 1 } }), { status: 201, headers: { 'Content-Type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ data: { paired: true, serviceName: 'studio-x', endpoint: '192.168.3.20:37123' } }), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    vi.stubGlobal('fetch', fetchMock)

    await createQRPairing()
    await pairQRDevice('qr-1')

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/v1/devices/qr-pairings', expect.objectContaining({ method: 'POST' }))
    expect(fetchMock).toHaveBeenNthCalledWith(2, '/api/v1/devices/qr-pairings/qr-1/pair', expect.objectContaining({ method: 'POST' }))
    expect(JSON.stringify(fetchMock.mock.calls)).not.toContain('password')
  })
})
