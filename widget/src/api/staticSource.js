// A fixed `/assets` payload, delivered once: for a sandbox, a story or a test
// without the service. Give it the payload itself (`data`) or a function that
// loads it (`load`, may return a promise or an ES module with `default`).

export function createStaticSource({ data = null, load = null, label = 'Snapshot' } = {}) {
  let stopped = true
  return {
    label,
    async start(onData, onError) {
      stopped = false
      try {
        const loaded = data ?? (await load?.())
        const payload = loaded?.default ?? loaded
        if (!payload || !Array.isArray(payload.assets)) throw new Error('no assets in the snapshot')
        if (!stopped) onData(payload)
      } catch (error) {
        if (!stopped) onError?.(error)
      }
    },
    stop() {
      stopped = true
    },
  }
}
