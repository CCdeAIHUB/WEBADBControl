interface PresenceWindow {
  setInterval(handler: TimerHandler, timeout?: number): number
  clearInterval(id?: number): void
  addEventListener(type: string, listener: EventListenerOrEventListenerObject): void
  removeEventListener(type: string, listener: EventListenerOrEventListenerObject): void
}

interface PresenceDocument {
  visibilityState: DocumentVisibilityState
  addEventListener(type: string, listener: EventListenerOrEventListenerObject): void
  removeEventListener(type: string, listener: EventListenerOrEventListenerObject): void
}

interface DevicePresenceSyncOptions {
  refresh: () => Promise<void>
  intervalSeconds: number
  windowTarget?: PresenceWindow
  documentTarget?: PresenceDocument
}

export function createDevicePresenceSync(options: DevicePresenceSyncOptions) {
  const windowTarget = options.windowTarget ?? window
  const documentTarget = options.documentTarget ?? document
  const intervalMs = Math.min(120, Math.max(1, options.intervalSeconds || 5)) * 1000
  let timer: number | undefined
  let running = false
  let inFlight = false

  async function trigger() {
    if (!running || inFlight || documentTarget.visibilityState === 'hidden') return
    inFlight = true
    try {
      await options.refresh()
    } finally {
      inFlight = false
    }
  }

  function handleActivity() { void trigger() }

  function start(immediate = true) {
    if (running) return
    running = true
    timer = windowTarget.setInterval(handleActivity, intervalMs)
    windowTarget.addEventListener('focus', handleActivity)
    windowTarget.addEventListener('online', handleActivity)
    documentTarget.addEventListener('visibilitychange', handleActivity)
    if (immediate) void trigger()
  }

  function stop() {
    if (!running) return
    running = false
    if (timer !== undefined) windowTarget.clearInterval(timer)
    timer = undefined
    windowTarget.removeEventListener('focus', handleActivity)
    windowTarget.removeEventListener('online', handleActivity)
    documentTarget.removeEventListener('visibilitychange', handleActivity)
  }

  return { start, stop, trigger }
}
