// Presets: a filter set and a sort, picked from the toolbar tabs. The tabs
// row is drawn only when there is more than one preset (built-in plus the
// user's saved ones), so shipping just "All" hides it. The categories of the
// prototype (Movers, Squeeze, Strong vs BTC, …) were taken out by the owner;
// adding one back is one entry here. Filters refer to column keys from
// `columns.js`; values are in the metric's own unit (fractions for
// percentages, USD for money). `sort.dir` is 'asc', 'desc' or 'absdesc'.

export const PRESETS = [
  {
    id: 'all',
    label: 'All',
    filters: [],
    sort: { key: 'vol24', dir: 'desc' },
    description: 'Everything the venue trades, by volume.',
  },
]

/** One-click filters offered in the Filters popover. */
export const QUICK_FILTERS = [
  { label: 'Vol 24h ≥ $50M', filter: { key: 'vol24', op: '>=', value: 50e6 } },
  { label: 'OI ≥ $20M', filter: { key: 'oi', op: '>=', value: 20e6 } },
  { label: 'Funding APR ≥ 30%', filter: { key: 'fundingAnn', op: '>=', value: 0.3 } },
  { label: 'Funding APR ≤ −10%', filter: { key: 'fundingAnn', op: '<=', value: -0.1 } },
  { label: 'Vol regime ≤ 0.6 (squeeze)', filter: { key: 'regime', op: '<=', value: 0.6 } },
  { label: 'Spread ≤ 3 bps', filter: { key: 'spread', op: '<=', value: 3 } },
  { label: 'Listed ≤ 30 days', filter: { key: 'listed', op: '<=', value: 30 } },
]

export const ALL_PRESET = PRESETS[0]

export function findPreset(id, customPresets = []) {
  return PRESETS.find((preset) => preset.id === id) ?? customPresets.find((preset) => preset.id === id) ?? null
}

export function sameFilters(first, second) {
  return JSON.stringify(first) === JSON.stringify(second)
}
