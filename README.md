# screener

Market-data service for the Scope360 terminal screener. It reads the public
APIs of OKX, Binance and Bybit for their linear USDT / USDC perpetuals (about
930 assets: coins, stocks and indices, commodities, FX), keeps the latest
state of every instrument, computes the screener metrics and serves them as
JSON: one row per OKX instrument (`GET /screener`) and one row per asset over
the three exchanges (`GET /assets`). No API keys: every endpoint used is
public.

- UI contract: the prototype in [`prototype/`](prototype/), one HTML page with
  a clean table over the three exchanges, filter phrases and ready tabs.
- Wire contract: [`api/screener.md`](api/screener.md).
- Vue component for the screener page of the terminal: [`widget/`](widget/),
  a port of the prototype with a sandbox on :5190 (`make widget`). In the
  terminal the screener is a page of its own, not a workspace widget; the
  folder keeps its first name.
- Where the project stands and what comes next: [`PLAN.md`](PLAN.md) (in Russian).

Quick start: `go run ./cmd/screener -static prototype`, then open
http://localhost:8090/ui/screener-v2.html.

## Architecture

```
OKX REST (public)  ──►  internal/okx        REST client, one rate limiter per endpoint family
OKX WS  trades     ──►  internal/okx/ws     trade stream over a few sockets → collector.Flow (per-minute taker buckets)
                              │
                              ▼
                        internal/collector  universe + poll loops + row computation
                              │        └──────────► internal/store   ClickHouse (ticks, candles, depth, rows)
                              ▼
                        internal/metrics    pure arithmetic (ATR, RV, regime, beta/corr/alpha/presence/stay, depth bands)
                              │
                              ▼
                        internal/api        GET /screener, /assets, /stream (SSE), /instruments, /health, /ui/ (static)
                              ▲
Binance, Bybit WS+REST ► internal/venue     tickers, funding, open interest, candles of the two exchanges;
                                            folds them with the collector's OKX rows into one row per asset
```

Poll loops (all per IP limits from the OKX docs, kept at ~90 %):

| loop | source | cadence | covers |
|------|--------|---------|--------|
| `stream` | WS `tickers`, `mark-price`, `open-interest`, `funding-rate`, `trades` for every instrument (3 sockets × ≤ 200) and `index-tickers` per underlying | live | price, mark, index, OI, funding and taker flow as the venue pushes them; rows recompute every second |
| `fast` | `market/tickers`, `public/mark-price`, `market/index-tickers` | 30 s (5 s with `-flow=false`) | REST safety net; a value applies only if newer than the stream's |
| `oi` | `public/open-interest` | 15 s | safety net; one OI point per minute kept for 26 h → `oiD` |
| `oiBackfill` | `rubik/stat/contracts/open-interest-history` (1H, 30 bars) | once at start | fills the last day of OI history for instruments that lack it, so `oiD` 4h / 24h read within minutes of a cold start |
| `funding` | `public/funding-rate?instId=ANY` | 60 s | safety net |
| `universe` | `public/instruments` | 10 min | adds listings, drops suspended |
| `hourly` | `market/candles bar=1H` | sweep every 2 min | 300 bars on first visit, then last 3 |
| `minute` | `market/candles bar=1m` | sweep every 2 min | 300 bars (five hours) on first visit, then every bar since the last one; the 1 h and 4 h windows of `chg`, `vsBtc` and `volx` are measured on them; kept in memory only |
| `daily` | `market/candles bar=1Dutc` | sweep every 15 min | 230 bars first (MA 200), then last 3 |
| `books` | `market/books sz=400`, `books-full sz=5000` when 400 levels stop short of ±1 % | continuous sweep, ≥ 30 s | depth bands ±0.1 / 0.5 / 1 %, spread, imbalance; a band beyond the snapshot is `null` |
| `meta` | CoinGecko `coins/markets` (top 2000 by cap, then the categories of a fixed sector taxonomy) | caps every 30 min, sectors every 24 h; one call per 13 s without a key | `mcap`, `fdv`, `mcapRank`, `sector`, `name` for crypto rows; the asset `class` itself comes from the venue's `instCategory` (crypto / stock / commodity / pre-market) |

Binance and Bybit (`internal/venue`, public WebSocket streams and REST, no keys):

