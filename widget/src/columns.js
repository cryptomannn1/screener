// The column catalogue. A column is `{ label, title, end, unit, text, level,
// quick, width, value(row), cell(row) }`:
//   value  what the column sorts and filters by; a column without it does neither
//   cell   what the table draws, see `format.js`
//   unit   'usd' | 'pct' | 'x' | 'bps', how a condition value is typed and shown
//   quick  usual thresholds offered as one-click answers: [operator, value, label?]
//   end    right-aligned number column; these share the free width of the table
// Rows come from `model/aggregate.js`.

import { fundingCell, isNum, levelCell, multipleCell, pctCell, plainCell, priceText, usdCell, usdText, fundingText } from './format.js'
import { EXCHANGES, LEVELS, LEVEL_TITLE } from './model/constants.js'

const per = (field, period) => (row) => (row[field] ? row[field][period] : null)
const MOVE_3 = [['>=', 0.03], ['<=', -0.03]]
const MOVE_10 = [['>=', 0.1], ['<=', -0.1]]
const VS_BTC = [['>=', 0.02], ['<=', -0.02]]
const TAKER = [['>=', 0.6], ['<=', 0.4]]
const DELTA = [['>=', 0.1], ['<=', -0.1]]
const BUSY = [['>=', 2], ['>=', 5]]

const pctCol = (label, value, o = {}) => ({
  label,
  end: true,
  unit: 'pct',
  value,
  cell: (row) => pctCell(value(row), o.digits ?? 2, o.signed !== false),
  quick: o.quick,
  title: o.title,
})
const usdCol = (label, value, quick, title) => ({ label, end: true, unit: 'usd', value, cell: (row) => usdCell(value(row)), quick, title })
const xCol = (label, value, quick, title) => ({ label, end: true, unit: 'x', value, cell: (row) => multipleCell(value(row)), quick, title })
const numCol = (label, value, digits, title) => ({ label, end: true, value, cell: (row) => plainCell(value(row), digits), title })

