// Price levels (opens of the day / week / month, daily moving averages): where
// the price sits against a level and whether it crossed it recently.

import { isNum } from '../format.js'
import { CROSS_HOURS } from './constants.js'

/** Hourly bars since the level came into being, the forming one included:
 *  a close before that says nothing about the level. */
function barsSinceStart(key, ts) {
  const d = new Date(ts || Date.now())
  const h = d.getUTCHours() + 1
  if (key === 'do') return h
  if (key === 'wo') return ((d.getUTCDay() + 6) % 7) * 24 + h
  if (key === 'mo') return (d.getUTCDate() - 1) * 24 + h
  return Infinity
}

/** The most recent hour in which an hourly close sat on the other side of the
 *  level: `{ dir: 'up' | 'down', ago }` or null. Cached on the row. */
export function cross(row, key, ts) {
  if (key in row.crossCache) return row.crossCache[key]
  let out = null
  const level = row.lv[key]
  const closes = row.spark
  if (level && level.px > 0 && closes && closes.length > 2) {
    const n = closes.length
    const ref = isNum(row.mark) && row.mark > 0 ? row.mark : closes[n - 1]
    const side = ref >= level.px ? 1 : -1
    for (let i = n - 2; i >= Math.max(0, n - barsSinceStart(key, ts)); i--) {
      const v = closes[i]
      if (!isNum(v)) continue
      if ((v >= level.px ? 1 : -1) !== side) {
        out = { dir: side > 0 ? 'up' : 'down', ago: n - 1 - i }
        break
      }
    }
  }
  row.crossCache[key] = out
  return out
}

function nearLimit(row, near) {
  if (near === 'atr') return isNum(row.atr) && row.atr > 0 ? row.atr * 0.5 : 0.005
  return parseFloat(near) / 100
}

/**
 * @param {string} mode  above | near | below | up | down
 * @param {string} near  range of "near", see NEAR_RANGES
 */
export function levelPass(row, key, mode, near, ts) {
  const level = row.lv[key]
  if (!level || !isNum(level.dist)) return false
  if (mode === 'above') return level.dist > 0
  if (mode === 'below') return level.dist < 0
  if (mode === 'near') return Math.abs(level.dist) <= nearLimit(row, near)
  const c = cross(row, key, ts)
  return Boolean(c) && c.dir === mode && c.ago <= CROSS_HOURS
}
