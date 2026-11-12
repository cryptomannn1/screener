// Tabs are ready tables. A tab is its columns in order, its conditions and its
// sort: the base table plus the columns of its question, with the conditions as
// ordinary phrases on the bar. Exchanges, asset class and search stay as chosen
// across tabs.

import { COLUMNS, DEFAULT_COLUMN_KEYS } from './columns.js'
import { numOn } from './filters.js'
import { isNum } from './format.js'
import { LEVEL_MODES, NEAR_RANGES } from './model/constants.js'

/** The same floor of 24h volume in every ready table. */
const LIQUID = 10e6
const VS_BTC_COLUMNS = ['symbol', 'mark', 'chg24', 'vs24h', 'vs4h', 'chg7d', 'vol24', 'oi', 'funding']

export const TABS = [
  { id: 'overview', name: 'Overview', title: 'The whole market, busiest first', view: {} },
  {
    id: 'anomalies',
    name: 'Anomalies',
    title: 'Trading at least twice its 7-day norm today',
    view: {
      cols: ['symbol', 'mark', 'chg4h', 'chg24', 'volx7', 'vol24', 'oi', 'funding'],
      nums: { volx7: { op: '>=', v: 2, v2: null }, vol24: { op: '>=', v: LIQUID, v2: null } },
      filters: ['volx7', 'vol24'],
      sort: { key: 'volx7', dir: 'desc' },
    },
  },
  {
    id: 'strong',
    name: 'Strong vs BTC',
    title: 'Ahead of BTC by 3 to 10% over 24 hours: stronger than the market, not a pump',
    view: {
      cols: VS_BTC_COLUMNS,
      nums: { vs24h: { op: 'between', v: 0.03, v2: 0.1 }, vol24: { op: '>=', v: LIQUID, v2: null } },
      filters: ['vs24h', 'vol24'],
      sort: { key: 'vs24h', dir: 'desc' },
    },
  },
  {
    id: 'weak',
    name: 'Weak vs BTC',
    title: 'Behind BTC by 3 to 10% over 24 hours: weaker than the market, not a collapse',
    view: {
      cols: VS_BTC_COLUMNS,
      nums: { vs24h: { op: 'between', v: -0.1, v2: -0.03 }, vol24: { op: '>=', v: LIQUID, v2: null } },
      filters: ['vs24h', 'vol24'],
      sort: { key: 'vs24h', dir: 'asc' },
    },
  },
]
export const TAB_BY_ID = Object.fromEntries(TABS.map((tab) => [tab.id, tab]))

/** The parts of a view that are its conditions (what "Reset filters" returns). */
export const FILTER_KEYS = ['filters', 'nums', 'levels', 'near', 'sector']

const clone = (value) => JSON.parse(JSON.stringify(value))

/**
 * The view a tab holds, normalised: known columns only with Symbol first,
 * conditions only on columns that are in the table, no unfinished phrases.
 * `cleanView(null, id)` is the tab as it ships.
 */
export function cleanView(view, id) {
  const base = { cols: DEFAULT_COLUMN_KEYS.slice(), sort: { key: null, dir: null }, nums: {}, levels: {}, near: '0.5', sector: '', filters: [] }
  const o = Object.assign(base, clone(TAB_BY_ID[id].view), view ? clone(view) : {})
  o.cols = ['symbol', ...[...new Set(Array.isArray(o.cols) ? o.cols : [])].filter((k) => COLUMNS[k] && k !== 'symbol')]
  const has = (k) => o.cols.includes(k)
  const byKey = (a, b) => (a[0] < b[0] ? -1 : 1)
  o.nums = Object.fromEntries(
    Object.entries(o.nums || {})
      .filter(([k, f]) => has(k) && f && numOn(f))
      .sort(byKey)
      .map(([k, f]) => [k, { op: f.op, v: isNum(f.v) ? f.v : null, v2: f.op === 'between' && isNum(f.v2) ? f.v2 : null }]),
  )
  o.levels = Object.fromEntries(
    Object.entries(o.levels || {})
      .filter(([k, mode]) => has(k) && COLUMNS[k].level && LEVEL_MODES.some((m) => m[0] === mode))
      .sort(byKey),
  )
  if (!has('sector') || !o.sector) o.sector = ''
  const on = (k) => Boolean(o.nums[k] || o.levels[k] || (k === 'sector' && o.sector))
  o.filters = [...new Set(Array.isArray(o.filters) ? o.filters : [])].filter(on)
  for (const k of [...Object.keys(o.nums), ...Object.keys(o.levels), 'sector']) if (on(k) && !o.filters.includes(k)) o.filters.push(k)
  if (!NEAR_RANGES.some((r) => r[0] === o.near) || !Object.values(o.levels).includes('near')) o.near = '0.5'
  if (!o.sort || !has(o.sort.key) || !COLUMNS[o.sort.key].value || !['asc', 'desc', 'near'].includes(o.sort.dir)) o.sort = { key: null, dir: null }
  return { cols: o.cols, sort: o.sort, nums: o.nums, levels: o.levels, near: o.near, sector: o.sector, filters: o.filters }
}

export const sameView = (a, b) => JSON.stringify(a) === JSON.stringify(b)