export const COLUMNS = {
  symbol: { label: 'Symbol', pin: true, text: true, value: (row) => row.base },
  sector: {
    label: 'Sector',
    text: true,
    title: 'Crypto sector, or the kind of instrument for stocks, indices, commodities and FX',
    value: (row) => row.sector || null,
    cell: (row) =>
      row.sector
        ? { text: row.sector, tone: row.cls === 'crypto' && row.sector !== 'Other' ? '' : 'mid', title: row.name }
        : null,
  },
  // Drawn by PriceCell: the digits run to the new value on a live tick.
  mark: { label: 'Price', end: true, price: true, width: 118, title: 'Last price on the busiest of the selected exchanges, in USD', value: (row) => row.mark },
  chg1h: pctCol('Chg 1h', per('chg', '1h'), { quick: MOVE_3, title: 'Price change over the last 60 minutes' }),
  chg4h: pctCol('Chg 4h', per('chg', '4h'), { quick: MOVE_3, title: 'Price change over the last 4 hours' }),
  chg24: pctCol('Chg 24h', per('chg', '24h'), { quick: MOVE_3, title: 'Price change over 24 hours on the busiest of the selected exchanges' }),
  chg7d: pctCol('Chg 7D', (row) => row.chg7d, { quick: MOVE_10 }),
  chg30d: pctCol('Chg 30D', (row) => row.chg30d, { quick: MOVE_10 }),
  vol24: usdCol('Vol 24h', (row) => row.vol24, [['>=', 20e6], ['>=', 100e6]], 'Traded over 24 hours on the selected exchanges together, in USD'),
  volx1h: xCol('Vol × 1h', per('volx', '1h'), BUSY, 'Volume of the last 60 minutes against the usual hour of the last 7 days'),
  volx4h: xCol('Vol × 4h', per('volx', '4h'), BUSY, 'Volume of the last 4 hours against the usual 4 hours of the last 7 days'),
  volx7: xCol('Vol × 7D', (row) => row.volx7, BUSY, 'Volume of the day against its 7-day average'),
  oi: usdCol('OI', (row) => row.oi, [['>=', 10e6], ['>=', 50e6]], 'Open interest on the selected exchanges together, in USD'),
  oiD1h: pctCol('OI Δ 1h', per('oiD', '1h'), { quick: MOVE_3 }),
  oiD4h: pctCol('OI Δ 4h', per('oiD', '4h'), { quick: MOVE_3 }),
  oiD24h: pctCol('OI Δ 24h', per('oiD', '24h'), { quick: MOVE_3 }),
  oiMcap: pctCol('OI/Mcap', (row) => row.oiMcap, { digits: 1, signed: false }),
  funding: {
    label: 'Funding',
    end: true,
    unit: 'pct',
    title: 'Funding rate per 8 hours, weighted by open interest across the selected exchanges',
    quick: [['>=', 0.0003], ['<=', -0.0003]],
    value: (row) => row.funding,
    cell: (row) => fundingCell(row.funding),
  },
  fundingAnn: pctCol('Funding APR', (row) => row.fundingAnn, { digits: 1, quick: [['>=', 0.3], ['<=', -0.1]] }),
  mcap: usdCol('Mcap', (row) => row.mcap, [['>=', 1e9], ['<=', 3e8]], 'Market capitalisation in USD, crypto only'),
  mcapRank: { label: 'Rank', end: true, value: (row) => row.mcapRank, cell: (row) => (isNum(row.mcapRank) ? { text: '#' + row.mcapRank, tone: '' } : null) },
  atr: pctCol('ATR %', (row) => row.atr, { signed: false }),
  regime: xCol('Vol regime', (row) => row.regime, [['<=', 0.7], ['>=', 1.6]], 'Range of the last 24 hours against its 20-day norm'),
  taker1h: pctCol('Taker buy 1h', per('takerBuy', '1h'), { digits: 0, signed: false, quick: TAKER }),
  taker4h: pctCol('Taker buy 4h', per('takerBuy', '4h'), { digits: 0, signed: false, quick: TAKER }),
  taker24h: pctCol('Taker buy 24h', per('takerBuy', '24h'), { digits: 0, signed: false, quick: TAKER }),
  delta1h: pctCol('Delta 1h', per('delta', '1h'), { digits: 1, quick: DELTA }),
  delta4h: pctCol('Delta 4h', per('delta', '4h'), { digits: 1, quick: DELTA }),
  delta24h: pctCol('Delta 24h', per('delta', '24h'), { digits: 1, quick: DELTA }),
  tape: numCol('Tape /min', (row) => row.tape, 0),
  vs1h: pctCol('vs BTC 1h', per('vsBtc', '1h'), { quick: VS_BTC }),
  vs4h: pctCol('vs BTC 4h', per('vsBtc', '4h'), { quick: VS_BTC }),
  vs24h: pctCol('vs BTC 24h', per('vsBtc', '24h'), { quick: VS_BTC, title: "Price change over 24 hours minus BTC's" }),
  beta: numCol('Beta', (row) => row.beta, 2),
  corr: numCol('Corr', (row) => row.corr, 2),
  spread: { label: 'Spread', end: true, unit: 'bps', quick: [['<=', 3]], value: (row) => row.spread, cell: (row) => plainCell(row.spread, 1, ' bps') },
  depth05: usdCol('Depth ±0.5%', (row) => row.depth05, [['>=', 1e6]]),
  venues: {
    label: 'Listed',
    end: true,
    title: 'On how many of the selected exchanges the asset trades',
    quick: [['<=', 1, 'One exchange'], ['>=', 3, 'All three']],
    value: (row) => row.venues.length,
    cell: (row) => ({ text: String(row.venues.length), tone: '' }),
  },
  fGap: {
    label: 'Funding gap',
    end: true,
    unit: 'pct',
    title: 'Widest difference in the 8-hour funding rate between the selected exchanges',
    quick: [['>=', 0.0001], ['>=', 0.0005]],
    value: (row) => row.fGap,
    cell: (row) => (isNum(row.fGap) ? { text: (row.fGap * 100).toFixed(4) + '%', tone: '' } : null),
  },
  pxGap: {
    label: 'Price gap',
    end: true,
    unit: 'bps',
    title: 'Widest price difference between the selected exchanges, in basis points',
    quick: [['>=', 10], ['>=', 30]],
    value: (row) => row.pxGap,
    cell: (row) => plainCell(row.pxGap, 1, ' bps'),
  },
}

for (const [key, label] of LEVELS) {
  COLUMNS[key] = {
    label,
    end: true,
    level: true,
    title: LEVEL_TITLE[key],
    value: (row) => (row.lv[key] ? row.lv[key].dist : null),
    cell: (row) => levelCell(row.lv[key]),
  }
}

// Each exchange as its own column. A figure of an exchange that is switched off is empty.
const block = (row, code) => (row.venues.includes(code) ? row.byVenue[code] : null)
for (const [code, name] of EXCHANGES) {
  COLUMNS['vol_' + code] = usdCol('Vol ' + name, (row) => block(row, code)?.vol ?? null, [['>=', 20e6], ['>=', 100e6]], `Traded over 24 hours on ${name}, in USD`)
  COLUMNS['oi_' + code] = usdCol('OI ' + name, (row) => block(row, code)?.oi ?? null, [['>=', 10e6], ['>=', 50e6]], `Open interest on ${name}, in USD`)
  COLUMNS['f_' + code] = {
    label: 'Funding ' + name,
    end: true,
    unit: 'pct',
    title: `Funding rate on ${name}, brought to 8 hours`,
    quick: [['>=', 0.0003], ['<=', -0.0003]],
    value: (row) => (isNum(row.f8[code]) ? row.f8[code] : null),
    cell: (row) => fundingCell(row.f8[code]),
  }
}

export const DEFAULT_COLUMN_KEYS = ['symbol', 'mark', 'chg24', 'vol24', 'oi', 'funding']

/** Any column of the table can carry a condition: numbers, levels, the sector. */
export function isFilterable(key) {
  const column = COLUMNS[key]
  if (!column || key === 'symbol') return false
  return Boolean(column.level || key === 'sector' || (column.value && !column.text))
}

