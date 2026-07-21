// Thin WebSocket transport helper for Game Mode chat. Holds no domain state —
// callers (Pinia stores) own message interpretation and reconnect policy.

export interface WSConnection {
  send: (obj: unknown) => void
  onMessage: (cb: (data: unknown) => void) => void
  close: () => void
}

export interface ConnectOptions {
  /**
   * Called before each reconnect attempt after an unexpected close. Return
   * false to abort reconnecting (e.g. the session already ended server-side).
   * Left undefined means "always retry" until the caller calls close().
   */
  shouldReconnect?: () => Promise<boolean>
}

const RECONNECT_BASE_MS = 1000
const RECONNECT_CAP_MS = 15000

function wsUrl(path: string): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${protocol}//${window.location.host}${path}`
}

/**
 * Opens a WebSocket at `path` (resolved against the current page's origin)
 * and returns a small handle for sending JSON, subscribing to inbound
 * messages, and closing. Reconnects with exponential backoff + jitter on
 * unexpected close, gated by `options.shouldReconnect`.
 */
export function connect(path: string, options: ConnectOptions = {}): WSConnection {
  const url = wsUrl(path)
  const messageHandlers: ((data: unknown) => void)[] = []

  let socket: WebSocket | null = null
  let closedByCaller = false
  let attempt = 0
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null

  function open() {
    socket = new WebSocket(url)

    socket.onopen = () => {
      attempt = 0
    }

    socket.onmessage = (ev: MessageEvent) => {
      let parsed: unknown
      try {
        parsed = JSON.parse(ev.data)
      } catch {
        return
      }
      for (const cb of messageHandlers) cb(parsed)
    }

    socket.onclose = () => {
      if (closedByCaller) return
      void scheduleReconnect()
    }
  }

  async function scheduleReconnect() {
    if (options.shouldReconnect) {
      const ok = await options.shouldReconnect()
      if (!ok) return
    }
    if (closedByCaller) return

    const backoff = Math.min(RECONNECT_BASE_MS * 2 ** attempt, RECONNECT_CAP_MS)
    const jitter = backoff * 0.2 * Math.random()
    attempt += 1

    reconnectTimer = setTimeout(open, backoff + jitter)
  }

  open()

  return {
    send(obj: unknown) {
      socket?.send(JSON.stringify(obj))
    },
    onMessage(cb: (data: unknown) => void) {
      messageHandlers.push(cb)
    },
    close() {
      closedByCaller = true
      if (reconnectTimer) clearTimeout(reconnectTimer)
      socket?.close()
      socket = null
    },
  }
}
