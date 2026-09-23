// @vitest-environment jsdom

import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const connection = vi.hoisted(() => ({
  cancelQRPairing: vi.fn(),
  createQRPairing: vi.fn(),
  discoverWirelessDevices: vi.fn(),
  pairQRDevice: vi.fn(),
  pairWirelessDevice: vi.fn(),
}))

vi.mock('@/services/deviceConnection', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/services/deviceConnection')>(),
  ...connection,
}))
vi.mock('@/stores/devices', () => ({ useDevicesStore: () => ({ connect: vi.fn() }) }))
vi.mock('@/stores/ui', () => ({ useUiStore: () => ({ notify: vi.fn(), failure: vi.fn() }) }))

import DeviceConnectionDialog from './DeviceConnectionDialog.vue'

describe('QR wireless pairing flow', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    connection.discoverWirelessDevices.mockResolvedValue([])
    connection.createQRPairing.mockResolvedValue({
      sessionId: 'qr-session', serviceName: 'studio-test', qrSvg: '<svg/>', mimeType: 'image/svg+xml', expiresAt: 4_000_000_000,
    })
    connection.pairQRDevice.mockResolvedValue({ paired: true, serviceName: 'studio-test', endpoint: '192.168.3.20:37123' })
    connection.cancelQRPairing.mockResolvedValue({ cancelled: true })
  })

  it('starts pairing automatically after rendering the QR code', async () => {
    // 场景：手机扫码后不应再依赖用户点击确认；网页必须立即等待 Core 发现手机广播并完成配对。
    const wrapper = mount(DeviceConnectionDialog, { props: { open: true }, attachTo: document.body })
    await flushPromises()
    const qrTab = [...document.body.querySelectorAll<HTMLButtonElement>('button[role="tab"]')].find((button) => button.textContent?.includes('扫码配对'))
    expect(qrTab).toBeDefined()
    qrTab!.click()
    await flushPromises()

    expect(connection.createQRPairing).toHaveBeenCalledOnce()
    expect(connection.pairQRDevice).toHaveBeenCalledWith('qr-session', expect.any(AbortSignal))
    wrapper.unmount()
  })
})
