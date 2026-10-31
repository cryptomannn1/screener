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
OKX REST (public)  ──►  internal/okx        client, one rate limiter per endpoint family
                              │
                              ▼
                        internal/collector  universe + poll loops + row computation
                              │        └──────────► internal/store   ClickHouse (ticks, candles, depth, rows)
                              ▼
                        internal/metrics    pure arithmetic (ATR, RV, regime, beta/corr/alpha/presence/stay, depth bands)
                              │
                              ▼
                        internal/api        GET /screener, /instruments, /health
```

Poll loops (all per IP limits from the OKX docs, kept at ~90 %):

| loop | source | cadence | covers |
|------|--------|---------|--------|
| `fast` | `market/tickers`, `public/mark-price`, `market/index-tickers` | 5 s | all instruments in 4 requests; recomputes every row |
| `oi` | `public/open-interest` | 15 s | all; one OI point per minute kept for 26 h → `oiD` |
| `funding` | `public/funding-rate?instId=ANY` | 60 s | all |
| `universe` | `public/instruments` | 10 min | adds listings, drops suspended |
| `hourly` | `market/candles bar=1H` | sweep every 2 min | 300 bars on first visit, then last 3 |
| `daily` | `market/candles bar=1Dutc` | sweep every 15 min | 40 bars first, then last 3 |
| `books` | `market/books sz=400` | continuous sweep, ≥ 30 s | depth bands ±0.1 / 0.5 / 1 %, spread, imbalance |

The benchmark (`-benchmark`, default `BTC-USDT-SWAP`) is always swept first so
the strength block compares against fresh BTC returns.

## Run

```sh
go run ./cmd/screener                        # in-memory, :8090
make ch-up && make run-ch                    # with ClickHouse from docker compose
curl -s localhost:8090/health | jq .
curl -s 'localhost:8090/screener?symbols=BTC,ETH' | jq '.rows[0]'
curl -s 'localhost:8090/screener?sort=chg&period=4h&limit=10' | jq '.rows[] | [.symbol, .chg["4h"], .vol24]'
```

Flags double as environment variables (`-ch-ttl-days` → `CH_TTL_DAYS`):

| flag | default | meaning |
|------|---------|---------|
| `-addr` | `:8090` | HTTP listen address |
| `-log` | `info` | `debug` / `info` / `warn` / `error` |
| `-settle` | `USDT,USDC` | settle currencies of the universe |
| `-benchmark` | `BTC-USDT-SWAP` | instrument for `vsBtc`, `beta`, `corr`, `alpha`, `presence`, `stay` |
| `-books-top` | `0` | sweep order books only for the top N by volume (`0` = all) |
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

Sizing: 517 instruments give ~750 k `ticks` and `rows` per day each and
~1.5 M `depth` rows; all small, flat columns — a few hundred MB a month.

Writes are batched (2000 rows or 2 s) through a queue; a full queue drops and
counts (`/health` → `store.dropped`) instead of slowing the collector.

## Roadmap

1. **Trade stream** (phase 2): OKX WS `trades` for the universe → taker buy
   share and delta per period, tape speed; fills `takerBuy` / `delta`, `cov.flow`.
2. **WebSocket push** of row changes for the widget (today: poll `/screener`).
3. **Strength backtest** over `candles` + `rows`: does `presence` / `beta` / `alpha`
   predict excess return over BTC at 4 h / 24 h / 72 h.
4. Liquidations (OKX `public/liquidation-orders`) and long/short ratio
   (`rubik`, fine from a server).
5. Second venue behind the same row contract.

## Development

```sh
make test      # go vet + go test ./...
make build     # bin/screener
make docker    # image screener:local
```
