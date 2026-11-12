// Assets from the Go service. It folds OKX, Binance and Bybit into one entry
// per asset (`GET {baseUrl}/assets`) and pushes the same payload about once a
// second as Server-Sent Events named `assets` (`GET {baseUrl}/stream?view=assets`).
// The stream reconnects by itself; while it is down the source polls `/assets`
// every `pollMs`. `sse: false` polls only.
//
// Source interface:
//   start(onData, onError)  onData(payload) gets `{ ts, assets: [...] }`
//   stop()
//   label                   for the "unreachable" state

export const DEFAULT_BASE_URL = 'http://localhost:8090'
export const DEFAULT_POLL_MS = 5000
const REQUEST_TIMEOUT_MS = 8000

export function createAssetsSource({ baseUrl = DEFAULT_BASE_URL, pollMs = DEFAULT_POLL_MS, sse = true } = {}) {
  const origin = baseUrl.replace(/\/+$/, '')
  let timer = null
  let stream = null
  let stopped = true
  let inFlight = false

  async function read(onData, onError) {
    if (inFlight) return
    inFlight = true
    const abort = new AbortController()
    const timeout = setTimeout(() => abort.abort(), REQUEST_TIMEOUT_MS)
    try {
      const response = await fetch(`${origin}/assets`, { cache: 'no-store', signal: abort.signal })
      if (!response.ok) throw new Error(`${response.status} ${response.statusText}`)
      const payload = await response.json()
      if (!stopped) onData(payload)
    } catch (error) {
      if (!stopped) onError?.(error)
    } finally {
      clearTimeout(timeout)
      inFlight = false
    }
  }

  function startPolling(onData, onError) {
    if (timer || stopped) return
    timer = setInterval(() => {
      if (!document.hidden) read(onData, onError)
    }, pollMs)
  }

  function stopPolling() {
    clearInterval(timer)
    timer = null
  }

  return {
    label: origin.replace(/^https?:\/\//, ''),
    start(onData, onError) {
      stopped = false
      read(onData, onError) // the table is not empty until the first event
      if (!sse || typeof EventSource !== 'function') {
        startPolling(onData, onError)
        return
      }
      stream = new EventSource(`${origin}/stream?view=assets`)
      stream.addEventListener('assets', (event) => {
        try {
          onData(JSON.parse(event.data))
        } catch (error) {
          onError?.(error)
        }
      })
      stream.addEventListener('error', () => startPolling(onData, onError))
      stream.addEventListener('open', stopPolling)
    },
    stop() {
      stopped = true
      stopPolling()
      stream?.close()
      stream = null
    },
  }
}
