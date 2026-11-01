// Command backfill loads hourly candle history from the OKX public REST API
// into the ClickHouse candles table, so the strength backtest has more than
// the ~300 bars the service keeps in memory.
//
//	go run ./cmd/backfill -top 150 -days 180                 # ~15–20 min, 44 requests per instrument
//	go run ./cmd/backfill -top 3 -days 5                     # smoke test
//
// The universe is every live linear swap settled in USDT / USDC, ranked by 24 h
// quote volume; -top keeps the first N and the benchmark is always included.
// For each instrument the tool pages market/history-candles backwards from
// the oldest stored bar (or from now with -resume=false) until the bars are
// older than -days or the venue runs out of history. Every page goes through
// Store.OnCandles; the table is a ReplacingMergeTree, so re-inserting a bar is
// harmless. Ctrl-C stops the workers and flushes what was fetched.
//
// Every flag can be set as an environment variable with the same name
// upper-cased and dashes as underscores (-days → DAYS).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"screener/internal/okx"
	"screener/internal/store"
)

const defaultDSN = "clickhouse://default:screener@localhost:9000/screener"

func main() {
	fs := flag.NewFlagSet("backfill", flag.ExitOnError)
	chDSN := fs.String("clickhouse", defaultDSN, "ClickHouse DSN")
	days := fs.Int("days", 180, "how far back to fetch")
	top := fs.Int("top", 150, "top N instruments by 24h quote volume; 0 = all live linear USDT/USDC swaps")
	bar := fs.String("bar", "1H", "OKX bar id")
	parallel := fs.Int("parallel", 4, "instruments fetched concurrently (the client's limiter still caps the total rate)")
	resume := fs.Bool("resume", true, "start from the oldest stored bar instead of from now")
	settle := fs.String("settle", "USDT,USDC", "settle currencies of the universe")
	benchmark := fs.String("benchmark", "BTC-USDT-SWAP", "always included in the universe")
	logLevel := fs.String("log", "info", "debug | info | warn | error")
	applyEnv(fs)
	_ = fs.Parse(os.Args[1:])

	log := newLogger(*logLevel)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, log, options{
		dsn: *chDSN, days: *days, top: *top, bar: *bar, parallel: *parallel, resume: *resume,
		settle: splitList(*settle), benchmark: *benchmark,
	}); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("backfill", "err", err)
		os.Exit(1)
	}
}

type options struct {
	dsn       string
	days      int
	top       int
	bar       string
	parallel  int
	resume    bool
	settle    []string
	benchmark string
}

func run(ctx context.Context, log *slog.Logger, o options) error {
	if o.parallel < 1 {
		o.parallel = 1
	}
	st, err := store.Open(ctx, store.Config{DSN: o.dsn}, log)
	if err != nil {
		return err
	}
	defer func() {
		if err := st.Close(); err != nil {
			log.Warn("store close", "err", err)
		}
	}()
	cli := okx.New(log)

	universe, err := rankUniverse(ctx, cli, o)
	if err != nil {
		return err
	}
	log.Info("universe", "instruments", len(universe), "bar", o.bar, "days", o.days, "resume", o.resume, "parallel", o.parallel)

	start := time.Now()
	cutoff := start.Add(-time.Duration(o.days) * 24 * time.Hour)
	var (
		totalBars atomic.Int64
		totalReq  atomic.Int64
		nErr      atomic.Int64
		done      atomic.Int64
		wg        sync.WaitGroup
		queue     = make(chan string)
	)
	worker := func() {
		defer wg.Done()
		for id := range queue {
			bars, reqs, oldest, err := backfillOne(ctx, cli, st, log, id, o.bar, cutoff, o.resume)
			totalBars.Add(int64(bars))
			totalReq.Add(int64(reqs))
			n := done.Add(1)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				nErr.Add(1)
				log.Warn("instrument failed", "inst", id, "bars", bars, "err", err, "done", n, "of", len(universe))
				continue
			}
			attrs := []any{"inst", id, "bars", bars, "requests", reqs, "done", n, "of", len(universe)}
			if !oldest.IsZero() {
				attrs = append(attrs, "oldest", oldest.UTC().Format(time.RFC3339))
			}
			log.Info("instrument done", attrs...)
		}
	}
	wg.Add(o.parallel)
	for i := 0; i < o.parallel; i++ {
		go worker()
	}
