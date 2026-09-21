import { describe, expect, it } from 'vitest'
import { screenDecoderPreference, screenDecoderStatusText } from './screenDecoder'

describe('screen decoder selection', () => {
  it('uses WebCodecs when the API is available', () => {
    expect(screenDecoderPreference({ hasWebCodecs: true, isSecureContext: true })).toBe('webcodecs')
  })

  it('uses the software decoder for an ordinary LAN HTTP page', () => {
    expect(screenDecoderPreference({ hasWebCodecs: false, isSecureContext: false })).toBe('software')
    expect(screenDecoderStatusText('software')).toContain('HTTP')
  })

  it('also falls back when a secure browser lacks WebCodecs', () => {
    expect(screenDecoderPreference({ hasWebCodecs: false, isSecureContext: true })).toBe('software')
  })
})