// The hover card of a summed cell: the same figure on each exchange that went into it.
export const VENUE_FIGURES = {
  mark: (x) => (isNum(x.p) ? '$' + priceText(x.p) : '—'),
  chg24: (x) => (isNum(x.c) ? (x.c > 0 ? '+' : '') + (x.c * 100).toFixed(2) + '%' : '—'),
  vol24: (x) => (isNum(x.vol) ? usdText(x.vol) : '—'),
  oi: (x) => (isNum(x.oi) ? usdText(x.oi) : '—'),
  funding: (x) => (isNum(x.f) ? fundingText((x.f * 8) / (x.fi || 8)) : '—'),
}
/** Price and change are taken from one exchange, the rest add up. */
export const VENUE_LEAD_COLUMNS = new Set(['mark', 'chg24'])

// The catalogue a newcomer reads: what the metric is in one line; periods are chips of one family.
const P3 = (prefix) => [[prefix + '1h', '1h'], [prefix + '4h', '4h'], [prefix + '24h', '24h']]
export const CATALOG = [
  { group: 'Price', items: [
    { name: 'Price', desc: 'Last price in USD', keys: [['mark', '']] },
    { name: 'Change', desc: 'Price change over the period', keys: [['chg1h', '1h'], ['chg4h', '4h'], ['chg24', '24h'], ['chg7d', '7D'], ['chg30d', '30D']] },
  ] },
  { group: 'Volume', items: [
    { name: 'Volume 24h', desc: 'Traded over the last day on the selected exchanges, in USD', keys: [['vol24', '']] },
    { name: 'Volume vs usual', desc: 'Against its 7-day norm: 2× is twice the usual', keys: [['volx1h', '1h'], ['volx4h', '4h'], ['volx7', '24h']] },
  ] },
  { group: 'Open interest', items: [
    { name: 'Open interest', desc: 'Open positions on the selected exchanges, in USD', keys: [['oi', '']] },
    { name: 'OI change', desc: 'Positions opened or closed over the period', keys: P3('oiD') },
    { name: 'OI to market cap', desc: 'How much leverage sits on the coin', keys: [['oiMcap', '']] },
  ] },
  { group: 'Funding', items: [
    { name: 'Funding rate', desc: 'What longs pay shorts, brought to 8 hours', keys: [['funding', '']] },
    { name: 'Funding APR', desc: 'The same rate as a yearly percentage', keys: [['fundingAnn', '']] },
  ] },
  { group: 'By exchange', items: [
    { name: 'Listings', desc: 'On how many of the selected exchanges it trades', keys: [['venues', '']] },
    { name: 'Volume by exchange', desc: 'Where the asset actually trades', keys: EXCHANGES.map(([code, name]) => ['vol_' + code, name]) },
    { name: 'Open interest by exchange', desc: 'Where the positions sit', keys: EXCHANGES.map(([code, name]) => ['oi_' + code, name]) },
    { name: 'Funding by exchange', desc: 'Rate per 8 hours on each one and the gap between them', keys: [...EXCHANGES.map(([code, name]) => ['f_' + code, name]), ['fGap', 'Gap']] },
    { name: 'Price gap', desc: 'Widest price difference between exchanges, in basis points', keys: [['pxGap', '']] },
  ] },
  { group: 'Asset', items: [
    { name: 'Sector', desc: 'Crypto sector, or the class for stocks and commodities', keys: [['sector', '']] },
    { name: 'Market cap', desc: 'Capitalisation in USD and its rank, crypto only', keys: [['mcap', 'Value'], ['mcapRank', 'Rank']] },
  ] },
  { group: 'Levels', items: [
    { name: 'Opens', desc: 'Open of the day, week, month and the distance to it', keys: [['do', 'Day'], ['wo', 'Week'], ['mo', 'Month']] },
    { name: 'Daily moving averages', desc: 'The average and the distance to it', keys: [['ma50', '50'], ['ma100', '100'], ['ma200', '200']] },
  ] },
  { group: 'Volatility', items: [
    { name: 'ATR', desc: 'Typical hourly move as a share of price', keys: [['atr', '']] },
    { name: 'Volatility regime', desc: 'Range of the last 24 hours against its norm: below 0.7 is a squeeze', keys: [['regime', '']] },
  ] },
  { group: 'Flow', items: [
    { name: 'Taker buy share', desc: 'Share of volume bought at market', keys: P3('taker') },
    { name: 'Delta', desc: 'Market buying minus market selling', keys: P3('delta') },
    { name: 'Tape speed', desc: 'Market orders per minute', keys: [['tape', '']] },
  ] },
  { group: 'Against BTC', items: [
    { name: 'Ahead or behind BTC', desc: "Price change minus BTC's over the period", keys: P3('vs') },
    { name: 'Beta and correlation', desc: 'How strongly it follows BTC', keys: [['beta', 'Beta'], ['corr', 'Corr']] },
  ] },
  { group: 'Order book', items: [
    { name: 'Spread', desc: 'Gap between best bid and ask, in basis points', keys: [['spread', '']] },
    { name: 'Depth', desc: 'Resting orders within ±0.5% of price', keys: [['depth05', '']] },
  ] },
]
