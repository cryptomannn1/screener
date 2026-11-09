// Command screener collects OKX linear-swap market data for the whole
// universe, computes the screener metrics and serves them over HTTP.
//
//	go run ./cmd/screener                                  # in-memory only
//	go run ./cmd/screener -clickhouse clickhouse://default:screener@localhost:9000/screener
//
// Every flag can be set as an environment variable with the same name
// upper-cased and dashes as underscores (-ch-ttl-days → CH_TTL_DAYS).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"screener/internal/api"
	"screener/internal/collector"
	"screener/internal/meta"
	"screener/internal/okx"
	"screener/internal/store"
)

const version = "0.1.0"

func main() {
	fs := flag.NewFlagSet("screener", flag.ExitOnError)
	addr := fs.String("addr", ":8090", "HTTP listen address")
	logLevel := fs.String("log", "info", "debug | info | warn | error")
	settle := fs.String("settle", "USDT,USDC", "settle currencies of the universe")
	benchmark := fs.String("benchmark", "BTC-USDT-SWAP", "benchmark instrument for strength metrics")
	booksTop := fs.Int("books-top", 0, "sweep order books only for the top N by volume; 0 = all")
	fast := fs.Duration("fast", 5*time.Second, "tickers / mark / index period")
	oiEvery := fs.Duration("oi", 15*time.Second, "open-interest period")
	hourlyEvery := fs.Duration("hourly", 2*time.Minute, "1H candles refresh pass period")
	dailyEvery := fs.Duration("daily", 15*time.Minute, "1Dutc candles refresh pass period")
	booksEvery := fs.Duration("books", 30*time.Second, "order-book sweep period")
	flow := fs.Bool("flow", true, "use the OKX WebSocket stream (tickers, mark, index, OI, funding, trades); off = REST only")
	computeEvery := fs.Duration("compute", time.Second, "row recompute period with the stream on")
	wsPerConn := fs.Int("ws-per-conn", 200, "instruments per WebSocket connection")
	static := fs.String("static", "", "directory served under /ui/ (prototype page); empty = off")
	chDSN := fs.String("clickhouse", "", "ClickHouse DSN; empty = no store")
	chTTL := fs.Int("ch-ttl-days", 180, "store retention for ticks / depth / rows")
	chOptional := fs.Bool("ch-optional", false, "run without the store if it fails to open")
	metaOn := fs.Bool("meta", true, "resolve market cap / FDV / sector for crypto rows from CoinGecko")
	cgBase := fs.String("cg-base", "https://api.coingecko.com/api/v3", "CoinGecko API base URL (pro-api.coingecko.com/api/v3 with a pro key)")
	cgKey := fs.String("cg-key", "", "CoinGecko demo or pro API key; empty = public rate limits")
	cgPages := fs.Int("cg-pages", 8, "market-cap pages of 250 coins to scan when matching symbols")
	cgMcapEvery := fs.Duration("cg-mcap", 30*time.Minute, "market-cap refresh period")
	cgSectorsEvery := fs.Duration("cg-sectors", 24*time.Hour, "sector refresh period")
	applyEnv(fs)
	_ = fs.Parse(os.Args[1:])

	log := newLogger(*logLevel)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var sinks []collector.Sink
	var st *store.Store
	if *chDSN != "" {
		s, err := store.Open(ctx, store.Config{DSN: *chDSN, TTLDays: *chTTL}, log)
		if err != nil {
			if !*chOptional {
				log.Error("store", "err", err)
				os.Exit(1)
			}
			log.Warn("store unavailable, running in memory", "err", err)
		} else {
			st = s
			sinks = append(sinks, s)
			log.Info("store connected", "ttlDays", *chTTL)
		}
	}

	// Asset metadata (market cap, FDV, sector) for the crypto part of the universe.
	var assets *meta.Service
	if *metaOn {
		var cache meta.Cache
		if st != nil {
			cache = st
		}
		assets = meta.New(meta.Config{
			BaseURL: *cgBase, APIKey: *cgKey, Pages: *cgPages, McapEvery: *cgMcapEvery, SectorsEvery: *cgSectorsEvery,
		}, log, cache)
		assets.Seed(ctx)
	}

	cli := okx.New(log)
	cfg := collector.Config{
		FastEvery: *fast, OIEvery: *oiEvery, HourlyEvery: *hourlyEvery, DailyEvery: *dailyEvery,
		BooksEvery: *booksEvery, BooksTop: *booksTop, Settle: splitList(*settle), BenchmarkInstID: *benchmark,
		Flow: *flow, WSPerConn: *wsPerConn, ComputeEvery: *computeEvery,
	}
	if assets != nil {
		cfg.Assets = assets
	}
	col := collector.New(cli, cfg, log, sinks...)

	if st != nil {
		seedOI(ctx, st, col, log)
		seedCandles(ctx, st, col, log)
		seedFlow(ctx, st, col, log)
	}
	if assets != nil {
		go func() {
			if err := assets.Run(ctx, col.CryptoSymbols); err != nil && !errors.Is(err, context.Canceled) {
				log.Warn("meta loop", "err", err)
			}
		}()
	}

	handler := api.New(col, st, version, *static)
	if assets != nil {
		handler.WithMeta(assets)
	}
	srv := &http.Server{Addr: *addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Info("listening", "addr", *addr, "version", version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http", "err", err)
			stop()
		}
	}()

	err := col.Run(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Error("collector", "err", err)
	}
	shctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shctx)
	if st != nil {
		if err := st.Close(); err != nil {
			log.Warn("store close", "err", err)
		}
	}
	log.Info("bye")
}

