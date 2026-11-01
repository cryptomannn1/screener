package store

import (
	"context"
	"time"

	"screener/internal/okx"
)

// RecentCandles returns the newest `limit` bars per instrument for one bar
// size, newest first, so a restarted service starts with its series intact
// instead of re-fetching the whole universe.
func (s *Store) RecentCandles(ctx context.Context, bar string, since time.Time, limit int) (map[string][]okx.Candle, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT inst_id, ts, open, high, low, close, vol_ccy, vol_quote, confirm
		FROM candles FINAL
		WHERE bar = ? AND ts >= ?
		ORDER BY inst_id, ts DESC`, bar, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]okx.Candle{}
	for rows.Next() {
		var id string
		var ts time.Time
		var c okx.Candle
		var conf uint8
		if err := rows.Scan(&id, &ts, &c.Open, &c.High, &c.Low, &c.Close, &c.VolCcy, &c.VolCcyQuote, &conf); err != nil {
			return nil, err
		}
		if len(out[id]) >= limit {
			continue
		}
		c.TS = ts.UnixMilli()
		c.Confirm = conf == 1
		out[id] = append(out[id], c)
	}
	return out, rows.Err()
}

// FlowRow is one stored taker minute.
type FlowRow struct {
	InstID    string
	Minute    int64 // unix minute
	BuyQuote  float64
	SellQuote float64
	Trades    uint32
	BuyTrades uint32
}

// RecentFlow returns taker minutes newer than since, oldest first.
func (s *Store) RecentFlow(ctx context.Context, since time.Time) ([]FlowRow, error) {
	rows, err := s.conn.Query(ctx, `
		SELECT inst_id, ts, buy_quote, sell_quote, trades, buy_trades
		FROM flow_1m FINAL
		WHERE ts >= ?
		ORDER BY ts`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FlowRow
	for rows.Next() {
		var r FlowRow
		var ts time.Time
		if err := rows.Scan(&r.InstID, &ts, &r.BuyQuote, &r.SellQuote, &r.Trades, &r.BuyTrades); err != nil {
			return nil, err
		}
		r.Minute = ts.Unix() / 60
		out = append(out, r)
	}
	return out, rows.Err()
}
