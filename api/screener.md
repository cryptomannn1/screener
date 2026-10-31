# screener wire contract — v0.1

Plain HTTP GET on `-addr` (default `:8090`), JSON, `Access-Control-Allow-Origin: *`,
`OPTIONS` answers 204. Timestamps are Unix milliseconds. Prices in the quote
currency (USDT / USDC), volumes, open interest and depth in **quote currency**
(USD-equivalent). Ratios are plain fractions: `0.012` = +1.2 %. A metric that
cannot be computed yet is `null`, never `0`.

## `GET /screener`

```
GET /screener?symbols=BTC,ETH&sort=chg&period=24h&dir=desc&limit=50
```

| param     | default | notes |
|-----------|---------|-------|
| `symbols` | all     | comma list of base coins (`BTC`) or instrument ids (`BTC-USDT-SWAP`) |
| `sort`    | `vol24` | `vol24`, `oi`, `chg`, `oiD`, `vsBtc`, `volx`, `funding`, `fundingAnn`, `atr`, `rv24`, `regime`, `beta`, `presence`, `depth05`, `spread`, `listed` |
| `period`  | `24h`   | horizon for per-period sort keys: `1h`, `4h`, `24h` |
| `dir`     | `desc`  | `asc` / `desc`; `null` sorts last |
| `limit`   | all     | keep the first N after sorting |

Response:

```json
{
  "ts": 1793479038481,
  "periods": ["1h", "4h", "24h"],
  "benchmark": "BTC-USDT-SWAP",
  "count": 2, "total": 2,
  "coverage": {"hourly": 517, "daily": 517, "book": 517, "oiHist": 0},
  "rows": [ { …row… } ]
}
```

`coverage` counts rows for which each data family has arrived (see `cov`
below); right after start the candle and book sweeps take 1–2 minutes and
`oiHist` needs an hour of running (or a store with history).

### Row

Field names equal the prototype's column keys. Per-period metrics are objects
keyed by `periods`.

| field | meaning |
|-------|---------|
| `instId`, `symbol`, `settle` | `BTC-USDT-SWAP`, `BTC`, `USDT` |
| `lev` | max leverage |
| `listed` | days since listing |
| `last`, `mark`, `index`, `bid`, `ask` | prices |
| `basis` | `mark / index − 1` |
| `chg` | `{"1h","4h","24h"}`: `last / close N hours ago − 1` (1H candles) |
| `chg1d`, `chg7d`, `chg30d` | vs the UTC daily close 1 / 7 / 30 days ago |
| `vol24` | 24 h quote volume (`volCcy24h × last`) |
| `volx7` | `vol24 / average quote volume of the last 7 full UTC days` |
| `volx` | per period: volume of the last N hours / its 7-day average for the same window |
| `oi` | open interest, USD |
| `oiD` | per period: `oi / oi N hours ago − 1` from the service's own OI history |
| `oiVol` | `oi / vol24` |
| `funding`, `fundingNext` | current and predicted next rate |
| `fundingAnn` | `funding × (24 h / interval) × 365` |
| `fundingInterval` | hours between settlements |
| `atr` | ATR(14) on 1H bars / `last` |
| `rv24` | annualised std of the last 24 hourly log returns |
| `regime` | today's true range / median daily true range of the last 20 days (`< 0.6` squeeze, `> 1.6` expansion) |
| `range1d` | today's `(high − low) / low` |
| `toHigh`, `toLow` | `last / 30-day high − 1`, `last / 30-day low − 1` |
| `vsBtc` | per period: `chg − chg(benchmark)` |
| `beta`, `corr` | regression slope / Pearson correlation of hourly log returns on the benchmark, 168 bars |
| `alpha` | mean hourly excess over `beta × benchmark` |
| `presence` | share of hours the instrument beat the benchmark |
| `stay` | probability of beating the benchmark next hour given it beat this hour |
| `depth01`, `depth05`, `depth1` | resting notional within ±0.1 / 0.5 / 1 % of mid, both sides, from a 400-level book |
| `imb` | `(bid05 − ask05) / (bid05 + ask05)` |
| `spread` | top-of-book spread, bps |
| `takerBuy`, `delta` | per period; **phase 2** (trade stream), `null` for now |
| `updated` | ms of the ticker |
| `cov` | `{hourly, daily, book, oiHist, flow}` booleans per row |

Sorting is by `vol24` descending unless `sort` is given.

## `GET /instruments`

The universe: OKX `SWAP` instruments with `ctType = linear`, `state = live`
and `settleCcy` in `-settle` (default `USDT,USDC`), with `ctVal`, `tickSz`,
`lever`, `listTime`.

## `GET /health`

```json
{
  "status": "ok",                      // "warming" + 503 until the first rows
  "version": "0.1.0",
  "collector": {
    "universe": 517, "rows": 517, "rowsAt": 1793479038481, "uptimeSec": 29,
    "benchmark": "BTC-USDT-SWAP",
    "coverage": {"hourly": 89, "daily": 88, "book": 85, "oiHist": 0},
    "loops": {
      "fast":     {"lastOk": 1793479038482, "passes": 6, "errors": 0, "items": 517},
      "oi":       {…}, "funding": {…}, "universe": {…},
      "hourly":   {…}, "daily": {…}, "books": {…}
    }
  },
  "store": {"written": 34902, "dropped": 0, "errors": 0, "queued": 0}   // null without -clickhouse
}
```

Each loop reports its last success, last error, pass and error counts and the
number of instruments covered by the last pass.

## Compatibility

Additive changes only: new fields may appear, existing ones keep their name
and unit. A breaking change bumps the major version in this file's title.
