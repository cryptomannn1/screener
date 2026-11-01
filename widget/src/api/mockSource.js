// Deterministic synthetic rows shaped exactly like `GET /screener`, so the
// widget renders every column without the Go service. The same seed always
// produces the same universe; each tick nudges prices a little so the status
// line's "Updated" moves. Phase-2 fields (`takerBuy`, `delta`) and `spark`
// are filled here, which the live server does not do yet.
//
// Same interface as `serverSource.js`: `{ label, start(onRows, onError), stop() }`.

const BASES = (
  'BTC ETH SOL XRP DOGE BNB ADA AVAX LINK SUI HYPE ZEC ENA NEAR PUMP LIT PEPE WIF ARB OP APT TIA ' +
  'SEI TON TRX LTC BCH DOT UNI AAVE TAO FET RENDER INJ ONDO JUP WLD ORDI STX FIL ATOM ETC XLM ' +
  'HBAR ALGO CRV LDO MKR PENDLE EIGEN ETHFI STRK ZK BLUR JTO PYTH BOME FLOKI SHIB BONK NOT TRUMP ' +
  'VIRTUAL AIXBT KAITO IP BERA S MOVE ME PNUT ACT GOAT POPCAT MEW NEIRO CAT TURBO DOGS HMSTR CATI ' +
  'BIGTIME MAGIC IMX GALA SAND MANA AXS APE GMT DYDX GMX SNX COMP YFI SUSHI 1INCH ZRO W OM POL KAS ' +
  'ICP XMR RUNE THETA GRT ENS ARKM TNSR DRIFT IO ZRX RAY ORCA JASMY CHZ ANKR MINA ROSE CELO KAVA ZIL ONE'
).split(' ')

const PERIODS = { '1h': 1, '4h': 4, '24h': 24 }
const BENCHMARK = 'BTC-USDT-SWAP'
const HOUR_MS = 3_600_000

/** Park–Miller LCG: the same seed gives the same sequence in every browser. */
function createRandom(seed) {
  let state = seed % 2147483647 || 1
  const next = () => (state = (state * 16807) % 2147483647) / 2147483647
  // A rough bell curve, enough for returns that look like returns.
  next.gauss = () => (next() + next() + next() + next() - 2) * 1.2
  return next
}

/** Hourly closes (oldest first) of an instrument that follows the benchmark with `beta` plus its own noise. */
function walk(random, benchmarkReturns, beta, ownVol, lastPrice) {
  const returns = benchmarkReturns.map(
    (r) => beta * r + random.gauss() * ownVol + (random() < 0.08 ? random.gauss() * 0.02 : 0),
  )
  const closes = []
  let price = lastPrice / returns.reduce((acc, r) => acc * (1 + r), 1)
  for (const r of returns) {
    price *= 1 + r
    closes.push(price)
  }
  return closes
}

function periodMap(fn) {
  return Object.fromEntries(Object.entries(PERIODS).map(([period, hours]) => [period, fn(hours)]))
}

/** `{ px, dist }` for a price level, or both null when the level is unknown. */
function levelOf(last, px) {
  return px == null ? { px: null, dist: null } : { px, dist: last / px - 1 }
}

/** Period opens read back from the hourly closes; moving averages as a plausible offset. */
function levels(random, closes, listedDays, now) {
  const n = closes.length
  const last = closes[n - 1]
  const date = new Date(now)
  const hoursToday = Math.max(1, date.getUTCHours())
  const hoursThisWeek = ((date.getUTCDay() + 6) % 7) * 24 + hoursToday
  const hoursThisMonth = (date.getUTCDate() - 1) * 24 + hoursToday
  const openAgo = (hours) => (hours < n ? closes[n - 1 - hours] : null)
  const ma = (days) => (listedDays >= days ? last * (1 + random.gauss() * Math.sqrt(days) * 0.012) : null)
  return {
    do: levelOf(last, openAgo(hoursToday)),
    wo: levelOf(last, openAgo(hoursThisWeek)),
    mo: levelOf(last, openAgo(hoursThisMonth) ?? last * (1 + random.gauss() * 0.08)),
    ma50: levelOf(last, ma(50)),
    ma100: levelOf(last, ma(100)),
    ma200: levelOf(last, ma(200)),
  }
}

