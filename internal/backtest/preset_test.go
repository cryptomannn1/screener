package backtest

import (
	"math"
	"testing"
	"time"
)

// flatPanel builds a panel of slots hours with a flat benchmark and the
// given instrument closes (one slice per instrument, all of length slots).
func flatPanel(t *testing.T, slots int, bench []float64, insts map[string][]float64) *Panel {
	t.Helper()
	var ss []Series
	for id, c := range insts {
		ss = append(ss, hourly(id, 0, c...))
	}
	p, err := Align(hourly("BTC", 0, bench...), ss, time.Hour, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Slots() != slots {
		t.Fatalf("slots = %d, want %d", p.Slots(), slots)
	}
	return p
}

func TestRunPresetSelectionAndFees(t *testing.T) {
	// Window 1 day (24 hourly returns), horizon 2 h, three rebalance periods:
	// t = 24, 26, 28 need closes up to slot 30 → 31 slots.
	const slots = 31
	b := altBench(slots - 1)
	rs, rl := make([]float64, len(b)), make([]float64, len(b))
	for i := range b {
		rs[i] = 2*b[i] + 0.02   // STRONG: beta 2, beats BTC every hour
		rl[i] = 0.5*b[i] + 0.02 // LOWBETA: beats BTC every hour but beta 0.5
	}
	bench, strong := fromLogReturns(100, b), fromLogReturns(100, rs)
	low := fromLogReturns(100, rl)
	p := flatPanel(t, slots, bench, map[string][]float64{"STRONG": strong, "LOWBETA": low})

	// Sanity on the selection itself at t = 24.
	for i, id := range p.Inst {
		s, ok := p.signalsAt(i, 24, 24)
		if !ok {
			t.Fatalf("%s: signals not ok", id)
		}
		match := DefaultPresets[0].Match(s)
		if (id == "STRONG") != match {
			t.Errorf("%s: strong match = %v (beta %.2f presence %.2f vsBtc24h %.4f)", id, match, s[SigBeta], s[SigPresence], s[SigVsBtc24h])
		}
	}

	// No fees: three periods, basket of exactly STRONG. Over 2 h the bench
	// returns to its level (+1 % −1 %) and STRONG gains 2×0 + 2×0.02 in log terms.
	r := RunPreset(p, DefaultPresets[0], 1, 2, 0, slots/2)
	if r.Periods != 3 || r.NonEmpty != 3 || r.AvgSize != 1 {
		t.Fatalf("periods=%d nonEmpty=%d avgSize=%v", r.Periods, r.NonEmpty, r.AvgSize)
	}
	wantGross := 3 * 0.04
	if !near(r.Gross, wantGross, 1e-9) || r.Fees != 0 || !near(r.Net, wantGross, 1e-9) {
		t.Errorf("gross=%v fees=%v net=%v, want gross %v", r.Gross, r.Fees, r.Net, wantGross)
	}
	if r.HitRate != 1 || r.MaxDD != 0 {
		t.Errorf("hitRate=%v maxDD=%v", r.HitRate, r.MaxDD)
	}
	// Halves: mid = 15 < 24, so everything accrues in the second half.
	if r.Half1 != 0 || !near(r.Half2, r.Net, 1e-12) {
		t.Errorf("halves = %v / %v", r.Half1, r.Half2)
	}

	// 10 bps per side: first period buys the whole book (turnover 1), the
	// next two keep the same single name (turnover 0) → fees = 1 × 10 bps.
	r = RunPreset(p, DefaultPresets[0], 1, 2, 10, slots/2)
	if !near(r.Fees, 0.001, 1e-12) || !near(r.Net, wantGross-0.001, 1e-9) {
		t.Errorf("fees=%v net=%v", r.Fees, r.Net)
	}

	// The short preset never matches here (nothing lags) → empty periods, zero excess.
	w := RunPreset(p, DefaultPresets[1], 1, 2, 10, slots/2)
	if w.Periods != 3 || w.NonEmpty != 0 || w.Net != 0 || !math.IsNaN(w.HitRate) {
		t.Errorf("weak: %+v", w)
	}
}

func TestRunPresetTurnoverOnChange(t *testing.T) {
	// Two periods; the basket switches from one name to another between them.
	// Turnover: period 1 = 1 (buy A), period 2 = |0 − 1| + |1 − 0| = 2.
	// Build synthetic signals via a custom preset that selects by slot parity
	// so the fee arithmetic is tested in isolation from the signal maths.
	const slots = 29 // t = 24, 26 ; closes up to 28
	bench := make([]float64, slots)
	a := make([]float64, slots)
	b := make([]float64, slots)
	for i := range bench {
		bench[i] = 100
		a[i] = 100 + float64(i%2) // keep returns defined, variance > 0
		b[i] = 200 + float64(i%2)
	}
	p := flatPanel(t, slots, bench, map[string][]float64{"A": a, "B": b})
	call := 0
	pr := Preset{Name: "alt", Match: func(Signals) bool {
		// Called once per instrument per period, instruments in sorted order
		// (A, B): period 1 picks A (calls 0,1), period 2 picks B (calls 2,3).
		call++
		switch call {
		case 1, 4:
			return true
		}
		return false
	}}
	r := RunPreset(p, pr, 1, 2, 5, slots/2)
	if r.Periods != 2 || r.NonEmpty != 2 {
		t.Fatalf("periods=%d nonEmpty=%d", r.Periods, r.NonEmpty)
	}
	if !near(r.Fees, 3*0.0005, 1e-12) {
		t.Errorf("fees = %v, want 3 × 5 bps", r.Fees)
	}
	// Excess: A and B both return 0 over 2 h (same parity) and so does BTC.
	if !near(r.Gross, 0, 1e-12) {
		t.Errorf("gross = %v, want 0", r.Gross)
	}
	if !near(r.MaxDD, 3*0.0005, 1e-12) {
		t.Errorf("maxDD = %v, want fees", r.MaxDD)
	}
}

func TestRunProducesCells(t *testing.T) {
	// 12 instruments with distinct drifts: a signal that ranks drift should get
	// IC = 1 for mom24 on every hour (returns are deterministic).
	const slots = 24*2 + 10
	bench := make([]float64, slots)
	for i := range bench {
		if i%2 == 0 {
			bench[i] = 100
		} else {
			bench[i] = 101
		}
	}
	insts := map[string][]float64{}
	for k := 0; k < 12; k++ {
		c := make([]float64, slots)
		for i := range c {
			base := 100.0
			if i%2 == 1 {
				base = 101
			}
			c[i] = base * math.Exp(0.001*float64(k)*float64(i))
		}
		insts[string(rune('A'+k))] = c
	}
	p := flatPanel(t, slots, bench, insts)
	res := Run(p, Config{Windows: []int{1}, Horizons: []int{2}, MinCross: 10, FeeBps: 5, PresetWindow: 1})
	if res.Universe != 12 || len(res.Cells) != NumSignals {
		t.Fatalf("universe=%d cells=%d", res.Universe, len(res.Cells))
	}
	var mom Cell
	for _, c := range res.Cells {
		if c.Signal == "mom24" {
			mom = c
		}
	}
	// Hours t = 24 … slots-3 → slots-26 observations.
	if mom.N != slots-26 || !near(mom.MeanIC, 1, 1e-12) || mom.HitRate != 1 {
		t.Errorf("mom24 cell = %+v", mom)
	}
	if mom.NNonOverlap != (mom.N+1)/2 {
		t.Errorf("non-overlap n = %d, want %d", mom.NNonOverlap, (mom.N+1)/2)
	}
	if !(mom.Spread > 0) {
		t.Errorf("spread = %v, want > 0", mom.Spread)
	}
	if len(res.Presets) != 2*len(res.Config.Horizons) {
		t.Errorf("presets = %d", len(res.Presets))
	}
	md := Markdown(res)
	if len(md) == 0 || !containsAll(md, "## Horizon 2h", "mom24 (control)", "## What this says") {
		t.Errorf("markdown missing sections:\n%s", md)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, x := range subs {
		if !contains(s, x) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