feed:
	for _, id := range universe {
		select {
		case <-ctx.Done():
			break feed
		case queue <- id:
		}
	}
	close(queue)
	wg.Wait()

	stats := st.Stats()
	log.Info("summary",
		"instruments", done.Load(), "of", len(universe), "bars", totalBars.Load(), "requests", totalReq.Load(),
		"errors", nErr.Load(), "duration", time.Since(start).Round(time.Second).String(),
		"storeQueued", stats.Queued, "storeDropped", stats.Dropped, "storeErrors", stats.Errors)
	if stats.Dropped > 0 {
		log.Warn("the store dropped rows: rerun with -resume to fill gaps", "dropped", stats.Dropped)
	}
	return ctx.Err()
}

// rankUniverse lists live linear swaps settled in the configured currencies,
// ranked by 24 h quote volume (volCcy24h × last), keeps the top N and makes
// sure the benchmark is in.
func rankUniverse(ctx context.Context, cli *okx.Client, o options) ([]string, error) {
	insts, err := cli.Instruments(ctx, "SWAP")
	if err != nil {
		return nil, fmt.Errorf("instruments: %w", err)
	}
	settle := map[string]bool{}
	for _, s := range o.settle {
		settle[s] = true
	}
	live := map[string]bool{}
	for _, in := range insts {
		if in.CtType == "linear" && in.State == "live" && settle[in.SettleCcy] {
			live[in.ID] = true
		}
	}
	if len(live) == 0 {
		return nil, errors.New("no live linear swaps matched the settle filter")
	}
	tks, err := cli.Tickers(ctx, "SWAP")
	if err != nil {
		return nil, fmt.Errorf("tickers: %w", err)
	}
	type kv struct {
		id  string
		vol float64
	}
	var ranked []kv
	seen := map[string]bool{}
	for _, t := range tks {
		if live[t.InstID] {
			ranked = append(ranked, kv{t.InstID, t.VolCcy24h * t.Last})
			seen[t.InstID] = true
		}
	}
	for id := range live { // instruments without a ticker go last
		if !seen[id] {
			ranked = append(ranked, kv{id, 0})
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].vol != ranked[j].vol {
			return ranked[i].vol > ranked[j].vol
		}
		return ranked[i].id < ranked[j].id
	})
	if o.top > 0 && len(ranked) > o.top {
		ranked = ranked[:o.top]
	}
	out := make([]string, 0, len(ranked)+1)
	hasBench := false
	for _, r := range ranked {
		out = append(out, r.id)
		if r.id == o.benchmark {
			hasBench = true
		}
	}
	if !hasBench && o.benchmark != "" {
		// Fetch the benchmark first: the backtest cannot run without it.
		out = append([]string{o.benchmark}, out...)
	}
	return out, nil
}

// backfillOne pages one instrument backwards. It returns bars written,
// requests made and the oldest bar seen.
func backfillOne(ctx context.Context, cli *okx.Client, st *store.Store, log *slog.Logger, id, bar string, cutoff time.Time, resume bool) (bars, reqs int, oldest time.Time, err error) {
	after := int64(0)
	if resume {
		ts, ok, err := st.OldestCandle(ctx, id, bar)
		if err != nil {
			return 0, 0, oldest, fmt.Errorf("oldest stored: %w", err)
		}
		if ok {
			after = ts.UnixMilli()
			oldest = ts
			if !ts.After(cutoff) {
				log.Debug("already covered", "inst", id, "oldest", ts.UTC().Format(time.RFC3339))
				return 0, 0, oldest, nil
			}
		}
	}
	cutoffMs := cutoff.UnixMilli()
	for {
		if err := ctx.Err(); err != nil {
			return bars, reqs, oldest, err
		}
		page, err := fetchPage(ctx, cli, id, bar, after)
		reqs++
		if err != nil {
			return bars, reqs, oldest, err
		}
		if len(page) == 0 {
			return bars, reqs, oldest, nil // venue has nothing older: listing date reached
		}
		st.OnCandles(id, bar, page)
		bars += len(page)
		last := page[len(page)-1].TS // newest first → last element is the oldest
		oldest = time.UnixMilli(last)
		if last <= cutoffMs || (after > 0 && last >= after) {
			return bars, reqs, oldest, nil
		}
		after = last
		log.Debug("page", "inst", id, "bars", bars, "oldest", oldest.UTC().Format(time.RFC3339))
	}
}

// fetchPage retries transient errors a few times with a growing pause.
func fetchPage(ctx context.Context, cli *okx.Client, id, bar string, after int64) ([]okx.Candle, error) {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		page, err := cli.HistoryCandles(ctx, id, bar, after, 100)
		if err == nil {
			return page, nil
		}
		lastErr = err
		var api *okx.APIError
		if errors.As(err, &api) && !errors.Is(err, okx.ErrRateLimited) {
			return nil, err // a venue-side rejection will not heal by waiting
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 2 * time.Second):
		}
	}
	return nil, lastErr
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