function buildRow(random, base, index, benchmarkCloses, benchmarkReturns, now) {
  const isBenchmark = base === 'BTC'
  const last = isBenchmark ? 77600 : base === 'ETH' ? 2250 : Math.exp(random.gauss() * 2 + (index < 10 ? 2.5 : -0.5))
  const beta = isBenchmark ? 1 : 0.6 + random() * 2.2
  const ownVol = 0.003 + random() * 0.012
  const closes = isBenchmark ? benchmarkCloses : walk(random, benchmarkReturns, beta, ownVol, last)
  const n = closes.length
  const change = (hours) => closes[n - 1] / closes[n - 1 - hours] - 1
  const benchmarkChange = (hours) => benchmarkCloses[n - 1] / benchmarkCloses[n - 1 - hours] - 1
  const vol24 = (isBenchmark ? 2.0e9 : base === 'ETH' ? 1.6e9 : 3e8 / Math.pow(index + 1, 0.7)) * (0.6 + random() * 0.8)
  const oi = vol24 * (0.3 + random() * 1.5)
  const funding = random.gauss() * 0.00012 + 0.00005
  const fundingInterval = random() < 0.2 ? 4 : 8
  const high30 = Math.max(...closes) * (1 + random() * 0.08)
  const low30 = Math.min(...closes) * (1 - random() * 0.08)
  const regime = Math.exp(random.gauss() * 0.5)
  const takerBuy = periodMap((hours) => 0.5 + change(hours) * 6 + random.gauss() * 0.04)
  const depth05 = vol24 * (0.004 + random() * 0.02)
  const imb = random.gauss() * 0.25
  const listedDays = isBenchmark ? 2545 : 30 + random() * 1500

  return {
    instId: `${base}-USDT-SWAP`,
    symbol: base,
    settle: 'USDT',
    lev: [20, 50, 75, 100][Math.floor(random() * 4)],
    listed: listedDays,
    last: closes[n - 1],
    mark: closes[n - 1] * (1 + random.gauss() * 0.00005),
    index: closes[n - 1] * (1 + random.gauss() * 0.0003),
    bid: closes[n - 1] * (1 - 0.00005),
    ask: closes[n - 1] * (1 + 0.00005),
    basis: random.gauss() * 0.0004,
    chg: periodMap(change),
    chg1d: change(24) * (0.8 + random() * 0.4),
    chg7d: change(168),
    chg30d: change(168) * (1 + random.gauss() * 0.8),
    vol24,
    volx7: 0.4 + random() * 1.6,
    volx: periodMap(() => 0.3 + random() * 2.2),
    oi,
    oiD: periodMap((hours) => random.gauss() * 0.004 * Math.sqrt(hours)),
    oiVol: oi / vol24,
    funding,
    fundingAnn: funding * (24 / fundingInterval) * 365,
    fundingNext: funding + random.gauss() * 0.00003,
    fundingInterval,
    atr: ownVol * (1.2 + random() * 0.6),
    rv24: ownVol * Math.sqrt(24 * 365) * (0.8 + random() * 0.4),
    regime,
    range1d: ownVol * (3 + random() * 4),
    toHigh: closes[n - 1] / high30 - 1,
    toLow: closes[n - 1] / low30 - 1,
    vsBtc: periodMap((hours) => change(hours) - benchmarkChange(hours)),
    beta,
    alpha: isBenchmark ? 0 : random.gauss() * 0.0003,
    presence: isBenchmark ? null : 0.3 + random() * 0.4,
    stay: isBenchmark ? null : 0.3 + random() * 0.4,
    corr: isBenchmark ? 1 : 0.4 + random() * 0.55,
    depth01: depth05 * (0.2 + random() * 0.3),
    depth05,
    depth1: index % 9 === 8 ? null : depth05 * (1.4 + random() * 0.6),
    imb,
    spread: 0.5 + random() * (index < 20 ? 3 : 25),
    takerBuy: Object.fromEntries(
      Object.entries(takerBuy).map(([period, share]) => [period, Math.min(0.9, Math.max(0.1, share))]),
    ),
    delta: Object.fromEntries(
      Object.entries(takerBuy).map(([period, share]) => [period, Math.min(0.9, Math.max(0.1, share)) * 2 - 1]),
    ),
    ...levels(random, closes, listedDays, now),
    spark: closes.slice(-24),
    updated: now,
    cov: { hourly: true, daily: true, book: true, oiHist: true, flow: true },
  }
}

export function buildMockRows(seed = 20261031, now = Date.now()) {
  const random = createRandom(seed)
  const benchmarkReturns = Array.from({ length: 200 }, () => random.gauss() * 0.004)
  const benchmarkCloses = walk(random, benchmarkReturns, 1, 0, 77600)
  return BASES.map((base, index) =>
    buildRow(random, base, index, benchmarkCloses, benchmarkReturns, now - (index % 3) * 1000),
  )
}

/** A small deterministic price nudge per tick, so the table visibly refreshes. */
function nudge(rows, tick, now) {
  const random = createRandom(7919 + tick)
  return rows.map((row) => {
    const step = 1 + random.gauss() * row.atr * 0.1
    const last = row.last * step
    return {
      ...row,
      last,
      mark: row.mark * step,
      bid: last * (1 - 0.00005),
      ask: last * (1 + 0.00005),
      chg: Object.fromEntries(Object.entries(row.chg).map(([p, v]) => [p, (1 + v) * step - 1])),
      updated: now,
    }
  })
}

export function createMockSource({ pollMs = 5000, seed = 20261031 } = {}) {
  let timer = null
  let tick = 0
  let rows = null

  function emit(onRows) {
    const now = Date.now()
    rows = rows ? nudge(rows, (tick += 1), now) : buildMockRows(seed, now - HOUR_MS)
    onRows({
      ts: now,
      periods: Object.keys(PERIODS),
      benchmark: BENCHMARK,
      count: rows.length,
      total: rows.length,
      coverage: { hourly: rows.length, daily: rows.length, book: rows.length, oiHist: rows.length },
      rows,
    })
  }

  return {
    label: 'Mock',
    start(onRows) {
      emit(onRows)
      timer = setInterval(() => emit(onRows), pollMs)
    },
    stop() {
      clearInterval(timer)
      timer = null
    },
  }
}
