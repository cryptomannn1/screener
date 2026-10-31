// Package store persists the collector's observations in ClickHouse. Writes
// are queued and inserted in batches per table; a full queue drops rows and
// counts them rather than slowing the collector down.
package store

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"screener/internal/collector"
	"screener/internal/metrics"
	"screener/internal/okx"
)

//go:embed schema.sql
var schemaSQL string

// Config of the store.
type Config struct {
	DSN     string
	TTLDays int           // retention of ticks / depth / rows, default 180
	Batch   int           // rows per insert, default 2000
	Flush   time.Duration // flush a partial batch after, default 2s
	Queue   int           // queued rows before dropping, default 200000
}

func (c Config) withDefaults() Config {
	if c.TTLDays <= 0 {
		c.TTLDays = 180
	}
	if c.Batch <= 0 {
		c.Batch = 2000
	}
	if c.Flush <= 0 {
		c.Flush = 2 * time.Second
	}
	if c.Queue <= 0 {
		c.Queue = 200_000
	}
	return c
}

type item struct {
	table string
	vals  []any
}

// Store is a ClickHouse sink.
type Store struct {
	cfg  Config
	conn driver.Conn
	log  *slog.Logger
	q    chan item
	done chan struct{}
	wg   sync.WaitGroup

	written atomic.Uint64
	dropped atomic.Uint64
	errs    atomic.Uint64
	lastErr atomic.Value // string
}

// Open connects, applies the schema and starts the writer.
func Open(ctx context.Context, cfg Config, log *slog.Logger) (*Store, error) {
	cfg = cfg.withDefaults()
	opt, err := clickhouse.ParseDSN(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("store: dsn: %w", err)
	}
	opt.Compression = &clickhouse.Compression{Method: clickhouse.CompressionLZ4}
	conn, err := clickhouse.Open(opt)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := conn.Ping(pctx); err != nil {
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	s := &Store{cfg: cfg, conn: conn, log: log, q: make(chan item, cfg.Queue), done: make(chan struct{})}
	if err := s.ensureSchema(pctx); err != nil {
		return nil, err
	}
	s.wg.Add(1)
	go s.writer()
	return s, nil
}

func (s *Store) ensureSchema(ctx context.Context) error {
	sql := strings.ReplaceAll(schemaSQL, "{ttl}", fmt.Sprint(s.cfg.TTLDays))
	// Strip comment lines first, then split statements: a ';' inside a
	// comment must not cut a statement.
	var lines []string
	for _, l := range strings.Split(sql, "\n") {
		if t := strings.TrimSpace(l); t != "" && !strings.HasPrefix(t, "--") {
			lines = append(lines, l)
		}
	}
	for _, stmt := range strings.Split(strings.Join(lines, "\n"), ";") {
		clean := strings.TrimSpace(stmt)
		if clean == "" {
			continue
		}
		if err := s.conn.Exec(ctx, clean); err != nil {
			return fmt.Errorf("store: schema: %w", err)
		}
	}
	return nil
}

// Close flushes and closes.
func (s *Store) Close() error {
	close(s.done)
	s.wg.Wait()
	return s.conn.Close()
}

// Stats for /health.
type Stats struct {
	Written uint64 `json:"written"`
	Dropped uint64 `json:"dropped"`
	Errors  uint64 `json:"errors"`
	LastErr string `json:"lastErr,omitempty"`
	Queued  int    `json:"queued"`
}

// Stats returns the counters.
func (s *Store) Stats() Stats {
	st := Stats{Written: s.written.Load(), Dropped: s.dropped.Load(), Errors: s.errs.Load(), Queued: len(s.q)}
	if v := s.lastErr.Load(); v != nil {
		st.LastErr = v.(string)
	}
	return st
}

func (s *Store) push(table string, vals ...any) {
	select {
	case s.q <- item{table, vals}:
	default:
		s.dropped.Add(1)
	}
}

func (s *Store) writer() {
	defer s.wg.Done()
	pending := map[string][][]any{}
	count := 0
	t := time.NewTicker(s.cfg.Flush)
	defer t.Stop()
	flush := func() {
		for table, rows := range pending {
			if err := s.insert(table, rows); err != nil {
				s.errs.Add(1)
				s.lastErr.Store(err.Error())
				s.log.Warn("store insert", "table", table, "rows", len(rows), "err", err)
			} else {
				s.written.Add(uint64(len(rows)))
			}
			delete(pending, table)
		}
		count = 0
	}
	for {
		select {
		case it := <-s.q:
			pending[it.table] = append(pending[it.table], it.vals)
			count++
			if count >= s.cfg.Batch {
				flush()
			}
		case <-t.C:
			if count > 0 {
				flush()
			}
		case <-s.done:
			for {
				select {
				case it := <-s.q:
					pending[it.table] = append(pending[it.table], it.vals)
				default:
					flush()
					return
				}
			}
		}
	}
}

func (s *Store) insert(table string, rows [][]any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	b, err := s.conn.PrepareBatch(ctx, "INSERT INTO "+table)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if err := b.Append(r...); err != nil {
			return err
		}
	}
	return b.Send()
}

