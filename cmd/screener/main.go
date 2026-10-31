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
	chDSN := fs.String("clickhouse", "", "ClickHouse DSN; empty = no store")
	chTTL := fs.Int("ch-ttl-days", 180, "store retention for ticks / depth / rows")
	chOptional := fs.Bool("ch-optional", false, "run without the store if it fails to open")
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

	cli := okx.New(log)
	col := collector.New(cli, collector.Config{
		FastEvery: *fast, OIEvery: *oiEvery, HourlyEvery: *hourlyEvery, DailyEvery: *dailyEvery,
		BooksEvery: *booksEvery, BooksTop: *booksTop, Settle: splitList(*settle), BenchmarkInstID: *benchmark,
	}, log, sinks...)

	if st != nil {
		seedOI(ctx, st, col, log)
	}

	srv := &http.Server{Addr: *addr, Handler: api.New(col, st, version), ReadHeaderTimeout: 5 * time.Second}
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
