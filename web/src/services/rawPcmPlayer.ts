/** Plays scrcpy raw signed 16-bit little-endian, 48 kHz stereo packets. */
export class RawPcmPlayer {
  private context?: AudioContext
  private nextStart = 0

  start() {
    const AudioContextCtor = window.AudioContext
    if (!AudioContextCtor) return false
    try {
      this.context ??= new AudioContextCtor({ sampleRate: 48_000 })
      void this.context.resume()
      this.nextStart = this.context.currentTime
      return true
    } catch {
      this.context = undefined
      this.nextStart = 0
      return false
    }
  }

  push(bytes: Uint8Array) {
    const context = this.context
    if (!context || bytes.byteLength < 4) return
    const frames = Math.floor(bytes.byteLength / 4)
    const buffer = context.createBuffer(2, frames, 48_000)
    const view = new DataView(bytes.buffer, bytes.byteOffset, frames * 4)
    const left = buffer.getChannelData(0)
    const right = buffer.getChannelData(1)
    for (let index = 0; index < frames; index++) {
      left[index] = view.getInt16(index * 4, true) / 32768
      right[index] = view.getInt16(index * 4 + 2, true) / 32768
    }
    const source = context.createBufferSource()
    source.buffer = buffer
    source.connect(context.destination)
    // Drop accumulated latency after a suspended tab or network burst.
    if (this.nextStart < context.currentTime || this.nextStart > context.currentTime + 0.5) this.nextStart = context.currentTime + 0.02
    source.start(this.nextStart)
    this.nextStart += frames / 48_000
  }

  close() {
    const context = this.context
    this.context = undefined
    this.nextStart = 0
    if (context) void context.close()
  }
}
