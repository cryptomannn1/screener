package backtest

import (
	"math"
	"testing"
	"time"
)

const hourMs = int64(3600 * 1000)

// hourly builds a series from a start slot with the given closes; a NaN close
// becomes a missing bar (no row at all).
func hourly(id string, start int64, closes ...float64) Series {
	s := Series{InstID: id}
	for i, c := range closes {
		if math.IsNaN(c) {
			continue
		}
		s.TS = append(s.TS, start+int64(i)*hourMs)
		s.Close = append(s.Close, c)
	}
	return s
}

func TestAlign(t *testing.T) {
	t0 := int64(1_700_000_000_000) - 1_700_000_000_000%hourMs // on the hour
	bench := hourly("BTC", t0, 100, 101, 102, 103, 104)
	a := hourly("A", t0, 1, 2, NaN, 4, 5)                                 // gap in the middle
	b := hourly("B", t0-2*hourMs, 7, 8, 9, 10)                            // starts before the benchmark, ends early
	c := hourly("C", t0+10*hourMs, 1, 2, 3)                               // entirely after the benchmark → dropped
	d := Series{InstID: "D", TS: []int64{t0 + 1234}, Close: []float64{5}} // off-grid → dropped

	cutoff := time.UnixMilli(t0 + 5*hourMs) // last bar (opens t0+4h) closes exactly at cutoff → kept
	p, err := Align(bench, []Series{c, a, d, b}, time.Hour, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if p.Slots() != 5 {
		t.Fatalf("slots = %d, want 5", p.Slots())
	}
	if len(p.Inst) != 2 || p.Inst[0] != "A" || p.Inst[1] != "B" {
		t.Fatalf("instruments = %v, want [A B]", p.Inst)
	}
	wantA := []float64{1, 2, NaN, 4, 5}
	for i, v := range wantA {
		got := p.Close[0][i]
		if math.IsNaN(v) != math.IsNaN(got) || (!math.IsNaN(v) && got != v) {
			t.Errorf("A[%d] = %v, want %v", i, got, v)
		}
	}
	wantB := []float64{9, 10, NaN, NaN, NaN}
	for i, v := range wantB {
		got := p.Close[1][i]
		if math.IsNaN(v) != math.IsNaN(got) || (!math.IsNaN(v) && got != v) {
			t.Errorf("B[%d] = %v, want %v", i, got, v)
		}
	}
	// Returns: A has NaN at slot 2 (missing) and slot 3 (previous missing).
	if !math.IsNaN(p.instRet[0][2]) || !math.IsNaN(p.instRet[0][3]) || math.IsNaN(p.instRet[0][4]) {
		t.Errorf("A returns = %v", p.instRet[0])
	}
	if p.Bars(0) != 4 || p.Bars(1) != 2 {
		t.Errorf("bars A=%d B=%d, want 4, 2", p.Bars(0), p.Bars(1))
	}
	if q := p.Filter(3); len(q.Inst) != 1 || q.Inst[0] != "A" {
		t.Errorf("Filter(3) = %v, want [A]", q.Inst)
	}

	// A cutoff one hour earlier drops the forming last bar.
	p2, err := Align(bench, []Series{a}, time.Hour, time.UnixMilli(t0+4*hourMs+1800*1000))
	if err != nil {
		t.Fatal(err)
	}
	if p2.Slots() != 4 {
		t.Errorf("slots with forming bar cut = %d, want 4", p2.Slots())
	}

	if _, err := Align(Series{}, nil, time.Hour, time.Time{}); err != ErrNoBenchmark {
		t.Errorf("empty benchmark: err = %v", err)
	}
}

// fromLogReturns builds closes from a start price and per-hour log returns
// (rs[0] is the return into slot 1).
func fromLogReturns(start float64, rs []float64) []float64 {
	c := make([]float64, len(rs)+1)
	c[0] = start
	for i, r := range rs {
		c[i+1] = c[i] * math.Exp(r)
	}
	return c
}

// altBench returns n log returns alternating +1 % / −1 % (variance > 0).
func altBench(n int) []float64 {
	b := make([]float64, n)
	for i := range b {
		if i%2 == 0 {
			b[i] = 0.01
		} else {
			b[i] = -0.01
		}
	}
	return b
}

func TestExcessAndSignals(t *testing.T) {
	n := 30
	b := altBench(n - 1)
	r := make([]float64, len(b))
	for i := range b {
		r[i] = 2*b[i] + 0.02 // beta 2, alpha 0.02, always above BTC
	}
	bc, ac := fromLogReturns(100, b), fromLogReturns(50, r)
	p, err := Align(hourly("BTC", 0, bc...), []Series{hourly("A", 0, ac...)}, time.Hour, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	// From slot 5 to 9: bench 0.01−0.01+0.01−0.01 = 0 (b[5..8] = −,+,−,+ → 0); A = 2×0 + 4×0.02.
	ex, ok := p.excess(0, 5, 4)
	if !ok || !near(ex, 0.08, 1e-9) {
		t.Errorf("excess = %v, %v; want 0.08", ex, ok)
	}
	if _, ok := p.excess(0, 27, 4); ok {
		t.Error("excess past the end should not be ok")
	}
	s, ok := p.signalsAt(0, 29, 24)
	if !ok {
		t.Fatal("signals not ok")
	}
	if !near(s[SigBeta], 2, 1e-9) || !near(s[SigAlpha], 0.02, 1e-9) || !near(s[SigCorr], 1, 1e-9) {
		t.Errorf("beta/alpha/corr = %v %v %v", s[SigBeta], s[SigAlpha], s[SigCorr])
	}
	if s[SigPresence] != 1 || s[SigStay] != 1 {
		t.Errorf("presence/stay = %v %v", s[SigPresence], s[SigStay])
	}
	// 24 returns ending at slot 29 (r[5..28]) sum to 2×0 + 24×0.02 for A, 0 for BTC.
	if !near(s[SigMom24], math.Exp(0.48)-1, 1e-9) {
		t.Errorf("mom24 = %v", s[SigMom24])
	}
	if !near(s[SigVsBtc24h], math.Exp(0.48)-1, 1e-9) {
		t.Errorf("vsBtc24h = %v", s[SigVsBtc24h])
	}
	if !near(s[SigVsBtc4h], math.Exp(0.08)-1, 1e-9) {
		t.Errorf("vsBtc4h = %v", s[SigVsBtc4h])
	}
	if _, ok := p.signalsAt(0, 23, 24); ok {
		t.Error("window needs t-n ≥ 0")
	}
}
