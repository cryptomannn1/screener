package collector

import (
	"math"
	"testing"
)

func TestFlowWindow(t *testing.T) {
	f := NewFlow()
	base := int64(1_700_000_000_000) // ms
	// Minute 0: 100 buy, 50 sell. Minute 1: 20 sell. Minute 61: 10 buy.
	f.Add("X", base, true, 100, 1)
	f.Add("X", base+1000, false, 50, 2)
	f.Add("X", base+60_000, false, 20, 1)
	f.Add("X", base+61*60_000, true, 10, 1)

	now := base + 61*60_000 + 5000
	w1h := f.Window("X", now, 60) // minutes 2..61 → only minute 61
	if w1h.BuyQuote != 10 || w1h.SellQuote != 0 || w1h.Trades != 1 {
		t.Fatalf("1h window %+v", w1h)
	}
	w2h := f.Window("X", now, 120)
	if w2h.BuyQuote != 110 || w2h.SellQuote != 70 || w2h.Trades != 5 {
		t.Fatalf("2h window %+v", w2h)
	}
	if got := w2h.TakerShare(); math.Abs(got-110.0/180) > 1e-12 {
		t.Fatalf("taker share %v", got)
	}
	if got := w2h.Delta(); math.Abs(got-40.0/180) > 1e-12 {
		t.Fatalf("delta %v", got)
	}
	if !math.IsNaN(f.Window("Y", now, 60).Delta()) {
		t.Fatal("unknown instrument must be NaN")
	}
	closed := f.Closed(base / 60_000)
	if len(closed) != 1 || closed[0].BuyQuote != 100 || closed[0].Trades != 3 || closed[0].BuyTrades != 1 {
		t.Fatalf("closed %+v", closed)
	}
}
