// Runs the configured source and exposes what it delivered: the assets, the
// payload time, a tick that counts live deliveries and a status for the empty
// states. A hidden page keeps the latest payload but does not redraw.

import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'

import { createSource } from '../api/index.js'

/** At most one redraw per this many milliseconds, however often the source delivers. */
const PAINT_EVERY_MS = 250

/**
 * @param {() => object} readConfig  the widget's `source` prop
 */
export function useAssets(readConfig) {
  const assets = shallowRef([])
  const ts = ref(null)
  /** Grows with every delivery after the first: a price that changed together with it changed live. */
  const tick = ref(0)
  const error = ref(null)
  const sourceLabel = ref('')
  let source = null
  let pending = null
  let timer = null

  /** 'loading' until the first assets, 'error' while unreachable with nothing shown, else 'live'. */
  const status = computed(() => (assets.value.length ? 'live' : error.value ? 'error' : 'loading'))

  function paint() {
    timer = null
    if (!pending) return
    const first = !assets.value.length
    assets.value = pending.assets
    ts.value = pending.ts ?? Date.now()
    if (!first) tick.value++
    pending = null
  }

  function onData(payload) {
    if (!payload || !Array.isArray(payload.assets) || !payload.assets.length) return
    error.value = null
    pending = payload
    if (!assets.value.length) paint()
    else if (!timer && !document.hidden) timer = setTimeout(paint, PAINT_EVERY_MS)
  }

  function onError(nextError) {
    error.value = nextError
  }

  function onVisible() {
    if (!document.hidden && pending && !timer) paint()
  }

  function restart() {
    source?.stop()
    clearTimeout(timer)
    timer = null
    pending = null
    source = createSource(readConfig())
    sourceLabel.value = source.label ?? 'Custom source'
    assets.value = []
    error.value = null
    source.start(onData, onError)
  }

  onMounted(() => {
    restart()
    document.addEventListener('visibilitychange', onVisible)
  })
  // A host may pass the config inline, as a new object on every render: restart
  // only when it says something different.
  watch(() => {
    const config = readConfig()
    return typeof config?.start === 'function' ? config : JSON.stringify(config)
  }, restart)
  onBeforeUnmount(() => {
    source?.stop()
    clearTimeout(timer)
    document.removeEventListener('visibilitychange', onVisible)
  })

  return { assets, ts, tick, error, status, sourceLabel }
}
