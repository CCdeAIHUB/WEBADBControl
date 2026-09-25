import JMuxer from 'jmuxer'
import { TinyH264Decoder } from '@yume-chan/scrcpy-decoder-tinyh264'

export type ScreenDecoderMode = 'webcodecs' | 'mse' | 'software'

export interface ScreenDecoderEnvironment {
  hasWebCodecs: boolean
  hasMediaSource: boolean
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
  if (environment.hasWebCodecs) return 'webcodecs'
  if (!environment.isSecureContext) return 'software'
  return environment.hasMediaSource ? 'mse' : 'software'
}

export function browserScreenDecoderPreference(): ScreenDecoderMode {
  const browserWindow = window as Window & { WebKitMediaSource?: typeof MediaSource; ManagedMediaSource?: typeof MediaSource }
  return screenDecoderPreference({
    hasWebCodecs: 'VideoDecoder' in window && 'EncodedVideoChunk' in window,
    hasMediaSource: Boolean(window.MediaSource || browserWindow.WebKitMediaSource || browserWindow.ManagedMediaSource),
    isSecureContext: window.isSecureContext,
  })
}

export function screenDecoderStatusText(mode: ScreenDecoderMode): string {
  if (mode === 'webcodecs') return '硬件解码 · WebCodecs'
  return mode === 'mse' ? 'HTTP 兼容 · MSE 硬件播放' : 'HTTP 兼容 · 软件解码'
}

export function screenDecoderFallback(mode: ScreenDecoderMode): ScreenDecoderMode | undefined {
  return mode === 'software' ? undefined : 'software'
}

export async function createScreenDecoder(
  mode: ScreenDecoderMode,
  target: HTMLCanvasElement | HTMLVideoElement,
  callbacks: ScreenDecoderCallbacks,
  fps = 30,
): Promise<ScreenDecoder> {
  if (mode === 'mse') {
    if (!(target instanceof HTMLVideoElement)) throw new Error('MSE 投屏缺少视频播放元素')
    return new MseScreenDecoder(target, callbacks, fps)
  }
  if (!(target instanceof HTMLCanvasElement)) throw new Error('投屏缺少画布元素')
  if (mode === 'webcodecs') return new WebCodecsScreenDecoder(target, callbacks)
  return SoftwareScreenDecoder.create(target, callbacks)
}

type VideoFrameCallback = (now: DOMHighResTimeStamp, metadata: VideoFrameCallbackMetadata) => void
type VideoWithFrameCallback = HTMLVideoElement & {
  requestVideoFrameCallback?: (callback: VideoFrameCallback) => number
  cancelVideoFrameCallback?: (handle: number) => void
}

class MseScreenDecoder implements ScreenDecoder {
  readonly mode = 'mse' as const
  private readonly muxer: JMuxer
  private codecConfig?: Uint8Array
  private disposed = false
  private videoFrameHandle?: number
  private mseReady = false
  private readonly readyPromise: Promise<void>
  private resolveReady!: () => void

  constructor(
    private readonly video: HTMLVideoElement,
    private readonly callbacks: ScreenDecoderCallbacks,
    private readonly fps: number,
  ) {
    this.readyPromise = new Promise((resolve) => { this.resolveReady = resolve })
    const options: JMuxer.Options & {
      videoCodec: 'H264'
      onUnsupportedCodec: (codec: string) => void
    } = {
      node: video,
      mode: 'video',
      videoCodec: 'H264',
      fps,
      flushingTime: 0,
      maxDelay: 120,
      clearBuffer: true,
      onReady: () => {
        this.mseReady = true
        this.resolveReady()
      },
      onError: (error) => callbacks.onError(new Error(`MSE 缓冲区错误：${formatUnknownError(error)}`)),
      onUnsupportedCodec: (codec) => callbacks.onError(new Error(`浏览器不支持设备视频编码：${codec}`)),
    }
    this.muxer = new JMuxer(options)
    this.observeFrames()
  }

  async configure(data: Uint8Array) {
    this.codecConfig = data.slice()
    await this.waitUntilReady()
  }

  async decode(data: Uint8Array, keyframe: boolean) {
    await this.waitUntilReady()
    let packet = data
    if (keyframe && this.codecConfig) {
      packet = new Uint8Array(this.codecConfig.length + data.length)
      packet.set(this.codecConfig)
      packet.set(data, this.codecConfig.length)
      this.codecConfig = undefined
    }
    this.muxer.feed({
      video: packet,
      duration: Math.max(1, Math.round(1000 / this.fps)),
      isLastVideoFrameComplete: true,
    } as JMuxer.Feeder & { isLastVideoFrameComplete: boolean })
    void this.video.play().catch(() => undefined)
  }

  dispose() {
    this.disposed = true
    const video = this.video as VideoWithFrameCallback
    if (this.videoFrameHandle !== undefined) video.cancelVideoFrameCallback?.(this.videoFrameHandle)
    this.video.removeEventListener('timeupdate', this.handleFallbackFrame)
    this.muxer.destroy()
    this.video.pause()
    this.video.removeAttribute('src')
    this.video.load()
    this.codecConfig = undefined
  }

  private async waitUntilReady() {
    if (this.mseReady) return
    await Promise.race([
      this.readyPromise,
      new Promise<never>((_, reject) => window.setTimeout(() => reject(new Error('MSE 播放器初始化超时')), 5000)),
    ])
  }

  private observeFrames() {
    const video = this.video as VideoWithFrameCallback
    if (!video.requestVideoFrameCallback) {
      this.video.addEventListener('timeupdate', this.handleFallbackFrame)
      return
    }
    const observe: VideoFrameCallback = () => {
      if (this.disposed) return
      this.callbacks.onFrame()
      this.videoFrameHandle = video.requestVideoFrameCallback!(observe)
    }
    this.videoFrameHandle = video.requestVideoFrameCallback(observe)
  }

  private handleFallbackFrame = () => {
    if (!this.disposed) this.callbacks.onFrame()
  }
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
    private readonly decoder: TinyH264Decoder,
    private readonly writer: SoftwarePacketWriter,
    private readonly callbacks: ScreenDecoderCallbacks,
  ) {
    this.observeFrames()
  }

  static async create(canvas: HTMLCanvasElement, callbacks: ScreenDecoderCallbacks): Promise<SoftwareScreenDecoder> {
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

function formatUnknownError(error: unknown) {
  if (error instanceof Error) return error.message
  if (typeof error === 'string') return error
  try { return JSON.stringify(error) } catch { return String(error) }
}
