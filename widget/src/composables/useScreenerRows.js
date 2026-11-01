// Search, filters, preset and sort applied to the rows the source delivered.

import { computed } from 'vue'

import { findColumn, readValue } from '../columns.js'
import { ALL_PRESET, PRESETS, findPreset } from '../presets.js'

/** Comma-separated tickers: "btc, eth" matches BTC and ETH by symbol or instrument id. */
export function parseSearch(text) {
  return text
    .toUpperCase()
    .replace(/[^A-Z0-9,]/g, '')
    .split(',')
    .filter(Boolean)
}

function matchesSearch(row, terms) {
  if (!terms.length) return true
  const instId = row.instId.replace(/-/g, '')
  return terms.some((term) => row.symbol.includes(term) || instId.includes(term))
}

function matchesFilters(row, filters, ctx) {
  for (const filter of filters) {
    const column = findColumn(filter.key)
    if (!column) continue
    const value = readValue(column, row, ctx)
    if (value === null) return false
    if (filter.op === '>=' ? value < filter.value : value > filter.value) return false
  }
  return true
}

export function rowPasses(row, { terms = [], filters }, ctx) {
  return matchesSearch(row, terms) && matchesFilters(row, filters, ctx)
}

export function sortRows(rows, sort, ctx) {
  const column = findColumn(sort?.key)
  if (!column?.value) return rows
  const read = (row) => {
    const value = readValue(column, row, ctx)
    return value === null ? null : sort.dir === 'absdesc' ? Math.abs(value) : value
  }
  const keyed = rows.map((row) => [read(row), row])
  keyed.sort(([a], [b]) => {
    if (a === null && b === null) return 0
    if (a === null) return 1
    if (b === null) return -1
    return sort.dir === 'asc' ? a - b : b - a
  })
  return keyed.map(([, row]) => row)
}

/**
 * @param {import('vue').Ref<object[]>} rows
 * @param {object} state  the settings state (period, preset, filters, sort, watchlist, customPresets)
 * @param {import('vue').Ref<string>} search
 * @param {import('vue').ComputedRef<{ period: string, benchmark: string }>} ctx
 */
export function useScreenerRows(rows, state, search, ctx) {
  const watchlist = computed(() => new Set(state.watchlist))
  const terms = computed(() => parseSearch(search.value))
  // A saved preset id that no longer exists (a removed category) reads as All.
  const activePreset = computed(() => findPreset(state.preset, state.customPresets) ?? ALL_PRESET)

  const visibleRows = computed(() => {
    const criteria = { terms: terms.value, filters: state.filters }
    const passing = rows.value.filter((row) => rowPasses(row, criteria, ctx.value))
    const sorted = sortRows(passing, state.sort, ctx.value)
    // On the All preset starred instruments float to the top, like a watchlist.
    if (state.preset === 'all' && watchlist.value.size) {
      sorted.sort((a, b) => Number(watchlist.value.has(b.instId)) - Number(watchlist.value.has(a.instId)))
    }
    return sorted
  })

  /** How many rows each preset tab would show, ignoring the search box. */
  const presetCounts = computed(() => {
    const counts = {}
    for (const preset of [...PRESETS, ...state.customPresets]) {
      counts[preset.id] = rows.value.filter((row) =>
        rowPasses(row, { filters: preset.filters }, ctx.value),
      ).length
    }
    return counts
  })

  return { visibleRows, presetCounts, activePreset, watchlist }
}
