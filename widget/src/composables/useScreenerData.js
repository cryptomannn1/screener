// Runs the configured source and exposes what it delivered: rows, the
// response's metadata and a status for the empty states.

import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'

import { createSource } from '../api/index.js'

/**
 * @param {() => object} readConfig  the widget's `source` prop
 */
export function useScreenerData(readConfig) {
  const rows = shallowRef([])
  const meta = shallowRef({ ts: null, benchmark: '', coverage: null, total: 0 })
  const error = ref(null)
  const sourceLabel = ref('')
  let source = null

  /** 'loading' until the first rows, 'error' while unreachable with nothing shown, else 'live'. */
  const status = computed(() => {
    if (rows.value.length) return 'live'
    return error.value ? 'error' : 'loading'
  })

  function onRows(payload) {
    error.value = null
    rows.value = Array.isArray(payload.rows) ? payload.rows : []
    meta.value = {
      ts: payload.ts ?? Date.now(),
      benchmark: payload.benchmark ?? '',
      coverage: payload.coverage ?? null,
      total: payload.total ?? rows.value.length,
    }
  }

  function onError(nextError) {
    error.value = nextError
  }

  function restart() {
    source?.stop()
    source = createSource(readConfig())
    sourceLabel.value = source.label ?? 'Custom source'
    rows.value = []
    error.value = null
    source.start(onRows, onError)
  }

  onMounted(restart)
  watch(readConfig, restart, { deep: true })
  onBeforeUnmount(() => source?.stop())

  return { rows, meta, error, status, sourceLabel }
}
