// The column catalogue. Keys follow the wire contract (`api/screener.md`);
// per-period metrics (`chg`, `volx`, `oiD`, `vsBtc`, `takerBuy`, `delta`) are
// read for the period the user picked, so their keys end in `P`.
//
// A column with `value` is numeric: sortable and filterable. `unit` tells the
// filter popover how to parse and show a typed value; `signed` lets the sort
// cycle offer "by size" (absolute value, descending). `label` may be a function
// of the context `{ period, benchmark }`.

import * as f from './format.js'

export const GROUPS = [
  'Instrument',
  'Price',
  'Volume',
  'Open interest',
  'Funding',
  'Volatility',
  'Flow',
  'Strength vs BTC',
  'Levels',
  'Order book',
  'Listing',
]

const field = (name) => (row) => row[name] ?? null
const perPeriod = (name) => (row, ctx) => row[name]?.[ctx.period] ?? null

/** A price level `{ px, dist }`: the cell shows the level price and, below it, `dist = last / px − 1`; sorts by dist. */
const level = (name, label, def, title) => ({
  key: name,
  group: 'Levels',
  label,
  unit: 'pct',
  signed: true,
  def,
  title,
  value: (row) => row[name]?.dist ?? null,
  format: (_v, row) => f.level(row[name]),
})

/** Price and taker flow disagree: price up with net selling (-1), price down with net buying (+1). */
function divergence(row, ctx) {
  const change = row.chg?.[ctx.period]
  const delta = row.delta?.[ctx.period]
  if (change == null || delta == null) return null
  if (change > 0.001 && delta < -0.05) return -1
  if (change < -0.001 && delta > 0.05) return 1
  return 0
}

