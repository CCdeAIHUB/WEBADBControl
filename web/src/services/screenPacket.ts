export interface ScreenPacket {
  kind: number
  presentationTimeUs?: number
  data: Uint8Array
}

export function parseScreenPacket(bytes: Uint8Array, protocol: number): ScreenPacket {
  const headerSize = protocol >= 2 ? 9 : 1
  if (bytes.length < headerSize) throw new Error('投屏视频包头不完整')
  return {
    kind: bytes[0],
    presentationTimeUs: protocol >= 2 ? readUint64(bytes, 1) : undefined,
    data: bytes.slice(headerSize),
  }
}

function readUint64(bytes: Uint8Array, offset: number): number {
  const view = new DataView(bytes.buffer, bytes.byteOffset + offset, 8)
  const value = view.getBigUint64(0, false)
  if (value > BigInt(Number.MAX_SAFE_INTEGER)) throw new Error('投屏时间戳超出安全范围')
  return Number(value)
}
