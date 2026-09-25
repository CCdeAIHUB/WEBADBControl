export interface ScreenPoint {
  x: number
  y: number
  width: number
  height: number
}

export interface ScreenGestureStart {
  point: ScreenPoint
  startedAt: number
}

export function screenGestureAction(start: ScreenGestureStart, end: ScreenPoint, endedAt: number): Record<string, number | string> {
  const distance = Math.hypot(end.x - start.point.x, end.y - start.point.y)
  const tapThreshold = Math.max(8, Math.min(24, Math.hypot(end.width, end.height) * 0.008))
  if (distance <= tapThreshold) {
    return {
      type: 'tap',
      x: end.x,
      y: end.y,
      coordinateWidth: end.width,
      coordinateHeight: end.height,
    }
  }
  return {
    type: 'swipe',
    x: start.point.x,
    y: start.point.y,
    endX: end.x,
    endY: end.y,
    durationMs: Math.max(1, Math.min(3000, endedAt - start.startedAt)),
    coordinateWidth: end.width,
    coordinateHeight: end.height,
  }
}