func seedOI(ctx context.Context, st *store.Store, col *collector.Collector, log *slog.Logger) {
	lctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pts, err := st.LoadOI(lctx, time.Now().Add(-26*time.Hour))
	if err != nil {
		log.Warn("seed oi", "err", err)
		return
	}
	for _, p := range pts {
		col.SeedOI(p.InstID, p.TS, p.USD)
	}
	log.Info("seeded oi history", "points", len(pts))
}

func seedCandles(ctx context.Context, st *store.Store, col *collector.Collector, log *slog.Logger) {
	lctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	n := 0
	for _, bar := range []struct {
		bar   string
		since time.Duration
		limit int
	}{{"1H", 320 * time.Hour, 300}, {"1Dutc", 240 * 24 * time.Hour, 230}} {
		m, err := st.RecentCandles(lctx, bar.bar, time.Now().Add(-bar.since), bar.limit)
		if err != nil {
			log.Warn("seed candles", "bar", bar.bar, "err", err)
			continue
		}
		for id, cs := range m {
			col.SeedCandles(id, bar.bar, cs)
			n += len(cs)
		}
	}
	log.Info("seeded candles", "bars", n)
}

func seedFlow(ctx context.Context, st *store.Store, col *collector.Collector, log *slog.Logger) {
	lctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	rows, err := st.RecentFlow(lctx, time.Now().Add(-25*time.Hour))
	if err != nil {
		log.Warn("seed flow", "err", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	f := col.Flow()
	oldest, newest := rows[0].Minute, rows[0].Minute
	for _, r := range rows {
		f.Seed(r.InstID, r.Minute, r.BuyQuote, r.SellQuote, int(r.Trades), int(r.BuyTrades))
		if r.Minute < oldest {
			oldest = r.Minute
		}
		if r.Minute > newest {
			newest = r.Minute
		}
	}
	// Only count the restored range as continuous if it reaches (almost) now.
	if gap := time.Now().Unix()/60 - newest; gap <= 5 {
		f.SetSince(time.Unix(oldest*60, 0))
		log.Info("seeded flow", "minutes", len(rows), "since", time.Unix(oldest*60, 0).UTC().Format(time.RFC3339))
	} else {
		log.Info("seeded flow buckets but gap too large to count as continuous", "minutes", len(rows), "gapMin", gap)
	}
}

func applyEnv(fs *flag.FlagSet) {
	fs.VisitAll(func(f *flag.Flag) {
		name := strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
		if v, ok := os.LookupEnv(name); ok {
			if err := fs.Set(f.Name, v); err != nil {
				fmt.Fprintf(os.Stderr, "env %s: %v\n", name, err)
				os.Exit(2)
			}
		}
	})
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, strings.ToUpper(p))
		}
	}
	return out
}

func newLogger(level string) *slog.Logger {
	var lv slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lv = slog.LevelDebug
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lv}))
}