| loop | source | cadence | covers |
|------|--------|---------|--------|
| `universe` | Binance `exchangeInfo` + `fundingInfo`, Bybit `instruments-info` | 30 min | trading linear USDT / USDC perpetuals, asset class, funding interval |
| stream `bn` | WS `!ticker@arr` + `!markPrice@arr@1s` on one socket (`wss://fstream.binance.com/market/stream`) | live, once a second | price, 24 h change, volume of every symbol that traded; mark price and funding rate of every symbol |
| stream `bb` | WS `tickers.<symbol>` for every instrument (`wss://stream.bybit.com/v5/public/linear`, 2 sockets for ~900 symbols) | live, snapshot then deltas | price, mark, 24 h change, volume, open interest, funding and its interval |
| `binance.tickers` | `ticker/24hr` (all symbols, weight 40) | 60 s while the stream is healthy, 5 s without it | REST safety net: the symbols the stream has not mentioned, delisted ones dropped |
| `binance.premium` | `premiumIndex` (all symbols, weight 10) | 60 s while the stream is healthy, 30 s without it | REST safety net for mark price and funding rate |
| `binance.oi` | `openInterest?symbol=` | continuous, 4 requests / s, busiest first | open interest; there is no bulk endpoint, a pass takes a little over 3 min |
| `bybit.tickers` | `tickers?category=linear` | 60 s while the stream is healthy, 5 s without it | REST safety net for the same figures |
| `candles` | Bybit `kline`, else Binance `klines` | 200 hourly and 240 daily bars once, then the last few every 10 / 30 min and right after each hour and UTC day turns; 300 one-minute bars once, then the fresh ones every 2 min | price metrics of the assets OKX does not list |

A stream counts as healthy while its sockets are up and spoke within the last 5 s; a stream that went quiet hands
the feed back to REST until it returns. Sockets reconnect by themselves with a backoff of 1 to 30 s. Bybit caps
the topic names of one connection at 21,000 characters, so its symbols are spread over connections of at most
12,000; a changed symbol set re-subscribes. With the streams on the Binance weight stays near 300 of the 2400 per
minute allowed per IP (near 800 on REST alone).

The benchmark (`-benchmark`, default `BTC-USDT-SWAP`) is always swept first so
the strength block compares against fresh BTC returns.

On start with a store the service restores the last 26 h of OI, the last
300 hourly and 40 daily bars per instrument and the last 25 h of taker minutes,
so a restart costs seconds, not a re-backfill. The taker window counts as
continuous only if the stored minutes reach within 5 minutes of now.

## Run

```sh
go run ./cmd/screener                        # in-memory, :8090
make ch-up && make run-ch                    # with ClickHouse from docker compose
curl -s localhost:8090/health | jq .
curl -s 'localhost:8090/screener?symbols=BTC,ETH' | jq '.rows[0]'
curl -s 'localhost:8090/screener?sort=chg&period=4h&limit=10' | jq '.rows[] | [.symbol, .chg["4h"], .vol24]'
curl -N 'localhost:8090/stream?symbols=BTC,ETH'                      # SSE, one `rows` event per recompute
go run ./cmd/screener -static prototype     # the UI prototype: open /ui/screener-v2.html
```

Flags double as environment variables (`-ch-ttl-days` → `CH_TTL_DAYS`):

| flag | default | meaning |
|------|---------|---------|
| `-addr` | `:8090` | HTTP listen address |
| `-log` | `info` | `debug` / `info` / `warn` / `error` |
| `-settle` | `USDT,USDC` | settle currencies of the universe |
| `-benchmark` | `BTC-USDT-SWAP` | instrument for `vsBtc`, `beta`, `corr`, `alpha`, `presence`, `stay` |
| `-books-top` | `0` | sweep order books only for the top N by volume (`0` = all) |
| `-flow` | `true` | use the WebSocket stream; `false` = REST polling only |
| `-ws-per-conn` | `200` | instruments per WebSocket connection |
| `-compute` | `1s` | row recompute period with the stream on |
| `-static` | — | directory served under `/ui/` (the prototype page) |
| `-fast`, `-oi`, `-hourly`, `-daily`, `-books` | see table | loop periods |
| `-clickhouse` | — | DSN `clickhouse://user:pass@host:9000/db`; empty = no store |
| `-ch-ttl-days` | `180` | retention of `ticks`, `depth`, `rows` (candles keep 730 days) |
| `-ch-optional` | `false` | run in memory if the store fails to open |
| `-venues` | `true` | poll Binance and Bybit and serve `GET /assets`: one row per asset over the three exchanges |
| `-venue-streams` | `true` | read Binance and Bybit tickers and mark prices over WebSocket, about once a second; `false` = REST polling only |
| `-venue-tickers` | `5s` | Binance and Bybit REST tickers period when their stream is off or down |
| `-binance-base`, `-bybit-base` | production | API base URLs of the two exchanges |
| `-meta` | `true` | resolve market cap / FDV / sector for crypto rows from CoinGecko |
| `-cg-key` | — | CoinGecko demo or pro key; without it the public limit applies (one call per 13 s, first full pass ≈ 8 min, cached in the store) |
| `-cg-base` | `https://api.coingecko.com/api/v3` | API base; `https://pro-api.coingecko.com/api/v3` with a pro key |
| `-cg-pages` | `8` | market pages of 250 coins scanned when matching tickers |
| `-cg-mcap`, `-cg-sectors` | `30m`, `24h` | refresh periods |

