// One WebSocket to the game server, shared by the whole page. A component
// "watches" a GET route: the server pushes that route's answer whenever it
// changes. While the socket is down (server restarting, proxy without
// WebSocket support) every watch falls back to plain polling of the same
// route, so the game keeps working either way.

export interface Pushed<T> {
  data: T
  sentAt: number // when the answer was asked for (polling) or received (push)
  pushed: boolean
}

interface WatchOpts<T> {
  every: number // polling interval while the socket is down
  onData: (p: Pushed<T>) => void
  onError?: (status: number) => void
  // A live game streams match frames: where the client is in the stream.
  game?: () => { round: number; from: number }
}

interface Watch extends WatchOpts<unknown> {
  id: string
  path: string
  timer: number
  busy: boolean
}

const watches = new Map<string, Watch>()
let socket: WebSocket | null = null
let open = false
let wanted = false
let retry = 0
let retryTimer = 0
let seq = 0

function subscribe(w: Watch) {
  if (!open || !socket) return
  const g = w.game?.()
  socket.send(JSON.stringify({ op: 'watch', id: w.id, path: w.path, round: g?.round ?? 0, from: g?.from ?? 0 }))
}

async function pollOnce(w: Watch) {
  if (open || w.busy || !watches.has(w.id)) return
  w.busy = true
  const sentAt = Date.now()
  try {
    const g = w.game?.()
    const url = g ? `${w.path}${w.path.includes('?') ? '&' : '?'}from=${g.from}` : w.path
    const res = await fetch(url, { credentials: 'same-origin' })
    const data = await res.json().catch(() => null)
    if (!watches.has(w.id) || open) return
    if (res.ok) w.onData({ data, sentAt, pushed: false })
    else w.onError?.(res.status)
  } catch {
    /* network hiccup: the next tick tries again */
  } finally {
    w.busy = false
  }
}

function startPolling(w: Watch) {
  window.clearInterval(w.timer)
  void pollOnce(w)
  w.timer = window.setInterval(() => void pollOnce(w), w.every)
}

function connect() {
  if (socket || !wanted) return
  const ws = new WebSocket(`${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/api/ws`)
  socket = ws
  ws.onopen = () => {
    open = true
    retry = 0
    for (const w of watches.values()) {
      window.clearInterval(w.timer)
      subscribe(w)
    }
  }
  ws.onmessage = (e) => {
    let m: { id: string; status: number; data: unknown }
    try {
      m = JSON.parse(String(e.data))
    } catch {
      return
    }
    const w = watches.get(m.id)
    if (!w) return
    if (m.status >= 200 && m.status < 300) w.onData({ data: m.data, sentAt: Date.now(), pushed: true })
    else w.onError?.(m.status)
  }
  ws.onclose = () => {
    if (socket !== ws) return
    socket = null
    open = false
    for (const w of watches.values()) startPolling(w)
    if (!wanted) return
    // Back off up to 15 seconds, with jitter so clients do not reconnect in step.
    const wait = Math.min(15000, 500 * 2 ** retry++) * (0.7 + Math.random() * 0.6)
    window.clearTimeout(retryTimer)
    retryTimer = window.setTimeout(connect, wait)
  }
}

/** Signed in: open the socket. Signed out: close it and stop everything. */
export function setLive(on: boolean) {
  wanted = on
  if (on) connect()
  else {
    window.clearTimeout(retryTimer)
    const ws = socket
    socket = null
    open = false
    ws?.close()
  }
}

/** Follows a GET route. Returns the function that stops following it. */
export function watch<T>(path: string, opts: WatchOpts<T>): () => void {
  const w: Watch = { ...(opts as WatchOpts<unknown>), id: `w${++seq}`, path, timer: 0, busy: false }
  watches.set(w.id, w)
  if (open) subscribe(w)
  else startPolling(w)
  return () => {
    watches.delete(w.id)
    window.clearInterval(w.timer)
    if (open && socket) socket.send(JSON.stringify({ op: 'unwatch', id: w.id }))
  }
}
