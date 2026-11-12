// The widget's persisted state, in one flat object the host can store as is:
// the terminal passes it as `settings` and receives `update:settings` with the
// changed keys, exactly like `useWidget().setSettings(partial)`.
//
//   exchanges  { okx, bn, bb }: which exchanges go into a row
//   cls        asset class: all | crypto | stock | commodity | fx
//   tab        the open tab
//   views      tab id -> its view (columns, order, conditions, sort); only
//              tabs the user changed, so a ready table follows its definition
//              until the user makes it their own
//   selected   asset key of the highlighted row
//
// Changes are sent at most once per EMIT_DELAY_MS: a condition value is typed
// key by key, and the terminal syncs settings to the account.

import { computed, onBeforeUnmount, reactive, watch } from 'vue'

import { CLASSES, EXCHANGES } from '../model/constants.js'
import { TABS, TAB_BY_ID, cleanView, sameView } from '../tabs.js'

const EMIT_DELAY_MS = 300
const ALL_ON = Object.fromEntries(EXCHANGES.map(([code]) => [code, true]))

function readExchanges(saved) {
  const out = { ...ALL_ON }
  if (saved && typeof saved === 'object') for (const [code] of EXCHANGES) if (saved[code] === false) out[code] = false
  return EXCHANGES.some(([code]) => out[code]) ? out : { ...ALL_ON } // one exchange always stays on
}

/**
 * @param {() => object} readSettings  the `settings` prop
 * @param {(partial: object) => void} emitUpdate
 */
export function useScreenerState(readSettings, emitUpdate) {
  const saved = readSettings() || {}
  const state = reactive({
    exchanges: readExchanges(saved.exchanges),
    cls: CLASSES.some((c) => c[0] === saved.cls) ? saved.cls : 'crypto',
    tab: TAB_BY_ID[saved.tab] ? saved.tab : 'overview',
    selected: typeof saved.selected === 'string' ? saved.selected : '',
  })
  /** Every tab's working view. The open one may hold an unfinished phrase; what is stored is always clean. */
  const views = reactive(Object.fromEntries(TABS.map((tab) => [tab.id, cleanView(saved.views?.[tab.id], tab.id)])))
  const view = computed(() => views[state.tab])

  function snapshot() {
    const changed = {}
    for (const tab of TABS) {
      const clean = cleanView(views[tab.id], tab.id)
      if (!sameView(clean, cleanView(null, tab.id))) changed[tab.id] = clean
    }
    return { exchanges: { ...state.exchanges }, cls: state.cls, tab: state.tab, selected: state.selected, views: changed }
  }

  const sent = {}
  for (const [key, value] of Object.entries(snapshot())) sent[key] = JSON.stringify(value)
  let timer = null

  function flush() {
    clearTimeout(timer)
    timer = null
    const partial = {}
    for (const [key, value] of Object.entries(snapshot())) {
      const text = JSON.stringify(value)
      if (text !== sent[key]) {
        sent[key] = text
        partial[key] = value
      }
    }
    if (Object.keys(partial).length) emitUpdate(partial)
  }

  watch(
    [state, views],
    () => {
      clearTimeout(timer)
      timer = setTimeout(flush, EMIT_DELAY_MS)
    },
    { deep: true },
  )
  onBeforeUnmount(flush)

  // The host may change settings on its own (another device, a reset): follow it.
  // What comes back equal to what was last sent is our own echo and is ignored.
  watch(
    readSettings,
    (next) => {
      if (!next) return
      for (const key of Object.keys(sent)) {
        if (next[key] === undefined) continue
        const text = JSON.stringify(next[key])
        if (text === sent[key]) continue
        sent[key] = text
        if (key === 'exchanges') Object.assign(state.exchanges, readExchanges(next.exchanges))
        else if (key === 'views') for (const tab of TABS) views[tab.id] = cleanView(next.views?.[tab.id], tab.id)
        else if (key === 'tab') state.tab = TAB_BY_ID[next.tab] ? next.tab : state.tab
        else if (key === 'cls') state.cls = CLASSES.some((c) => c[0] === next.cls) ? next.cls : state.cls
        else state[key] = next[key]
      }
    },
    { deep: true },
  )

  function switchTab(id) {
    if (!TAB_BY_ID[id] || id === state.tab) return false
    views[state.tab] = cleanView(views[state.tab], state.tab) // an unfinished phrase does not survive leaving its tab
    state.tab = id
    return true
  }

  return { state, views, view, switchTab }
}