## Store

With `-clickhouse` every observation is also written to ClickHouse
([`internal/store/schema.sql`](internal/store/schema.sql), applied with
`CREATE … IF NOT EXISTS`):

- `ticks` — per-minute snapshot per instrument (price, mark, index, volume, OI, funding). On restart the last 26 h seed the OI history so `oiD` is available at once.
- `candles` — every fetched bar (`1H`, `1Dutc`), re-fetches replace the forming bar.
- `depth` — the bands of every book sweep.
- `rows` — the computed screener row once a minute, for preset analytics and the strength backtest.
- `flow_1m` — taker buy / sell quote volume and order counts per closed minute from the trade stream.
- `assets` — market cap, FDV, rank and sector per crypto symbol from the metadata resolver; a restart reads it back so rows carry caps before the first CoinGecko pass.

Sizing: 517 instruments give ~750 k `ticks` and `rows` per day each and
~1.5 M `depth` rows; all small, flat columns — a few hundred MB a month.

Writes are batched (2000 rows or 2 s) through a queue; a full queue drops and
counts (`/health` → `store.dropped`) instead of slowing the collector.

## Status and roadmap

Done: WebSocket feed for price / mark / index / OI / funding / trades with a
1-second recompute, REST safety net, candles, books, levels (day / week /
month open, daily MA 50 / 100 / 200), SSE push, ClickHouse store with restart
seeding, REST + SSE API, static `/ui/`.
See `widget/` for the Vue component and `cmd/backfill` + `cmd/backtest` for the
strength-vs-BTC study.
Three exchanges: `GET /assets` gives one row per asset over OKX, Binance and
Bybit with a block per exchange (price, 24 h change, volume, open interest,
funding) and the asset-level price metrics; tickers, multipliers and asset
classes are brought to one form (`internal/venue`). Prices of all three
exchanges move once a second: OKX, Binance and Bybit are read over their
public WebSocket streams, REST is the safety net.

Next:

1. Liquidations (OKX `public/liquidation-orders`) and long/short ratio
   (`rubik`, fine from a server) as columns.
2. 5m / 15m periods once the trade stream and 5m candles are stored long enough.
3. Trades, order books and open-interest history from Binance and Bybit, so that flow, depth and OI-change
   metrics exist for the assets OKX does not list; store the two exchanges' data in ClickHouse (now in memory).
4. Auth / rate limiting in front of the API for a shared dev host.

## Vue component

`widget/` is the Vue 3 component of the terminal's screener page (Vue only, terminal token
names, no borders between rows) with a Vite sandbox on :5190 (`make widget`,
or `cd widget/sandbox && npm run dev`). It shows the three exchanges in one
table: it reads `/assets`, listens to `/stream?view=assets` (SSE) and falls
back to polling; `?server=` overrides the base URL, `?sse=0` forces polling.
`widget/HANDOVER.md` tells the terminal team how to mount it as a page.

## Backfill and backtest

```sh
make backfill    # top 150 USDT swaps, 1H, 180 days of history-candles into `candles` (15–20 min, resumable)
make backtest    # strength-vs-BTC study → backtest-report.md
```

`cmd/backfill` pages `market/history-candles` backwards per instrument and
writes through the store; with `-resume` (default) it only extends beyond the
oldest stored bar, the service keeps the newest side fresh. `cmd/backtest`
(`internal/backtest`) aligns hourly bars on the benchmark, computes the
strength signals on 3 / 7 / 14-day windows and measures rank IC, quintile
spreads and a preset portfolio against excess return over BTC at 4 / 24 /
72 h; see the report's reading notes. One caveat built in: ranking by
`vsBtc` is identical to ranking by own momentum (BTC's return is the same for
every instrument in a given hour), so vs-BTC only matters as a threshold.

## Development

```sh
make test      # go vet + go test ./...
make build     # bin/screener
make docker    # image screener:local
```
