# screener wire contract — v0.6

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
| `class`   | all     | comma list of asset classes: `crypto`, `stock`, `commodity`, `premarket` (v0.4) |
| `sector`  | all     | comma list of crypto sectors, e.g. `Meme,AI`; see `sector` in the row (v0.4) |

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
| `class` | asset class as the venue lists it (`instCategory` / `ruleType`): `crypto`, `stock` (tokenised equities, ETFs, indices), `commodity`, `premarket`, `other` |
| `name` | project name (crypto only, from the metadata resolver; absent when unknown) |
| `sector` | one sector per coin from CoinGecko categories, first match in the fixed order `Stable, Meme, Exchange, L2, L1, AI, RWA, DePIN, Gaming, Privacy, DeFi, Infra`, else `Other`; `""` for non-crypto rows and before the first sector pass |
| `mcap`, `fdv` | market capitalisation and fully diluted valuation, USD (crypto only; `null` for stocks, indices, commodities and unmatched tickers) |
| `mcapRank` | market-cap rank among all coins |
| `volMcap`, `oiMcap` | `vol24 / mcap`, `oi / mcap` |
| `last`, `mark`, `index`, `bid`, `ask` | prices |
| `basis` | `mark / index − 1` |
| `chg` | `{"1h","4h","24h"}`: `last / price N hours ago − 1`, a rolling window that always spans the whole period. For `1h` and `4h` the price N hours ago is read off the one-minute candle that covers that moment; for `24h` off the 1H candle, as a straight line from its open to its close (the same stands in for `1h` / `4h` in the first minutes after a start, until the one-minute candles are loaded). Before v0.6 the reference was the close of an hourly candle, so `1h` covered only the minutes since the hour began |
| `chgRoll` | `last / open24h − 1`: the venue's own rolling 24 h change, the figure its ticker shows; `chg["24h"]` is the service's own estimate off hourly candles and can differ from it slightly (v0.5) |
| `chg1d`, `chg7d`, `chg30d` | `chg1d`: vs yesterday's UTC daily close, the change of the current day. `chg7d`: rolling 7 × 24 h off the hourly candles (v0.6; from the daily close 7 days ago while fewer than 169 hourly candles are loaded). `chg30d`: vs the UTC daily close 30 days ago |
| `vol24` | 24 h quote volume (`volCcy24h × last`) |
| `volx7` | `vol24 / average quote volume of the last 7 full UTC days` |
| `volx` | per period: volume of the last N hours / the average N-hour volume of the 168 complete hours before them. The window is rolling. `1h` and `4h` are summed off one-minute candles and end when those were last read (about every 2 min); `24h` is summed off hourly candles, the one the window begins in counted by the share inside it. Before v0.6 only the forming candle and the complete ones counted, so `1h` read near zero right after an hour began |
| `oi` | open interest, USD |
| `oiD` | per period: `oi / oi N hours ago − 1`; history is the service's own minute snapshots, seeded at start from the venue's hourly OI history and from the store |
| `oiVol` | `oi / vol24` |
| `funding`, `fundingNext` | current and predicted next rate |
| `fundingAnn` | `funding × (24 h / interval) × 365` |
| `fundingInterval` | hours between settlements |
| `atr` | ATR(14) on 1H bars / `last` |
| `rv24` | annualised std of the last 24 hourly log returns |
| `regime` | true range of the last 24 hours (hourly candles) / median daily true range of the last 20 days (`< 0.6` squeeze, `> 1.6` expansion). Before v0.6 the numerator was the forming UTC day, which has no range yet right after midnight |
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

## `GET /assets` (v0.5)

One row per **asset** over OKX, Binance and Bybit (linear USDT / USDC
perpetuals on each). `/screener` stays as it is: one row per OKX instrument.

| param   | default | notes |
|---------|---------|-------|
| `ex`    | all     | comma list of exchanges to keep: `okx`, `bn`, `bb`; an asset none of them lists is dropped |
| `class` | all     | comma list of `crypto`, `stock`, `commodity`, `fx` |
| `pretty`| —       | any value indents the JSON |

```json
{
  "ts": 1794312139763, "count": 931, "venues": ["okx", "bn", "bb"],
  "assets": [{
    "k": "c:BTC", "s": "BTC", "n": "Bitcoin", "cls": "crypto", "sec": "L1",
    "mcap": 1491288915192, "rank": 1,
    "v": {
      "okx": {"p": 74348.4, "c": -0.0004, "vol": 4658891348, "oi": 2653856813, "f": 0.000042, "fi": 8},
      "bn":  {"p": 74355.7, "c": -0.0002, "vol": 11404719819, "oi": 9158000000, "f": 0.000088, "fi": 8},
      "bb":  {"p": 74356.1, "c": -0.0002, "vol": 3850671351, "oi": 5245193096, "f": 0.000055, "fi": 8}
    },
    "m": {"spark": […], "chg": {"1h": -0.0004, "4h": -0.0046}, "chg7d": -0.05, "chg30d": -0.10,
          "volx7": 1.12, "volx": {…}, "oiD": {…}, "atr": 0.0051, "regime": 0.69,
          "takerBuy": {…}, "delta": {…}, "tape": 284.2, "vsBtc": {"1h": 0, "4h": 0},
          "beta": 1, "corr": 1, "spread": 0.013, "depth05": 112641946,
          "lv": {"do": 75319.9, "wo": 74818.1, "mo": 77520.9, "ma50": 81693.6, "ma100": 77267.0, "ma200": 73167.7}}
  }]
}
```

