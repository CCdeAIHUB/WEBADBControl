import { describe, expect, it } from 'vitest'
import { parseScreenPacket } from './screenPacket'

describe('screen packet protocol', () => {
  it('parses the negotiated v2 PTS header', () => {
    // 场景：浏览器必须使用 scrcpy 的微秒 PTS，并从第 9 字节后读取 H.264，避免污染码流。
    const bytes = new Uint8Array([2, 0, 0, 0, 0, 0, 18, 214, 135, 0, 0, 1, 0x65])
    const packet = parseScreenPacket(bytes, 2)
    expect(packet.kind).toBe(2)
    expect(packet.presentationTimeUs).toBe(1_234_567)
    expect(Array.from(packet.data)).toEqual([0, 0, 1, 0x65])
  })

  it('keeps the v1 one-byte header compatible', () => {
    const packet = parseScreenPacket(new Uint8Array([0, 0, 0, 1, 0x41]), 1)
    expect(packet.presentationTimeUs).toBeUndefined()
    expect(Array.from(packet.data)).toEqual([0, 0, 1, 0x41])
  })
})
