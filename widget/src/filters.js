// Conditions and order. A view (see `tabs.js`) carries its conditions as
//   nums    column key -> { op: '>=' | '<=' | 'between', v, v2 }
//   levels  level column -> above | near | below | up | down
//   sector  one sector name or ''
// plus `near`, the range of "near" for level conditions. A number condition
// without a value is a phrase still being written: it does not filter yet.

import { COLUMNS } from './columns.js'
import { isNum, usdText } from './format.js'
import { operatorLabel } from './model/constants.js'
import { cross, levelPass } from './model/levels.js'

export const numOn = (f) => Boolean(f) && (isNum(f.v) || (f.op === 'between' && isNum(f.v2)))

/** Comma-separated tickers or names: "btc, eth". */
export function parseSearch(text) {
  return String(text || '')
    .toUpperCase()
    .split(',')
    .map((term) => term.trim())
    .filter(Boolean)
}

/**
 * @param {object} view    conditions of a tab
 * @param {object} scope   { cls, terms, ts }: asset class, search terms, payload time
 * @param {string} [skipLevel]  a level condition to leave out (for the counts of its own menu)
 */
export function passes(row, view, scope, skipLevel) {
  if (scope.cls !== 'all' && row.cls !== scope.cls) return false
  if (view.sector && row.sector !== view.sector) return false
  if (scope.terms.length && !scope.terms.some((t) => row.base.includes(t) || row.nameUpper.includes(t))) return false
  for (const key in view.nums) {
    const f = view.nums[key]
    const a = isNum(f.v)
    const b = f.op === 'between' && isNum(f.v2)
    if (!a && !b) continue
    const v = COLUMNS[key].value(row)
    if (!isNum(v)) return false
    if (f.op === 'between') {
      const lo = a && b ? Math.min(f.v, f.v2) : a ? f.v : null
      const hi = a && b ? Math.max(f.v, f.v2) : b ? f.v2 : null
      if ((lo != null && v < lo) || (hi != null && v > hi)) return false
    } else if (f.op === '>=' ? v < f.v : v > f.v) return false
  }
  for (const key in view.levels) {
    if (key !== skipLevel && !levelPass(row, key, view.levels[key], view.near, scope.ts)) return false
  }
  return true
}

/** How many rows would match this level condition with every other filter kept. */
export function countLevel(rows, view, scope, key, mode) {
  let n = 0
  for (const row of rows) if (passes(row, view, scope, key) && levelPass(row, key, mode, view.near, scope.ts)) n++
  return n
}

export const isFiltered = (view, key) => Boolean(view.levels[key] || numOn(view.nums[key]) || (key === 'sector' && view.sector))

/** A phrase left without a value: a number waiting for it, or the sector waiting for a choice. */
export const isDraft = (view, key) => Boolean((view.nums[key] && !numOn(view.nums[key])) || (key === 'sector' && !view.sector))

/* ---- how a threshold is shown and typed ---- */

export const UNIT_AFFIX = { usd: ['$', ''], pct: ['', '%'], x: ['', '×'], bps: ['', 'bps'] }

export function thresholdText(key, v) {
  const unit = COLUMNS[key].unit
  if (unit === 'usd') return usdText(v).replace(/\.0+(?=[KMBT])/, '')
  if (unit === 'pct') return String(+(v * 100).toFixed(4)).replace('-', '−') + '%'
  if (unit === 'x') return v + '×'
  if (unit === 'bps') return v + ' bps'
  return String(v)
}

/** Label of a one-click threshold: its own words when the column gives them, else "above $20M". */
export function quickLabel(key, op, v) {
  const quick = (COLUMNS[key].quick || []).find((q) => q[0] === op && q[1] === v)
  return quick && quick[2] ? quick[2] : `${operatorLabel(op)} ${thresholdText(key, v)}`
}

/** What the value field shows for a stored number. */
export function valueInput(key, v) {
  if (!isNum(v)) return ''
  const unit = COLUMNS[key].unit
  if (unit === 'usd') return usdText(v).slice(1).replace(/\.0+(?=[KMBT])/, '')
  if (unit === 'pct') return String(+(v * 100).toFixed(4))
  return String(v)
}

/** What a typed text means: 20m, 1.5b, 500k, 3, -0.01. Percent columns are typed in percent. */
export function parseValue(key, text) {
  const t = String(text).trim().toLowerCase().replace(/[$,%×\s]|bps/g, '').replace('−', '-')
  const m = t.match(/^(-?\d*\.?\d+)([kmbt])?$/)
  if (!m) return null
  const n = parseFloat(m[1]) * (m[2] ? { k: 1e3, m: 1e6, b: 1e9, t: 1e12 }[m[2]] : 1)
  if (!Number.isFinite(n)) return null
  return COLUMNS[key].unit === 'pct' ? n / 100 : n
}

/** The value part of a number phrase: "$20M", "3% and 10%". */
export function numText(view, key) {
  const f = view.nums[key]
  if (!numOn(f)) return ''
  if (f.op !== 'between') return thresholdText(key, f.v)
  return (isNum(f.v) ? thresholdText(key, f.v) : '…') + ' and ' + (isNum(f.v2) ? thresholdText(key, f.v2) : '…')
}

/* ---- order ---- */

const bad = (v) => v == null || (typeof v === 'number' && !Number.isFinite(v))

/** Sorts in place. Without a chosen column the order follows the question asked:
 *  nearest to the level, latest cross, else volume. Empty values always go last. */
export function sortRows(list, view, ts) {
  const sort = view.sort
  const column = sort.key ? COLUMNS[sort.key] : null
  if (column && column.value) {
    const get = sort.dir === 'near' ? (row) => { const v = column.value(row); return bad(v) ? null : Math.abs(v) } : column.value
    return list.sort((a, b) => {
      const x = get(a)
      const y = get(b)
      if (bad(x) && bad(y)) return 0
      if (bad(x)) return 1
      if (bad(y)) return -1
      const d = typeof x === 'string' ? x.localeCompare(y) : x - y
      return sort.dir === 'desc' ? -d : d
    })
  }
  const keys = Object.keys(view.levels)
  const nearKey = keys.find((k) => view.levels[k] === 'near')
  const crossKey = keys.find((k) => view.levels[k] === 'up' || view.levels[k] === 'down')
  if (nearKey) return list.sort((a, b) => Math.abs(a.lv[nearKey].dist) - Math.abs(b.lv[nearKey].dist))
  if (crossKey) {
    const ago = (row) => (cross(row, crossKey, ts) || { ago: 99 }).ago
    return list.sort((a, b) => ago(a) - ago(b) || (b.vol24 || 0) - (a.vol24 || 0))
  }
  return list.sort((a, b) => (b.vol24 || 0) - (a.vol24 || 0))
}

/** Header click: first direction, the other one, then back to the default order. */
export function nextSort(sort, key) {
  const column = COLUMNS[key]
  if (!column || !column.value) return sort
  const first = column.text ? 'asc' : 'desc'
  const second = column.text ? 'desc' : 'asc'
  if (sort.key !== key) return { key, dir: first }
  if (sort.dir === first) return { key, dir: second }
  return { key: null, dir: null }
}
