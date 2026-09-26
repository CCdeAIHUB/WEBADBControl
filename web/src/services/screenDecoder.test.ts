import { describe, expect, it } from 'vitest'
import { screenDecoderFallback, screenDecoderPreference, screenDecoderStatusText, screenFrameDurationMs } from './screenDecoder'

describe('screen decoder selection', () => {
  it('uses WebCodecs when the API is available', () => {
    expect(screenDecoderPreference({ hasWebCodecs: true, hasMediaSource: true, isSecureContext: true })).toBe('webcodecs')
  })

  it('uses MSE hardware playback for an ordinary LAN HTTP page', () => {
    // 场景：MSE 不要求安全上下文，局域网 HTTP 应优先使用浏览器硬件播放而不是受 Profile 限制的软件解码。
    expect(screenDecoderPreference({ hasWebCodecs: false, hasMediaSource: true, isSecureContext: false })).toBe('mse')
    expect(screenDecoderStatusText('mse')).toContain('HTTP')
  })

  it('keeps the software decoder as the final compatibility fallback', () => {
    expect(screenDecoderPreference({ hasWebCodecs: false, hasMediaSource: false, isSecureContext: false })).toBe('software')
    expect(screenDecoderStatusText('software')).toContain('HTTP')
  })

  it('also falls back when a secure browser lacks WebCodecs', () => {
    expect(screenDecoderPreference({ hasWebCodecs: false, hasMediaSource: true, isSecureContext: true })).toBe('mse')
  })

  it('falls back once from a browser decoder to the HTTP-safe software decoder', () => {
    expect(screenDecoderFallback('mse')).toBe('software')
    expect(screenDecoderFallback('webcodecs')).toBe('software')
    expect(screenDecoderFallback('software')).toBeUndefined()
  })

  it('uses scrcpy PTS deltas instead of the selected maximum FPS', () => {
    // 场景：设备实际以约 18 FPS 输出时，55ms 的真实间隔不能被压缩成 30 FPS 的 33ms，否则 MSE 会反复耗尽缓冲。
    expect(screenFrameDurationMs(1_000_000, 1_055_000, 30)).toBe(55)
    expect(screenFrameDurationMs(undefined, 1_055_000, 30)).toBe(33)
    expect(screenFrameDurationMs(1_000_000, 5_000_000, 30)).toBe(250)
  })
})
