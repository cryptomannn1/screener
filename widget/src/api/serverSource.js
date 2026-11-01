// Rows from the Go service. By default it subscribes to `GET {baseUrl}/stream`
// (Server-Sent Events named `rows`, data = the same JSON as /screener, one
// event per recompute, about every second) and falls back to polling
// `GET {baseUrl}/screener` every `pollMs` when the stream errors; `sse: false`
// polls only.
//
// Source interface, shared with `mockSource.js`:
//   start(onRows, onError)  onRows(payload) gets `{ ts, rows, coverage, benchmark, total, ... }`
//   stop()
//   label                   shown in the status line

export const DEFAULT_BASE_URL = 'http://localhost:8090'
export const DEFAULT_POLL_MS = 5000

export function createServerSource({
  baseUrl = DEFAULT_BASE_URL,
  pollMs = DEFAULT_POLL_MS,
  sse = true,
} = {}) {
  const origin = baseUrl.replace(/\/+$/, '')
  let timer = null
  let stream = null
  let stopped = false
  let inFlight = false

  async function poll(onRows, onError) {
    if (inFlight) return
    inFlight = true
    try {
      const response = await fetch(`${origin}/screener`, { cache: 'no-store' })
      if (!response.ok) throw new Error(`${response.status} ${response.statusText}`)
      const payload = await response.json()
      if (!stopped) onRows(payload)
    } catch (error) {
      if (!stopped) onError?.(error)
    } finally {
      inFlight = false
    }
  }

  function startPolling(onRows, onError) {
    if (timer) return
    poll(onRows, onError)
    timer = setInterval(() => poll(onRows, onError), pollMs)
  }

  function startStream(onRows, onError) {
    stream = new EventSource(`${origin}/stream`)
    stream.addEventListener('rows', (event) => {
      try {
        onRows(JSON.parse(event.data))
      } catch (error) {
        onError?.(error)
      }
    })
    stream.addEventListener('error', () => {
      // The browser retries the stream on its own; meanwhile the table stays
      // fresh from polling. Polling is cheap enough to leave on.
      startPolling(onRows, onError)
    })
  }

  return {
    label: `${sse ? 'Stream' : 'Server'} ${origin.replace(/^https?:\/\//, '')}`,
    start(onRows, onError) {
      stopped = false
      if (sse && typeof EventSource === 'function') {
        startStream(onRows, onError)
        // One immediate read so the table is not empty until the first event.
        poll(onRows, onError)
      } else {
        startPolling(onRows, onError)
      }
    },
    stop() {
      stopped = true
      clearInterval(timer)
      timer = null
      stream?.close()
      stream = null
    },
  }
}
