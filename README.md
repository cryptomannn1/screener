# screener

Market-data service for the Scope360 terminal screener. It polls the OKX
public API for the whole linear-swap universe (about 520 USDT / USDC
perpetuals), keeps the latest state of every instrument, computes the
screener metrics and serves them as one JSON snapshot. No API keys: every
endpoint used is public.

The widget that renders the rows lives with the terminal; the UI contract is
the prototype in the session folder (`sessions/active/2026-10-31_016_…/screener-v1.html`)
and the wire contract is [`api/screener.md`](api/screener.md).

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
                        internal/api        GET /screener, /stream (SSE), /instruments, /health, /ui/ (static)
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
| `daily` | `market/candles bar=1Dutc` | sweep every 15 min | 230 bars first (MA 200), then last 3 |
| `books` | `market/books sz=400`, `books-full sz=5000` when 400 levels stop short of ±1 % | continuous sweep, ≥ 30 s | depth bands ±0.1 / 0.5 / 1 %, spread, imbalance; a band beyond the snapshot is `null` |

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
go run ./cmd/screener -static ../sessions/active/2026-10-31_016_screener-prototype   # then open /ui/screener-v1.html
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

## Store

With `-clickhouse` every observation is also written to ClickHouse
([`internal/store/schema.sql`](internal/store/schema.sql), applied with
`CREATE … IF NOT EXISTS`):

- `ticks` — per-minute snapshot per instrument (price, mark, index, volume, OI, funding). On restart the last 26 h seed the OI history so `oiD` is available at once.
- `candles` — every fetched bar (`1H`, `1Dutc`), re-fetches replace the forming bar.
- `depth` — the bands of every book sweep.
- `rows` — the computed screener row once a minute, for preset analytics and the strength backtest.
- `flow_1m` — taker buy / sell quote volume and order counts per closed minute from the trade stream.

Sizing: 517 instruments give ~750 k `ticks` and `rows` per day each and
~1.5 M `depth` rows; all small, flat columns — a few hundred MB a month.

Writes are batched (2000 rows or 2 s) through a queue; a full queue drops and
counts (`/health` → `store.dropped`) instead of slowing the collector.

## Status and roadmap

Done: WebSocket feed for price / mark / index / OI / funding / trades with a
1-second recompute, REST safety net, candles, books, levels (day / week /
month open, daily MA 50 / 100 / 200), SSE push, ClickHouse store with restart
seeding, REST + SSE API, static `/ui/`.
See `widget/` for the Vue widget and `cmd/backfill` + `cmd/backtest` for the
strength-vs-BTC study.

Next:

1. Liquidations (OKX `public/liquidation-orders`) and long/short ratio
   (`rubik`, fine from a server) as columns.
2. 5m / 15m periods once the trade stream and 5m candles are stored long enough.
3. Second venue behind the same row contract.
4. Auth / rate limiting in front of the API for a shared dev host.

## Widget

`widget/` is the Vue 3 widget for the terminal (Vue only, terminal token
names, no borders between rows) with a Vite sandbox on :5190 (`make widget`,
or `cd widget/sandbox && npm run dev`). It reads `/stream` (SSE) and falls
back to polling `/screener`; `?server=` overrides the base URL, `?sse=0`
forces polling. `widget/HANDOVER.md` is the mounting guide for the terminal
team.

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