export const COLUMNS = [
  {
    key: 'symbol',
    group: 'Instrument',
    label: 'Symbol',
    pin: 'left',
    def: true,
    width: 150,
    title: 'Instrument. Click a row to switch the linked chart, order book and order form.',
  },
  {
    key: 'spark',
    group: 'Instrument',
    label: '24h',
    def: true,
    title: 'Last 24 hours of hourly closes against the price 24 hours ago: green above it, red below.',
  },
  {
    key: 'mark',
    group: 'Price',
    label: 'Mark',
    unit: 'price',
    def: true,
    title: 'Mark price in USD: the price liquidations and PnL are computed from.',
    value: field('mark'),
    format: f.dollarPrice,
  },
  {
    key: 'last',
    group: 'Price',
    label: 'Last',
    unit: 'price',
    def: false,
    title: 'Last traded price in USD.',
    value: field('last'),
    format: f.dollarPrice,
  },
  {
    key: 'chgP',
    group: 'Price',
    label: (ctx) => `Chg ${ctx.period}`,
    unit: 'pct',
    signed: true,
    def: true,
    title: 'Price change over the selected period, from 1H candles.',
    value: perPeriod('chg'),
    format: (v) => f.pct(v),
  },
  {
    key: 'chg1d',
    group: 'Price',
    label: 'Chg 1D',
    unit: 'pct',
    signed: true,
    def: true,
    title: 'Change against the UTC daily close one day ago.',
    value: field('chg1d'),
    format: (v) => f.pct(v),
  },
  {
    key: 'chg7d',
    group: 'Price',
    label: 'Chg 7D',
    unit: 'pct',
    signed: true,
    def: true,
    title: 'Change against the UTC daily close 7 days ago.',
    value: field('chg7d'),
    format: (v) => f.pct(v),
  },
  {
    key: 'chg30d',
    group: 'Price',
    label: 'Chg 30D',
    unit: 'pct',
    signed: true,
    def: false,
    title: 'Change against the UTC daily close 30 days ago.',
    value: field('chg30d'),
    format: (v) => f.pct(v),
  },
  {
    key: 'basis',
    group: 'Price',
    label: 'Basis',
    unit: 'pct',
    signed: true,
    def: false,
    title: 'Mark against index: premium of the perpetual over spot.',
    value: field('basis'),
    format: (v) => f.pct(v, 3),
  },
  {
    key: 'vol24',
    group: 'Volume',
    label: 'Vol 24h',
    unit: 'usd',
    def: true,
    title: 'Traded volume over 24 hours in USD.',
    value: field('vol24'),
    format: f.usd,
  },
  {
    key: 'volx7',
    group: 'Volume',
    label: 'Vol × 7D',
    unit: 'x',
    def: true,
    title:
      '24h volume divided by the average daily volume of the last 7 full UTC days. Above 1 is busier than usual.',
    value: field('volx7'),
    format: f.multiple,
  },
  {
    key: 'volxP',
    group: 'Volume',
    label: (ctx) => `Vol × ${ctx.period}`,
    unit: 'x',
    def: false,
    title:
      'Volume of the selected period against the average of the same window over the previous 7 days.',
    value: perPeriod('volx'),
    format: f.multiple,
  },
  {
    key: 'oi',
    group: 'Open interest',
    label: 'OI',
    unit: 'usd',
    def: true,
    title: 'Open interest in USD.',
    value: field('oi'),
    format: f.usd,
  },
  {
    key: 'oiDP',
    group: 'Open interest',
    label: (ctx) => `OI Δ ${ctx.period}`,
    unit: 'pct',
    signed: true,
    def: true,
    title:
      'Change of open interest over the selected period. Read together with price change: up and up is new longs, down and up is a short squeeze.',
    value: perPeriod('oiD'),
    format: (v) => f.pct(v),
  },
  {
    key: 'oiD1d',
    group: 'Open interest',
    label: 'OI Δ 1D',
    unit: 'pct',
    signed: true,
    def: true,
    title: 'Change of open interest over the last 24 hours, independent of the period switch.',
    value: (row) => row.oiD?.['24h'] ?? null,
    format: (v) => f.pct(v),
  },
  {
    key: 'oiVol',
    group: 'Open interest',
    label: 'OI / Vol',
    unit: 'x',
    def: false,
    title: 'Open interest divided by 24h volume. High values mean positions are large against turnover.',
    value: field('oiVol'),
    format: f.multiple,
  },
  {
    key: 'funding',
    group: 'Funding',
    label: 'Funding',
    unit: 'pct',
    signed: true,
    def: true,
    title: 'Current funding rate per interval. Positive: longs pay shorts.',
    value: field('funding'),
    format: f.funding,
  },
  {
    key: 'fundingAnn',
    group: 'Funding',
    label: 'Funding APR',
    unit: 'pct',
    signed: true,
    def: false,
    title: "Current funding rate annualised with the instrument's funding interval.",
    value: field('fundingAnn'),
    format: (v) => f.pct(v, 1),
  },
  {
    key: 'fundingNext',
    group: 'Funding',
    label: 'Next',
    unit: 'pct',
    signed: true,
    def: false,
    title: 'Predicted next funding rate.',
    value: field('fundingNext'),
    format: f.funding,
  },
  {
    key: 'atr',
    group: 'Volatility',
    label: 'ATR %',
    unit: 'pct',
    def: true,
    title: 'ATR 14 on 1H bars as a share of the last price. The normal hourly step of price, a stop size.',
    value: field('atr'),
    format: (v) => f.pct(v, 2, false),
  },
  {
    key: 'rv24',
    group: 'Volatility',
    label: 'RV 24h',
    unit: 'pct',
    def: false,
    title: 'Realised volatility: annualised standard deviation of the last 24 hourly log returns.',
    value: field('rv24'),
    format: (v) => f.pct(v, 0, false),
  },
  {
    key: 'regime',
    group: 'Volatility',
    label: 'Vol regime',
    unit: 'x',
    def: true,
    title:
      "Today's true range against the median daily true range of the last 20 days. Below 0.6 is a squeeze, above 1.6 an expansion.",
    value: field('regime'),
    format: f.regime,
  },
  {
    key: 'range1d',
    group: 'Volatility',
    label: 'Range 1D',
    unit: 'pct',
    def: false,
    title: "Today's high minus low as a share of the low.",
    value: field('range1d'),
    format: (v) => f.pct(v, 1, false),
  },
  {
    key: 'toHigh',
    group: 'Volatility',
    label: 'To High',
    unit: 'pct',
    signed: true,
    def: false,
    title: 'Last price against the 30-day high. Negative: below the high by this much.',
    value: field('toHigh'),
    format: (v) => f.pct(v, 1),
  },
  {
    key: 'toLow',
    group: 'Volatility',
    label: 'To Low',
    unit: 'pct',
    signed: true,
    def: false,
    title: 'Last price against the 30-day low. Positive: above the low by this much.',
    value: field('toLow'),
    format: (v) => f.pct(v, 1),
  },
  {
    key: 'takerP',
    group: 'Flow',
    label: (ctx) => `Taker buy ${ctx.period}`,
    unit: 'pct',
    def: true,
    start: true,
    title:
      'Share of taker (market) volume that bought over the selected period. Filled by the trade stream, phase 2.',
    value: perPeriod('takerBuy'),
    format: f.share,
  },
  {
    key: 'deltaP',
    group: 'Flow',
    label: (ctx) => `Delta ${ctx.period}`,
    unit: 'pct',
    signed: true,
    def: true,
    title:
      'Taker buy minus taker sell over the period, as a share of period volume. Net aggressive pressure. Filled by the trade stream, phase 2.',
    value: perPeriod('delta'),
    format: (v) => f.pct(v, 0),
  },
  {
    key: 'diverg',
    group: 'Flow',
    label: 'Divergence',
    unit: 'num',
    signed: true,
    def: false,
    start: true,
    title:
      'Price and delta disagree over the period: price up with net selling, or price down with net buying.',
    value: divergence,
    format: f.divergence,
  },
  {
    key: 'vsBtcP',
    group: 'Strength vs BTC',
    label: (ctx) => `vs ${ctx.benchmark} ${ctx.period}`,
    unit: 'pct',
    signed: true,
    def: true,
    title: 'Instrument change minus benchmark change over the period, in percentage points.',
    value: perPeriod('vsBtc'),
    format: (v) => f.pct(v),
  },
  {
    key: 'beta',
    group: 'Strength vs BTC',
    label: 'Beta',
    unit: 'num',
    def: true,
    title:
      'Regression slope of hourly log returns on the benchmark over 168 bars. On a 1% benchmark move the instrument moves beta percent.',
    value: field('beta'),
    format: (v) => f.number(v, 2),
  },
  {
    key: 'alpha',
    group: 'Strength vs BTC',
    label: 'Alpha 7D',
    unit: 'pct',
    signed: true,
    def: false,
    title: 'Mean hourly excess return over beta times the benchmark, in percentage points.',
    value: field('alpha'),
    format: (v) => f.pct(v, 3),
  },
  {
    key: 'presence',
    group: 'Strength vs BTC',
    label: 'Presence',
    unit: 'pct',
    def: true,
    title: 'Share of hours the instrument beat the benchmark. 50% is noise.',
    value: field('presence'),
    format: (v) => f.pct(v, 0, false),
  },
  {
    key: 'stay',
    group: 'Strength vs BTC',
    label: 'Stay',
    unit: 'pct',
    def: false,
    title:
      'Probability of beating the benchmark next hour given it beat this hour. Persistence of strength.',
    value: field('stay'),
    format: (v) => f.pct(v, 0, false),
  },
  {
    key: 'corr',
    group: 'Strength vs BTC',
    label: 'Corr 7D',
    unit: 'num',
    def: false,
    title: 'Pearson correlation of hourly log returns with the benchmark over 168 bars.',
    value: field('corr'),
    format: (v) => f.number(v, 2),
  },
  level('do', 'DO', true, "Distance from last to today's UTC daily open."),
  level('wo', 'WO', true, 'Distance from last to the open of the current ISO week, Monday 00:00 UTC.'),
  level('mo', 'MO', true, 'Distance from last to the open of the current month, UTC.'),
  level('ma50', 'MA 50', true, 'Distance from last to the 50-day simple moving average of daily closes, forming day included.'),
  level('ma100', 'MA 100', false, 'Distance from last to the 100-day simple moving average of daily closes, forming day included.'),
  level('ma200', 'MA 200', true, 'Distance from last to the 200-day simple moving average of daily closes, forming day included.'),
  {
    key: 'depth01',
    group: 'Order book',
    label: 'Depth 0.1%',
    unit: 'usd',
    def: true,
    title:
      'Resting notional within 0.1% of mid, both sides, in USD. What a market order of that size would walk through.',
    value: field('depth01'),
    format: f.usd,
  },
  {
    key: 'depth05',
    group: 'Order book',
    label: 'Depth 0.5%',
    unit: 'usd',
    def: false,
    title: 'Resting notional within 0.5% of mid, both sides, in USD.',
    value: field('depth05'),
    format: f.usd,
  },
  {
    key: 'depth1',
    group: 'Order book',
    label: 'Depth 1%',
    unit: 'usd',
    def: false,
    title: 'Resting notional within 1% of mid, both sides, in USD.',
    value: field('depth1'),
    format: f.usd,
  },
  {
    key: 'imb',
    group: 'Order book',
    label: 'Imbalance',
    unit: 'pct',
    signed: true,
    def: false,
    title: 'Bids minus asks over bids plus asks within 0.5%. Positive: more resting buy liquidity.',
    value: field('imb'),
    format: (v) => f.pct(v, 0),
  },
  {
    key: 'spread',
    group: 'Order book',
    label: 'Spread',
    unit: 'bps',
    def: true,
    title: 'Best ask minus best bid in basis points of mid.',
    value: field('spread'),
    format: f.bps,
  },
  {
    key: 'lev',
    group: 'Listing',
    label: 'Max lev',
    unit: 'num',
    def: false,
    title: 'Maximum leverage the venue allows.',
    value: field('lev'),
    format: f.leverage,
  },
  {
    key: 'listed',
    group: 'Listing',
    label: 'Listed',
    unit: 'days',
    def: false,
    title: 'Days since listing.',
    value: field('listed'),
    format: f.days,
  },
  {
    key: 'actions',
    group: 'Instrument',
    label: '',
    pin: 'right',
    def: true,
    width: 36,
    title: '',
  },
]

/** Symbol and actions are always drawn; the column picker never lists them. */
export const FIXED_COLUMN_KEYS = ['symbol', 'actions']

export const NUMERIC_COLUMNS = COLUMNS.filter((column) => column.value)

export const DEFAULT_COLUMN_KEYS = COLUMNS.filter((column) => column.def).map(
  (column) => column.key,
)

const byKey = new Map(COLUMNS.map((column) => [column.key, column]))

export function findColumn(key) {
  return byKey.get(key) ?? null
}

export function columnLabel(column, ctx) {
  return typeof column.label === 'function' ? column.label(ctx) : column.label
}

/** Numeric cells are right-aligned unless the column asks for the start. */
export function isEndAligned(column) {
  return Boolean(column.value) && !column.start
}

/** The number a sort or filter reads for this row, or null when unavailable. */
export function readValue(column, row, ctx) {
  const value = column.value?.(row, ctx)
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}
