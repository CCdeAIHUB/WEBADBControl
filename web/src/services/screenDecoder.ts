export type ScreenDecoderMode = 'webcodecs' | 'software'

export interface ScreenDecoderEnvironment {
  hasWebCodecs: boolean
  isSecureContext: boolean
}

export interface ScreenDecoder {
  readonly mode: ScreenDecoderMode
  configure(data: Uint8Array): Promise<void>
  decode(data: Uint8Array, keyframe: boolean): Promise<void>
  dispose(): void
}

export interface ScreenDecoderCallbacks {
  onFrame: () => void
  onError: (error: Error) => void
}

export function screenDecoderPreference(environment: ScreenDecoderEnvironment): ScreenDecoderMode {
  return environment.hasWebCodecs ? 'webcodecs' : 'software'
}

export function browserScreenDecoderPreference(): ScreenDecoderMode {
  return screenDecoderPreference({
    hasWebCodecs: 'VideoDecoder' in window && 'EncodedVideoChunk' in window,
    isSecureContext: window.isSecureContext,
  })
}

export function screenDecoderStatusText(mode: ScreenDecoderMode): string {
  return mode === 'webcodecs' ? '硬件解码 · WebCodecs' : 'HTTP 兼容 · 软件解码'
}

export async function createScreenDecoder(
  mode: ScreenDecoderMode,
  canvas: HTMLCanvasElement,
  callbacks: ScreenDecoderCallbacks,
): Promise<ScreenDecoder> {
  if (mode === 'webcodecs') return new WebCodecsScreenDecoder(canvas, callbacks)
  return SoftwareScreenDecoder.create(canvas, callbacks)
}

class WebCodecsScreenDecoder implements ScreenDecoder {
  readonly mode = 'webcodecs' as const
  private decoder: VideoDecoder
  private codecConfig?: Uint8Array

  constructor(private readonly canvas: HTMLCanvasElement, callbacks: ScreenDecoderCallbacks) {
    this.decoder = new VideoDecoder({
      output: (frame) => {
        this.canvas.width = frame.displayWidth
        this.canvas.height = frame.displayHeight
        this.canvas.getContext('2d')?.drawImage(frame, 0, 0)
        frame.close()
        callbacks.onFrame()
      },
      error: (error) => callbacks.onError(error),
    })
  }

  async configure(data: Uint8Array) {
    this.codecConfig = data.slice()
    this.decoder.configure({ codec: codecName(data), optimizeForLatency: true })
  }

  async decode(data: Uint8Array, keyframe: boolean) {
    let packet = data
    if (keyframe && this.codecConfig) {
      packet = new Uint8Array(this.codecConfig.length + data.length)
      packet.set(this.codecConfig)
      packet.set(data, this.codecConfig.length)
    }
    this.decoder.decode(new EncodedVideoChunk({
      type: keyframe ? 'key' : 'delta',
      timestamp: performance.now() * 1000,
      data: packet,
    }))
  }

  dispose() {
    if (this.decoder.state !== 'closed') this.decoder.close()
    this.codecConfig = undefined
  }
}

class SoftwareScreenDecoder implements ScreenDecoder {
  readonly mode = 'software' as const
  private disposed = false
  private lastRendered = 0
  private animationFrame?: number

  private constructor(
    private readonly decoder: import('@yume-chan/scrcpy-decoder-tinyh264').TinyH264Decoder,
    private readonly writer: SoftwarePacketWriter,
    private readonly callbacks: ScreenDecoderCallbacks,
  ) {
    this.observeFrames()
  }

  static async create(canvas: HTMLCanvasElement, callbacks: ScreenDecoderCallbacks): Promise<SoftwareScreenDecoder> {
    const { TinyH264Decoder } = await import('@yume-chan/scrcpy-decoder-tinyh264')
    const decoder = new TinyH264Decoder({ canvas })
    return new SoftwareScreenDecoder(decoder, decoder.writable.getWriter(), callbacks)
  }

  async configure(data: Uint8Array) {
    await this.writer.write({ type: 'configuration', data })
  }

  async decode(data: Uint8Array, keyframe: boolean) {
    await this.writer.write({ type: 'data', keyframe, data })
  }

  dispose() {
    this.disposed = true
    if (this.animationFrame !== undefined) cancelAnimationFrame(this.animationFrame)
    void this.writer.close().catch(() => undefined)
    this.decoder.dispose()
  }

  private observeFrames() {
    if (this.disposed) return
    if (this.decoder.framesRendered > this.lastRendered) {
      const frames = this.decoder.framesRendered - this.lastRendered
      this.lastRendered = this.decoder.framesRendered
      for (let index = 0; index < frames; index += 1) this.callbacks.onFrame()
    }
    this.animationFrame = requestAnimationFrame(() => this.observeFrames())
  }
}

type SoftwarePacket = { type: 'configuration'; data: Uint8Array } | { type: 'data'; keyframe?: boolean; data: Uint8Array }

interface SoftwarePacketWriter {
  write(packet: SoftwarePacket): Promise<void>
  close(): Promise<void>
}

function codecName(config: Uint8Array) {
  for (let index = 0; index + 7 < config.length; index += 1) {
    const start = config[index] === 0 && config[index + 1] === 0 && (config[index + 2] === 1 || (config[index + 2] === 0 && config[index + 3] === 1))
    if (!start) continue
    const offset = index + (config[index + 2] === 1 ? 3 : 4)
    if ((config[offset] & 0x1f) !== 7) continue
    return `avc1.${[config[offset + 1], config[offset + 2], config[offset + 3]].map((value) => value.toString(16).padStart(2, '0')).join('').toUpperCase()}`
  }
  return 'avc1.42E01E'
}
