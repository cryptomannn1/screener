// Cell formatting. Every formatter returns a cell view the table knows how to
// draw: `{ kind: 'text', text, tone }` with tone one of 'pos' | 'neg' | 'mid' |
// 'dim' | '', or NA when the metric is not available. NA is drawn as a dash:
// the server sends `null` for a metric it cannot compute, never 0.

export const NA = Object.freeze({ kind: 'na' })

const isNumber = (value) => typeof value === 'number' && Number.isFinite(value)
const text = (value, tone = '') => ({ kind: 'text', text: value, tone })

/** `$1.23B`, `$450M`, `$12.5M`, `$820K`, `$640` — without the sign for filters. */
export function compactUsd(value) {
  const abs = Math.abs(value)
  if (abs >= 1e9) return (value / 1e9).toFixed(2) + 'B'
  if (abs >= 1e6) return (value / 1e6).toFixed(abs >= 1e8 ? 0 : 1) + 'M'
  if (abs >= 1e3) return (value / 1e3).toFixed(0) + 'K'
  return value.toFixed(0)
}

export function usd(value) {
  return isNumber(value) ? text('$' + compactUsd(value)) : NA
}

/** Price with as many decimals as its magnitude needs. */
export function price(value) {
  if (!isNumber(value)) return NA
  const digits = value >= 1000 ? 1 : value >= 100 ? 2 : value >= 1 ? 3 : value >= 0.01 ? 5 : 7
  return text(
    value.toLocaleString('en-US', { minimumFractionDigits: digits, maximumFractionDigits: digits }),
  )
}

/** A price with a dollar sign (mark and last are quoted in USD-pegged stablecoins). */
export function dollarPrice(value) {
  const view = price(value)
  return view.kind === 'text' ? text('$' + view.text, view.tone) : view
}

/** A fraction as a percentage. Signed values carry a +/- sign and a tone. */
export function pct(value, digits = 2, signed = true) {
  if (!isNumber(value)) return NA
  const points = value * 100
  const sign = signed && points > 0 ? '+' : ''
  const tone = !signed ? '' : points > 0.0005 ? 'pos' : points < -0.0005 ? 'neg' : 'mid'
  return text(sign + points.toFixed(digits) + '%', tone)
}

/** Funding per interval. Positive funding means longs pay, so it reads as a cost. */
export function funding(value) {
  if (!isNumber(value)) return NA
  const points = value * 100
  const tone = points > 0.01 ? 'neg' : points < -0.005 ? 'pos' : 'mid'
  return text((points > 0 ? '+' : '') + points.toFixed(4) + '%', tone)
}

/** A ratio against a norm, `2.4×`. Busy reads green, quiet reads dim. */
export function multiple(value) {
  if (!isNumber(value)) return NA
  const tone = value >= 2 ? 'pos' : value < 0.6 ? 'dim' : ''
  return text(value.toFixed(value >= 10 ? 0 : 2) + '×', tone)
}

export function number(value, digits = 2) {
  return isNumber(value) ? text(value.toFixed(digits)) : NA
}

export function bps(value) {
  return isNumber(value) ? text(value.toFixed(value < 10 ? 1 : 0) + ' bps') : NA
}

export function days(value) {
  return isNumber(value) ? text(Math.floor(value) + 'd') : NA
}

export function leverage(value) {
  return isNumber(value) && value > 0 ? text(value + '×') : NA
}

/** Volatility regime: the server's thresholds are 0.6 (squeeze) and 1.6 (expansion). */
export function regime(value) {
  if (!isNumber(value)) return NA
  const badge = value < 0.6 ? 'squeeze' : value > 1.6 ? 'expansion' : ''
  return { kind: 'badge', text: value.toFixed(2) + '×', badge }
}

/** A share of 0..1 drawn as a small bar with its percentage. */
export function share(value) {
  if (!isNumber(value)) return NA
  const points = Math.round(value * 100)
  return { kind: 'bar', text: points + '%', ratio: Math.max(0, Math.min(1, value)) }
}

export function divergence(value) {
  if (!isNumber(value)) return NA
  if (value > 0) return text('absorbing', 'pos')
  if (value < 0) return text('distributing', 'neg')
  return text('none', 'dim')
}

/** A price level: the level price, then the signed distance of last from it to its right. */
export function level(lvl) {
  if (!lvl || !isNumber(lvl.dist) || !isNumber(lvl.px)) return NA
  return { kind: 'stack', main: price(lvl.px), sub: pct(lvl.dist) }
}

export function time(timestamp) {
  return timestamp ? new Date(timestamp).toLocaleTimeString('en-GB') : 'never'
}

/* ---- Filter values: what the user types and what the chip shows ---- */

/**
 * Parses a typed filter value for a column unit. Percent metrics are typed in
 * percent (`2` means 2%), money in plain USD with optional K / M / B.
 * Returns null when the text is not a number.
 */
export function parseFilterValue(unit, raw) {
  let source = String(raw).trim().toUpperCase().replace(/[$,\s%×X]/g, '')
  let factor = 1
  const suffix = source.slice(-1)
  if (suffix === 'K') factor = 1e3
  else if (suffix === 'M') factor = 1e6
  else if (suffix === 'B') factor = 1e9
  if (factor !== 1) source = source.slice(0, -1)
  const value = parseFloat(source) * factor
  if (!Number.isFinite(value)) return null
  return unit === 'pct' ? value / 100 : value
}

export function formatFilterValue(unit, value) {
  switch (unit) {
    case 'pct':
      return (value * 100).toFixed(2).replace(/\.?0+$/, '') + '%'
    case 'usd':
      return '$' + compactUsd(value)
    case 'x':
      return value + '×'
    case 'bps':
      return value + ' bps'
    case 'days':
      return value + 'd'
    default:
      return String(value)
  }
}
