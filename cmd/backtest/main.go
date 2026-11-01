// Command backtest replays the screener's strength-vs-BTC metrics over the
// hourly candles stored in ClickHouse and reports whether they predicted
// excess return over the benchmark (see internal/backtest and the plan in
// the session folder, strength-backtest-plan.md).
//
//	go run ./cmd/backtest -windows 3,7,14 -horizons 4,24,72 -out report.md
//	go run ./cmd/backtest -windows 3 -horizons 4,24                 # smoke test on ~12 days
//
// Every flag can be set as an environment variable with the same name
// upper-cased and dashes as underscores (-fee-bps → FEE_BPS).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"screener/internal/backtest"
	"screener/internal/store"
)

const defaultDSN = "clickhouse://default:screener@localhost:9000/screener"

func main() {
	fs := flag.NewFlagSet("backtest", flag.ExitOnError)
	chDSN := fs.String("clickhouse", defaultDSN, "ClickHouse DSN")
	from := fs.String("from", "", "start date YYYY-MM-DD (UTC); empty = everything stored")
	to := fs.String("to", "", "end date YYYY-MM-DD (UTC, inclusive); empty = now")
	bar := fs.String("bar", "1H", "bar id stored in candles (only hourly bars make sense for the windows)")
	benchmark := fs.String("benchmark", "BTC-USDT-SWAP", "benchmark instrument")
	windows := fs.String("windows", "3,7,14", "signal windows, days")
	horizons := fs.String("horizons", "4,24,72", "forward horizons, hours")
	minBars := fs.Int("min-bars", 0, "skip instruments with fewer bars; 0 = smallest window × 24 + largest horizon + 1")
	minCross := fs.Int("min-cross", 10, "minimum instruments in a cross-section")
	feeBps := fs.Float64("fee-bps", 5, "fee per side for the preset portfolio, bps")
	presetWindow := fs.Int("preset-window", 7, "signal window of the preset portfolio, days")
	out := fs.String("out", "", "write the Markdown report here (it is printed anyway)")
	jsonOut := fs.String("json", "", "write the machine-readable result here")
	logLevel := fs.String("log", "info", "debug | info | warn | error")
	applyEnv(fs)
	_ = fs.Parse(os.Args[1:])

	log := newLogger(*logLevel)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ws, err := parseInts(*windows)
	if err != nil {
		fatal(log, "windows", err)
	}
	hs, err := parseInts(*horizons)
	if err != nil {
		fatal(log, "horizons", err)
	}
	var fromT, toT time.Time
	if *from != "" {
		if fromT, err = time.Parse("2006-01-02", *from); err != nil {
			fatal(log, "from", err)
		}
	}
	if *to != "" {
		if toT, err = time.Parse("2006-01-02", *to); err != nil {
			fatal(log, "to", err)
		}
		toT = toT.Add(24*time.Hour - time.Second)
	}
	step, err := barDuration(*bar)
	if err != nil {
		fatal(log, "bar", err)
	}
	if *minBars <= 0 {
		*minBars = minInt(ws)*24 + maxInt(hs) + 1
	}

	st, err := store.Open(ctx, store.Config{DSN: *chDSN}, log)
	if err != nil {
		fatal(log, "store", err)
	}
	defer st.Close()

	t0 := time.Now()
	hist, err := st.LoadCandles(ctx, *bar, fromT, toT)
	if err != nil {
		fatal(log, "load candles", err)
	}
	rows := 0
	for _, bs := range hist {
		rows += len(bs)
	}
	log.Info("loaded", "instruments", len(hist), "bars", rows, "took", time.Since(t0).Round(time.Millisecond).String())

	bench, ok := hist[*benchmark]
	if !ok {
		fatal(log, "benchmark", fmt.Errorf("no %s bars stored", *benchmark))
	}
	var insts []backtest.Series
	dropped := 0
	for id, bs := range hist {
		if id == *benchmark {
			continue
		}
		s := toSeries(id, bs)
		dropped += len(bs) - len(s.TS)
		insts = append(insts, s)
	}
	bseries := toSeries(*benchmark, bench)
	dropped += len(bench) - len(bseries.TS)

	cutoff := time.Now()
	if !toT.IsZero() && toT.Before(cutoff) {
		cutoff = toT
	}
	panel, err := backtest.Align(bseries, insts, step, cutoff)
	if err != nil {
		fatal(log, "align", err)
	}
	before := len(panel.Inst)
	panel = panel.Filter(*minBars)
	log.Info("aligned", "slots", panel.Slots(), "from", panel.Time(0).Format(time.RFC3339), "to", panel.Time(panel.Slots()-1).Format(time.RFC3339),
		"instruments", len(panel.Inst), "skippedMinBars", before-len(panel.Inst), "unconfirmedDropped", dropped, "minBars", *minBars)

	t1 := time.Now()
	res := backtest.Run(panel, backtest.Config{
		Windows: ws, Horizons: hs, MinCross: *minCross, FeeBps: *feeBps, PresetWindow: *presetWindow, Benchmark: *benchmark,
	})
	log.Info("computed", "cells", len(res.Cells), "presets", len(res.Presets), "took", time.Since(t1).Round(time.Millisecond).String())

	md := backtest.Markdown(res)
	fmt.Print(md)
	if *out != "" {
		if err := os.WriteFile(*out, []byte(md), 0o644); err != nil {
			fatal(log, "write report", err)
		}
		log.Info("report written", "path", *out)
	}
	if *jsonOut != "" {
		b, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			fatal(log, "json", err)
		}
		if err := os.WriteFile(*jsonOut, b, 0o644); err != nil {
			fatal(log, "write json", err)
		}
		log.Info("json written", "path", *jsonOut)
	}
}

// toSeries keeps confirmed bars only: a bar last fetched while forming has a
// provisional close and must not enter the test.
func toSeries(id string, bs []store.HistoryBar) backtest.Series {
	s := backtest.Series{InstID: id}
	for _, b := range bs {
		if !b.Confirm || b.C <= 0 {
			continue
		}
		s.TS = append(s.TS, b.TS)
		s.Close = append(s.Close, b.C)
	}
	return s
}

func barDuration(bar string) (time.Duration, error) {
	switch strings.ToUpper(bar) {
	case "1H":
		return time.Hour, nil
	case "2H":
		return 2 * time.Hour, nil
	case "4H":
		return 4 * time.Hour, nil
	case "1D", "1DUTC":
		return 24 * time.Hour, nil
	}
	return 0, fmt.Errorf("unsupported bar %q (windows and horizons assume hourly bars)", bar)
}

func parseInts(s string) ([]int, error) {
	var out []int
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.Atoi(p)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("bad value %q", p)
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, errors.New("empty list")
	}
	return out, nil
}

func minInt(xs []int) int {
	m := xs[0]
	for _, x := range xs {
		if x < m {
			m = x
		}
	}
	return m
}

func maxInt(xs []int) int {
	m := xs[0]
	for _, x := range xs {
		if x > m {
			m = x
		}
	}
	return m
}

func fatal(log *slog.Logger, what string, err error) {
	log.Error(what, "err", err)
	os.Exit(1)
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
