# screener wire contract — v0.3

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
| `oiD` | per period: `oi / oi N hours ago − 1`; history is the service's own minute snapshots, seeded at start from the venue's hourly OI history and from the store |
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
| `takerBuy` | per period: taker buy quote volume / total taker volume, from the trade stream (`null` until the stream has run for the period) |
| `delta` | per period: `(taker buy − taker sell) / total`, net aggressive pressure |
| `trades` | per period: number of taker orders |
| `tape` | taker orders per minute over the last 5 minutes (the "Ticks" of tape speed) |
| `spark` | last 24 hourly closes, oldest first, forming bar last; `null` without candles |
| `do`, `wo`, `mo` | `{px, dist}`: today's UTC daily open, the open of the current ISO week (Monday 00:00 UTC), the open of the current month; `dist = last / px − 1` |
| `ma50`, `ma100`, `ma200` | `{px, dist}`: simple moving average of daily closes including the forming day, and the distance of last from it |
| `updated` | ms of the ticker |
| `cov` | `{hourly, daily, book, oiHist, flow}` booleans per row; `flow` = the stream has run for an hour and the instrument traded in it |

Sorting is by `vol24` descending unless `sort` is given.

`pretty=1` indents the JSON.

## `GET /stream`

Server-Sent Events with the same query parameters as `/screener`. One event
named `rows` whose `data` is the `/screener` payload, sent every time the
collector recomputes (about every 5 s, only when the snapshot changed), plus a
`: ping` comment every 15 s. Browser:

```js
const es = new EventSource('http://localhost:8090/stream?limit=200');
es.addEventListener('rows', e => render(JSON.parse(e.data)));
```

## `GET /ui/…`

When the service runs with `-static <dir>`, that directory is served here
(the prototype page: `/ui/screener-v1.html`). Same origin as the API, so the
page needs no CORS.

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
    "coverage": {"hourly": 89, "daily": 88, "book": 85, "oiHist": 0, "flow": 0},
    "stream": {"connections": 3, "live": 3, "instruments": 517, "msgs": 2978, "trades": 2458,
               "reconnects": 0, "startedAt": 1793480639645, "lastMsgAt": 1793480660730},
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
number of instruments covered by the last pass. With the WebSocket stream on
(default) `stream` carries the per-instrument channels (tickers, mark-price,
open-interest, funding-rate, trades) and `indexStream` the index tickers;
`byChannel` counts messages per channel. The REST loops `fast`, `oi` and
`funding` then run as a safety net and apply a value only when it is newer
than what the stream delivered, so their `items` are usually 0.

Rows are recomputed every second (`-compute`) when the stream is on, every
`-fast` period otherwise; `ts` is the time of the last recompute.

## Compatibility

Additive changes only: new fields may appear, existing ones keep their name
and unit. A breaking change bumps the major version in this file's title.