Rows come busiest first (24 h volume over all exchanges).

| field | meaning |
|-------|---------|
| `k` | asset id: the common ticker with a class prefix, `c:` for coins and `t:` for everything else. The prefix is part of the identity: the same ticker can be a coin on one exchange and a stock on another (`BB`, `CAT`, `ON`, `QNT`) |
| `s` | common ticker. Exchange spellings are brought to one form: contract multipliers are stripped (`1000PEPE`, `1000000MOG`, `1MBABYDOGE`, `SHIB1000` → price of one coin), Bybit's `STOCK` suffix is dropped (`AMDSTOCK` → `AMD`), `COPPER` = `XCU`, `NATGAS` = `NG`, `PAYP` = `PYPL` |
| `n` | name, when known (coins: from the metadata resolver) |
| `cls` | `crypto`, `stock` (equities, ETFs, equity indices, pre-IPO), `commodity`, `fx`. From OKX `instCategory`, Binance `underlyingType`, Bybit `symbolType` |
| `sec` | coins: the resolver's sector, else a sector from Binance's tags, else `Other`; other classes: `Stock`, `ETF`, `Index`, `Pre-IPO`, `Commodity`, `FX` |
| `mcap`, `rank` | market cap in USD and its rank; coins only |
| `v` | one block per exchange that lists the asset, keyed `okx`, `bn`, `bb` |
| `v.*.p` | last price of **one coin**, USD |
| `v.*.c` | the exchange's rolling 24 h change, fraction |
| `v.*.vol`, `v.*.oi` | 24 h volume and open interest in USD. The USDT and USDC contracts of the asset on that exchange add up; price, change and funding are those of the busier contract. `oi` is `null` until read: Binance publishes open interest one symbol per request, a full pass takes a few minutes |
| `v.*.f`, `v.*.fi` | funding rate per interval and the interval in hours. Intervals differ between exchanges and symbols (1, 4, 8 h): compare rates as `f × 8 / fi` |
| `m` | asset-level metrics, the same names and formulas as in the `/screener` row (`chg` carries `1h` and `4h`; the 24 h change is per exchange in `v`). `lv` holds the level **prices**; the distance is `price / level − 1` with the price of the exchange you show |

Where `m` comes from: for an asset OKX lists, from its OKX row (all metrics).
For the others, from the hourly and daily candles of its busiest contract on
Bybit, else Binance, with the same formulas; `vsBtc`, `beta`, `corr` use BTC
on that exchange. Metrics that need trades, the order book or open-interest
history (`takerBuy`, `delta`, `tape`, `spread`, `depth05`, `oiD`) are `null` /
`{}` for those assets until the service collects them from Binance and Bybit.

The service does not add the exchanges up: which of them a table shows is the
reader's choice. The prototype sums `vol` and `oi`, takes price and change
from the busiest of the chosen exchanges and weights funding (brought to 8 h)
by open interest.

`GET /stream?view=assets` pushes the same payload as events named `assets`,
about once a second; `ex` and `class` apply.

Freshness: `v.*.p`, `c`, `vol` and `f` of all three exchanges move about once a
second (public WebSocket streams); `v.bb.oi` too. `v.bn.oi` is as old as the
per-symbol pass over Binance (a few minutes). If an exchange's stream is down
its figures fall back to REST polling every 5 s; `/health` shows which.

## `GET /ui/…`

When the service runs with `-static <dir>`, that directory is served here
(the prototype page: `-static prototype`, then `/ui/screener-v2.html`). Same origin as the API, so the
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
  "store": {"written": 34902, "dropped": 0, "errors": 0, "queued": 0},  // null without -clickhouse
  "venues": {                          // absent with -venues=false
    "instruments": {"okx": 528, "bn": 794, "bb": 890},   // okx: assets on OKX; bn, bb: linear perpetuals
    "assets": 931, "outsideOkx": 403,
    "candles": 403,                    // assets outside OKX with hourly and daily candles loaded
    "minutes": 403,                    // of them, with an hour of one-minute candles loaded (v0.6)
    "binanceOi": 794,                  // Binance symbols with an open-interest reading
    "loops": {"universe": {…}, "binance.tickers": {…}, "binance.premium": {…}, "binance.oi": {…}, "bybit.tickers": {…}, "candles": {…}},
    "streams": {                       // absent with -venue-streams=false
      "bn": {"live": true, "conns": 1, "msgs": 5120, "updates": 412300, "lastMsgAt": 1794323235029, "reconnects": 0},
      "bb": {"live": true, "conns": 2, "subscribed": 890, "msgs": 2551000, "updates": 610400, "lastMsgAt": 1794323235134, "reconnects": 0}
    }                                  // live: every socket is up; updates: ticker rows applied; lastErr when a socket failed
  }
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
