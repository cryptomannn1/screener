package metrics

import (
	"math"
	"testing"
)

func approx(t *testing.T, name string, got, want, eps float64) {
	t.Helper()
	if math.IsNaN(got) || math.Abs(got-want) > eps {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func TestChange(t *testing.T) {
	bars := []Bar{{C: 105}, {C: 100}, {C: 90}}
	approx(t, "1 bar", Change(105, bars, 1), 0.05, 1e-9)
	approx(t, "2 bars", Change(105, bars, 2), 105.0/90-1, 1e-9)
	if !math.IsNaN(Change(105, bars, 3)) {
		t.Fatal("expected NaN without history")
	}
}

func TestATRPct(t *testing.T) {
	// Constant 2-point ranges, no gaps → ATR = 2.
	bars := make([]Bar, 20)
	for i := range bars {
		bars[i] = Bar{O: 100, H: 101, L: 99, C: 100}
	}
	approx(t, "atr", ATRPct(100, bars, 14), 0.02, 1e-9)
}

func TestStrength(t *testing.T) {
	b := make([]float64, 48)
	r := make([]float64, 48)
	for i := range b {
		b[i] = math.Sin(float64(i)) * 0.01
		r[i] = 2*b[i] + 0.001 // perfect beta 2, alpha 0.001
	}
	s := StrengthVs(r, b)
	approx(t, "beta", s.Beta, 2, 1e-9)
	approx(t, "corr", s.Corr, 1, 1e-9)
	approx(t, "alpha", s.Alpha, 0.001, 1e-9)
	// r > b whenever b > −0.001.
	if s.Presence <= 0.4 || s.Presence > 1 {
		t.Fatalf("presence %v", s.Presence)
	}
}

func TestDepthBands(t *testing.T) {
	bids := []BookLevel{{Px: 99.95, Qty: 1}, {Px: 99.6, Qty: 1}, {Px: 99.2, Qty: 1}, {Px: 98, Qty: 1}}
	asks := []BookLevel{{Px: 100.05, Qty: 1}, {Px: 100.4, Qty: 1}, {Px: 100.8, Qty: 1}, {Px: 102, Qty: 1}}
	d := DepthBands(bids, asks, false, false)
	approx(t, "mid", d.Mid, 100, 1e-9)
	approx(t, "spread", d.SpreadBps, 10, 1e-6)
	approx(t, "bid01", d.Bid01, 99.95, 1e-9)
	approx(t, "bid05", d.Bid05, 99.95+99.6, 1e-9)
	approx(t, "bid1", d.Bid1, 99.95+99.6+99.2, 1e-9)
	approx(t, "ask1", d.Ask1, 100.05+100.4+100.8, 1e-9)

	// A bid side cut at 0.4 % below mid leaves the 0.5 % and 1 % bands unknown.
	short := []BookLevel{{Px: 99.95, Qty: 1}, {Px: 99.6, Qty: 1}}
	d = DepthBands(short, asks, true, false)
	approx(t, "bid01 trunc", d.Bid01, 99.95, 1e-9)
	if !math.IsNaN(d.Bid05) || !math.IsNaN(d.Bid1) || !math.IsNaN(d.Imbalance05) {
		t.Fatalf("truncated bands must be NaN: %+v", d)
	}
	approx(t, "reach", d.BidReach, 0.004, 1e-9)
}

func TestVolRegime(t *testing.T) {
	daily := make([]Bar, 25)
	for i := range daily {
		daily[i] = Bar{H: 102, L: 98, C: 100}
	}
	daily[0] = Bar{H: 104, L: 96, C: 100} // today twice the range
	approx(t, "regime", VolRegime(daily, 20), 2, 1e-9)
}

func TestFundingAPR(t *testing.T) {
	approx(t, "8h", FundingAPR(0.0001, 8*3600*1000), 0.0001*3*365, 1e-12)
}
