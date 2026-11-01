// The widget's persisted state, in one flat object the host can store as is:
// the terminal passes it as `settings` and receives `update:settings` with the
// changed keys, exactly like `useWidget().setSettings(partial)`.

import { reactive, watch } from 'vue'

import { DEFAULT_COLUMN_KEYS } from '../columns.js'
import { ALL_PRESET } from '../presets.js'

export const DEFAULT_SETTINGS = Object.freeze({
  period: '1h',
  preset: ALL_PRESET.id,
  filters: ALL_PRESET.filters,
  sort: ALL_PRESET.sort,
  columns: DEFAULT_COLUMN_KEYS,
  watchlist: [],
  customPresets: [],
  selected: '',
})

/** Only the keys the widget knows; a host may keep other things in the same object. */
function pickKnown(settings) {
  const known = {}
  for (const key of Object.keys(DEFAULT_SETTINGS)) {
    if (settings && settings[key] !== undefined) known[key] = settings[key]
  }
  return known
}

/**
 * @param {() => object} readSettings  the `settings` prop
 * @param {(partial: object) => void} emitUpdate  called with every change
 */
export function useScreenerSettings(readSettings, emitUpdate) {
  const state = reactive({ ...DEFAULT_SETTINGS, ...pickKnown(readSettings()) })

  // The host may change settings on its own (another tab, a reset): follow it.
  watch(readSettings, (next) => Object.assign(state, pickKnown(next)), { deep: true })

  function patch(partial) {
    Object.assign(state, partial)
    emitUpdate(partial)
  }

  return { state, patch }
}
