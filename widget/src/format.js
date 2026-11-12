// Number formatting. The `*Text` functions return plain strings; the `*Cell`
// functions return what a table cell draws: `{ text, tone }` with tone one of
// 'pos' | 'neg' | 'mid' | 'dim' | '', or null when the metric is not available
// (drawn as a dash: the server sends `null` for a metric it cannot compute).

export const isNum = (value) => typeof value === 'number' && Number.isFinite(value)

/** Price with as many decimals as its magnitude needs. */
export function priceText(value) {
  const digits = value >= 1000 ? 1 : value >= 100 ? 2 : value >= 1 ? 3 : value >= 0.01 ? 5 : value >= 0.0001 ? 7 : 10
  return value.toLocaleString('en-US', { minimumFractionDigits: digits, maximumFractionDigits: digits })
}

/** `$1.23T`, `$45.10B`, `$450M`, `$12.5M`, `$820K`, `$640`. */
export function usdText(value) {
  const abs = Math.abs(value)
  if (abs >= 1e12) return '$' + (value / 1e12).toFixed(2) + 'T'
  if (abs >= 1e9) return '$' + (value / 1e9).toFixed(abs >= 1e11 ? 0 : 2) + 'B'
  if (abs >= 1e6) return '$' + (value / 1e6).toFixed(abs >= 1e8 ? 0 : 1) + 'M'
  if (abs >= 1e3) return '$' + (value / 1e3).toFixed(0) + 'K'
  return '$' + value.toFixed(0)
}

/** Funding rate as a signed percentage with four decimals. */
export function fundingText(value) {
  return (value > 0 ? '+' : '') + (value * 100).toFixed(4) + '%'
}

export function pctCell(value, digits = 2, signed = true) {
  if (!isNum(value)) return null
  const points = value * 100
  const text = (signed && points > 0 ? '+' : '') + points.toFixed(digits) + '%'
  const tone = !signed ? '' : points > 0.0005 ? 'pos' : points < -0.0005 ? 'neg' : 'mid'
  return { text, tone }
}

export function usdCell(value) {
  return isNum(value) ? { text: usdText(value), tone: '' } : null
}

/** A ratio against a norm, `2.40×`. Busy reads green, quiet reads dim. */
export function multipleCell(value) {
  if (!isNum(value)) return null
  return { text: value.toFixed(value >= 10 ? 0 : 2) + '×', tone: value >= 2 ? 'pos' : value < 0.6 ? 'dim' : '' }
}

export function fundingCell(value) {
  if (!isNum(value)) return null
  return { text: fundingText(value), tone: value > 0 ? 'pos' : value < 0 ? 'neg' : 'mid' }
}

export function plainCell(value, digits = 2, suffix = '') {
  return isNum(value) ? { text: value.toFixed(digits) + suffix, tone: '' } : null
}

/** A price level: its price, then the signed distance of the last price from it. */
export function levelCell(level) {
  if (!level || !isNum(level.dist)) return null
  return { lead: priceText(level.px), ...pctCell(level.dist) }
}
