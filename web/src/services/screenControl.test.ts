import { describe, expect, it } from 'vitest'
import { screenGestureAction } from './screenControl'

describe('screen gesture actions', () => {
  it('maps a short pointer movement to a companion tap', () => {
    expect(screenGestureAction(
      { point: { x: 100, y: 200, width: 1080, height: 2400 }, startedAt: 1000 },
      { x: 104, y: 204, width: 1080, height: 2400 },
      1120,
    )).toEqual({ type: 'tap', x: 104, y: 204, coordinateWidth: 1080, coordinateHeight: 2400 })
  })

  it('maps a drag to a bounded companion swipe with source dimensions', () => {
    expect(screenGestureAction(
      { point: { x: 100, y: 1800, width: 1080, height: 2400 }, startedAt: 1000 },
      { x: 100, y: 400, width: 1080, height: 2400 },
      1450,
    )).toEqual({
      type: 'swipe', x: 100, y: 1800, endX: 100, endY: 400, durationMs: 450,
      coordinateWidth: 1080, coordinateHeight: 2400,
    })
  })
})
