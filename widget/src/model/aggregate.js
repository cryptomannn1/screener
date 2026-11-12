// One row per asset over the selected exchanges, built from the `/assets`
// payload (`../../api/screener.md`). Which exchanges go into a row is the
// user's choice, so the widget adds up the per-exchange blocks itself.
//
// Volume and open interest add up. Price and the 24h change come from the
// busiest of the selected exchanges. Funding is brought to 8 hours on every
// exchange (intervals differ: 1h, 4h, 8h) and weighted by open interest.

import { isNum } from '../format.js'
import { EXCHANGES, LEVELS } from './constants.js'

const BENCHMARK = 'c:BTC'

/**
 * @param {object[]} assets  `assets` of the payload
 * @param {Record<string, boolean>} selected  exchange code -> on
 * @returns {object[]} rows; an asset none of the selected exchanges lists is left out
 */
export function aggregate(assets, selected) {
  const on = EXCHANGES.map(([code]) => code).filter((code) => selected[code])
  const rows = []
  for (const asset of assets) {
    const venues = on.filter((code) => asset.v[code])
    if (!venues.length) continue

    let primary = venues[0]
    for (const code of venues) {
      if ((asset.v[code].vol || 0) > (asset.v[primary].vol || 0)) primary = code
    }
    const lead = asset.v[primary]
    const m = asset.m || {}

    const f8 = {}
    const prices = []
    let vol = 0
    let oi = 0
    let hasOi = false
    let weight = 0
    let weighted = 0
    let count = 0
    let plain = 0
    for (const code of venues) {
      const x = asset.v[code]
      vol += x.vol || 0
      if (isNum(x.oi)) {
        oi += x.oi
        hasOi = true
      }
      if (isNum(x.f)) {
        const rate = (x.f * 8) / (x.fi || 8)
        const w = isNum(x.oi) && x.oi > 0 ? x.oi : 0
        f8[code] = rate
        weight += w
        weighted += rate * w
        count++
        plain += rate
      }
      if (isNum(x.p) && x.p > 0) prices.push(x.p)
    }
    const funding = count ? (weight > 0 ? weighted / weight : plain / count) : null
    const rates = Object.values(f8)

    const lv = {}
    for (const [key] of LEVELS) {
      const px = m.lv ? m.lv[key] : null
      lv[key] = isNum(px) && px > 0 && isNum(lead.p) ? { px, dist: lead.p / px - 1 } : null
    }

    rows.push({
      id: asset.k,
      base: asset.s,
      name: asset.n || '',
      nameUpper: (asset.n || '').toUpperCase(),
      cls: asset.cls,
      sector: asset.sec || '',
      venues,
      primary,
      byVenue: asset.v,
      f8,
      mark: lead.p,
      chg: { '1h': m.chg ? m.chg['1h'] : null, '4h': m.chg ? m.chg['4h'] : null, '24h': lead.c },
      chg7d: m.chg7d,
      chg30d: m.chg30d,
      vol24: vol,
      volx7: m.volx7,
      volx: m.volx || {},
      oi: hasOi ? oi : null,
      oiD: m.oiD || {},
      funding,
      fundingAnn: isNum(funding) ? funding * 3 * 365 : null,
      fGap: rates.length > 1 ? Math.max(...rates) - Math.min(...rates) : null,
      pxGap: prices.length > 1 ? (Math.max(...prices) / Math.min(...prices) - 1) * 1e4 : null,
      mcap: asset.mcap,
      mcapRank: asset.rank,
      oiMcap: hasOi && asset.mcap > 0 ? oi / asset.mcap : null,
      atr: m.atr,
      regime: m.regime,
      takerBuy: m.takerBuy || {},
      delta: m.delta || {},
      tape: m.tape,
      vsBtc: { '1h': m.vsBtc ? m.vsBtc['1h'] : null, '4h': m.vsBtc ? m.vsBtc['4h'] : null, '24h': null },
      beta: m.beta,
      corr: m.corr,
      spread: m.spread,
      depth05: m.depth05,
      lv,
      spark: m.spark && m.spark.length > 1 ? m.spark : null,
      crossCache: {},
    })
  }

  // Strength against BTC over 24 hours follows the same exchange choice as the row's own change.
  let btc24 = null
  const btcRow = rows.find((row) => row.id === BENCHMARK)
  if (btcRow) btc24 = btcRow.chg['24h']
  else {
    const btc = assets.find((asset) => asset.k === BENCHMARK)
    const block = btc ? Object.values(btc.v)[0] : null
    btc24 = block ? block.c : null
  }
  for (const row of rows) {
    row.vsBtc['24h'] = isNum(btc24) && isNum(row.chg['24h']) ? row.chg['24h'] - btc24 : null
  }
  return rows
}

/** How many assets each exchange lists, for the Exchanges menu. */
export function exchangeCounts(assets) {
  const counts = {}
  for (const [code] of EXCHANGES) counts[code] = 0
  for (const asset of assets) {
    for (const [code] of EXCHANGES) if (asset.v[code]) counts[code]++
  }
  return counts
}

/** Rows per asset class, `all` included. */
export function classCounts(rows) {
  const counts = { all: rows.length }
  for (const row of rows) counts[row.cls] = (counts[row.cls] || 0) + 1
  return counts
}

/** Sectors of the rows in the chosen class with their sizes, largest first. */
export function sectorCounts(rows, cls) {
  const counts = new Map()
  for (const row of rows) {
    if (row.sector && (cls === 'all' || row.cls === cls)) {
      counts.set(row.sector, (counts.get(row.sector) || 0) + 1)
    }
  }
  return [...counts].sort((a, b) => b[1] - a[1])
}
