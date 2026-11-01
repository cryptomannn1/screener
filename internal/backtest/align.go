// Package backtest replays the screener's "strength vs BTC" metrics over
// stored hourly candles and measures whether they predicted excess return
// over the benchmark. Everything here is pure Go over slices: loading is the
// caller's job (cmd/backtest reads the store), so the arithmetic is unit
// testable.
//
// Conventions: series are OLDEST FIRST (the opposite of internal/metrics,
// which follows the OKX wire order), returns are log returns unless the name
// says otherwise, and a missing value is NaN.
package backtest

import (
	"errors"
	"math"
	"sort"
	"time"
)

// NaN marks a missing value.
var NaN = math.NaN()

// Series is one instrument's closes, oldest first.
type Series struct {
	InstID string
	TS     []int64 // bar open time, ms
	Close  []float64
}

// Panel is the aligned data set: one regular time grid shared by the
// benchmark and every instrument. A slot an instrument has no bar for is NaN,
// so every downstream computation checks finiteness instead of indices.
type Panel struct {
	Step  int64     // ms per grid slot
	TS    []int64   // grid open times, oldest first
	Bench []float64 // benchmark close per slot
	Inst  []string  // instrument ids, sorted
	Close [][]float64

	// Derived once: per-slot log returns, r[i] = ln(c[i]/c[i-1]); r[0] = NaN.
	benchRet []float64
	instRet  [][]float64
}

// ErrNoBenchmark is returned when the benchmark series is empty.
var ErrNoBenchmark = errors.New("backtest: benchmark has no bars")

// Align builds the grid from the benchmark's first to last bar in steps of
// step, drops every slot whose bar has not closed by cutoff (the forming bar)
// and maps the instruments onto it. Instruments with no bar on the grid are
// dropped. Bars outside the benchmark's range are ignored: without a
// benchmark close there is no excess return to measure.
func Align(bench Series, insts []Series, step time.Duration, cutoff time.Time) (*Panel, error) {
	if len(bench.TS) == 0 {
		return nil, ErrNoBenchmark
	}
	st := step.Milliseconds()
	if st <= 0 {
		return nil, errors.New("backtest: step must be positive")
	}
	first, last := bench.TS[0], bench.TS[len(bench.TS)-1]
	if !cutoff.IsZero() {
		// The last closed bar opens at cutoff − step or earlier.
		if lim := cutoff.UnixMilli() - st; lim < last {
			last = lim
		}
	}
	if last < first {
		return nil, ErrNoBenchmark
	}
	n := int((last-first)/st) + 1
	p := &Panel{Step: st, TS: make([]int64, n), Bench: fill(n)}
	for i := range p.TS {
		p.TS[i] = first + int64(i)*st
	}
	slot := func(ts int64) (int, bool) {
		if ts < first || ts > last || (ts-first)%st != 0 {
			return 0, false
		}
		return int((ts - first) / st), true
	}
	place := func(s Series, dst []float64) int {
		placed := 0
		for i, ts := range s.TS {
			if k, ok := slot(ts); ok && s.Close[i] > 0 && !math.IsNaN(s.Close[i]) {
				dst[k] = s.Close[i]
				placed++
			}
		}
		return placed
	}
	place(bench, p.Bench)

	sorted := append([]Series(nil), insts...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].InstID < sorted[j].InstID })
	for _, s := range sorted {
		dst := fill(n)
		if place(s, dst) == 0 {
			continue
		}
		p.Inst = append(p.Inst, s.InstID)
		p.Close = append(p.Close, dst)
	}
	p.benchRet = logReturns(p.Bench)
	p.instRet = make([][]float64, len(p.Close))
	for i, c := range p.Close {
		p.instRet[i] = logReturns(c)
	}
	return p, nil
}

// Slots is the number of grid slots.
func (p *Panel) Slots() int { return len(p.TS) }

// Bars counts the finite closes of one instrument.
func (p *Panel) Bars(i int) int {
	n := 0
	for _, c := range p.Close[i] {
		if !math.IsNaN(c) {
			n++
		}
	}
	return n
}

// Filter returns a copy keeping only instruments with at least minBars
// finite closes.
func (p *Panel) Filter(minBars int) *Panel {
	q := &Panel{Step: p.Step, TS: p.TS, Bench: p.Bench, benchRet: p.benchRet}
	for i := range p.Inst {
		if p.Bars(i) >= minBars {
			q.Inst = append(q.Inst, p.Inst[i])
			q.Close = append(q.Close, p.Close[i])
			q.instRet = append(q.instRet, p.instRet[i])
		}
	}
	return q
}

// Time of slot i.
func (p *Panel) Time(i int) time.Time { return time.UnixMilli(p.TS[i]).UTC() }

func fill(n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = NaN
	}
	return s
}

// logReturns is ln(c[i]/c[i-1]) per slot; NaN where either close is missing.
func logReturns(c []float64) []float64 {
	r := fill(len(c))
	for i := 1; i < len(c); i++ {
		if a, b := c[i-1], c[i]; a > 0 && b > 0 {
			r[i] = math.Log(b / a)
		}
	}
	return r
}

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
