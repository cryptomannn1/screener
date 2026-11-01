package store

import (
	"context"
	"time"

	"screener/internal/metrics"
)

// Read side of the candles table, used by cmd/backfill (resume point) and
// cmd/backtest (history). Every query reads FROM candles FINAL so a bar that
// was re-fetched (the forming bar converging) is seen once, at its latest
// fetch.

// OldestCandle returns the open time of the oldest stored bar of one
// instrument / bar size. ok is false when nothing is stored.
func (s *Store) OldestCandle(ctx context.Context, instID, bar string) (ts time.Time, ok bool, err error) {
	row := s.conn.QueryRow(ctx,
		"SELECT min(ts), count() FROM candles WHERE inst_id = ? AND bar = ?", instID, bar)
	var n uint64
	if err := row.Scan(&ts, &n); err != nil {
		return time.Time{}, false, err
	}
	return ts, n > 0, nil
}

// CandleCounts returns the number of stored bars per instrument for one bar
// size (no FINAL: duplicates from re-fetches are counted, which is fine for
// a progress overview).
func (s *Store) CandleCounts(ctx context.Context, bar string) (map[string]uint64, error) {
	rows, err := s.conn.Query(ctx, "SELECT inst_id, count() FROM candles WHERE bar = ? GROUP BY inst_id", bar)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]uint64{}
	for rows.Next() {
		var id string
		var n uint64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// HistoryBar is one stored candle for the backtest: metrics.Bar plus the
// confirm flag, so a caller can drop bars that were still forming when they
// were last fetched.
type HistoryBar struct {
	metrics.Bar
	Confirm bool
}

// LoadCandles returns the stored bars of one bar size per instrument, OLDEST
// first, limited to [from, to] when those are non-zero. The whole range is
// read in one streaming query ordered by (inst_id, ts).
func (s *Store) LoadCandles(ctx context.Context, bar string, from, to time.Time) (map[string][]HistoryBar, error) {
	q := "SELECT inst_id, ts, open, high, low, close, vol_quote, confirm FROM candles FINAL WHERE bar = ?"
	args := []any{bar}
	if !from.IsZero() {
		q += " AND ts >= ?"
		args = append(args, from)
	}
	if !to.IsZero() {
		q += " AND ts <= ?"
		args = append(args, to)
	}
	q += " ORDER BY inst_id, ts"
	rows, err := s.conn.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]HistoryBar{}
	for rows.Next() {
		var id string
		var ts time.Time
		var o, h, l, c, vq float64
		var conf uint8
		if err := rows.Scan(&id, &ts, &o, &h, &l, &c, &vq, &conf); err != nil {
			return nil, err
		}
		out[id] = append(out[id], HistoryBar{
			Bar:     metrics.Bar{TS: ts.UnixMilli(), O: o, H: h, L: l, C: c, QuoteVol: vq},
			Confirm: conf == 1,
		})
	}
	return out, rows.Err()
}
