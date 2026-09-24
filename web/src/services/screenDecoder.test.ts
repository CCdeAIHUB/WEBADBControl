import { describe, expect, it } from 'vitest'
import { screenDecoderFallback, screenDecoderPreference, screenDecoderStatusText } from './screenDecoder'

describe('screen decoder selection', () => {
  it('uses WebCodecs when the API is available', () => {
    expect(screenDecoderPreference({ hasWebCodecs: true, hasMediaSource: true, isSecureContext: true })).toBe('webcodecs')
  })

  it('uses software video decoding immediately for an ordinary LAN HTTP page', () => {
    expect(screenDecoderPreference({ hasWebCodecs: false, hasMediaSource: true, isSecureContext: false })).toBe('software')
    expect(screenDecoderStatusText('software')).toContain('HTTP')
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
})