// clean turns NaN / Inf into 0 for storage (ClickHouse Float64 accepts NaN,
// but 0 keeps aggregates simple; rows store coverage via other means).
func clean(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

func cf(v collector.F) float64 { return clean(float64(v)) }

// OnTick implements collector.Sink.
func (s *Store) OnTick(ts time.Time, rows []collector.Tick) {
	for _, r := range rows {
		s.push("ticks", ts, r.InstID, r.Last, r.Mark, r.Index, r.Bid, r.Ask,
			r.VolCcy24h, r.VolQuote24h, r.OICcy, r.OIUsd, r.Funding, clean(r.NextFunding))
	}
}

// OnCandles implements collector.Sink.
func (s *Store) OnCandles(instID, bar string, cs []okx.Candle) {
	now := time.Now()
	for _, c := range cs {
		conf := uint8(0)
		if c.Confirm {
			conf = 1
		}
		s.push("candles", instID, bar, time.UnixMilli(c.TS), c.Open, c.High, c.Low, c.Close, c.VolCcy, c.VolCcyQuote, conf, now)
	}
}

// OnDepth implements collector.Sink.
func (s *Store) OnDepth(ts time.Time, instID string, d metrics.Depth) {
	s.push("depth", ts, instID, clean(d.Mid), clean(d.SpreadBps),
		clean(d.Bid01), clean(d.Ask01), clean(d.Bid05), clean(d.Ask05), clean(d.Bid1), clean(d.Ask1))
}

// OnRows implements collector.Sink.
func (s *Store) OnRows(ts time.Time, rows []collector.Row) {
	for _, r := range rows {
		s.push("rows", ts, r.InstID, cf(r.Last), cf(r.Mark), cf(r.Basis),
			cf(r.Chg["1h"]), cf(r.Chg["4h"]), cf(r.Chg["24h"]), cf(r.Chg1d), cf(r.Chg7d), cf(r.Chg30d),
			cf(r.Vol24), cf(r.VolX7), cf(r.VolX["1h"]), cf(r.VolX["4h"]), cf(r.VolX["24h"]),
			cf(r.OI), cf(r.OID["1h"]), cf(r.OID["4h"]), cf(r.OID["24h"]), cf(r.OIVol),
			cf(r.Funding), cf(r.FundingAnn),
			cf(r.ATR), cf(r.RV24), cf(r.Regime), cf(r.Range1d), cf(r.ToHigh), cf(r.ToLow),
			cf(r.VsBtc["1h"]), cf(r.VsBtc["4h"]), cf(r.VsBtc["24h"]),
			cf(r.Beta), cf(r.Alpha), cf(r.Presence), cf(r.Stay), cf(r.Corr),
			cf(r.Depth01), cf(r.Depth05), cf(r.Depth1), cf(r.Imb), cf(r.Spread))
	}
}

// OIPoint is one stored OI observation.
type OIPoint struct {
	InstID string
	TS     int64 // ms
	USD    float64
}

// LoadOI returns OI history newer than since, oldest first per instrument.
func (s *Store) LoadOI(ctx context.Context, since time.Time) ([]OIPoint, error) {
	rows, err := s.conn.Query(ctx,
		"SELECT inst_id, ts, oi_usd FROM ticks FINAL WHERE ts >= ? AND oi_usd > 0 ORDER BY inst_id, ts", since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OIPoint
	for rows.Next() {
		var id string
		var ts time.Time
		var usd float64
		if err := rows.Scan(&id, &ts, &usd); err != nil {
			return nil, err
		}
		out = append(out, OIPoint{InstID: id, TS: ts.UnixMilli(), USD: usd})
	}
	return out, rows.Err()
}

// ErrNoStore is returned by helpers when the store is nil.
var ErrNoStore = errors.New("store: not configured")
