package backtest

import (
	"math"

	"screener/internal/metrics"
)

// Signal indices. The first seven are the screener's strength block, mom24 is
// the control: the instrument's own 24 h return, no benchmark involved.
const (
	SigVsBtc4h = iota
	SigVsBtc24h
	SigBeta
	SigAlpha
	SigPresence
	SigStay
	SigCorr
	SigMom24
	NumSignals
)

// SignalNames in index order.
var SignalNames = [NumSignals]string{"vsBtc4h", "vsBtc24h", "beta", "alpha", "presence", "stay", "corr", "mom24"}

// IsControl reports whether a signal is the momentum control.
func IsControl(s int) bool { return s == SigMom24 }

// Signals is one instrument's signal vector at one hour.
type Signals [NumSignals]float64

// signalsAt computes the signals of instrument i at slot t from the n hourly
// log returns ending at t (closes c[t-n] … c[t]). ok is false when any return
// in the window is missing on either side, so every signal of the vector is
// computed on exactly the same n paired hours. n must be ≥ 24 (the smallest
// window metrics.StrengthVs accepts) and ≥ 25 to leave room for the 24 h
// change; the caller guarantees n = days × 24 with days ≥ 1... in practice
// windows of 3 days and more.
func (p *Panel) signalsAt(i, t, n int) (Signals, bool) {
	var s Signals
	if n < 24 || t-n < 0 || t >= len(p.TS) {
		return s, false
	}
	r, b := p.instRet[i][t-n+1:t+1], p.benchRet[t-n+1:t+1]
	for k := range r {
		if !finite(r[k]) || !finite(b[k]) {
			return s, false
		}
	}
	st := metrics.StrengthVs(r, b)
	c, bc := p.Close[i], p.Bench
	s[SigVsBtc4h] = (c[t]/c[t-4] - 1) - (bc[t]/bc[t-4] - 1)
	s[SigVsBtc24h] = (c[t]/c[t-24] - 1) - (bc[t]/bc[t-24] - 1)
	s[SigBeta] = st.Beta
	s[SigAlpha] = st.Alpha
	s[SigPresence] = st.Presence
	s[SigStay] = st.Stay
	s[SigCorr] = st.Corr
	s[SigMom24] = c[t]/c[t-24] - 1
	return s, true
}

// excess is the log return of instrument i over the benchmark from slot t to
// t+h; ok is false when a close is missing.
func (p *Panel) excess(i, t, h int) (float64, bool) {
	if t+h >= len(p.TS) {
		return NaN, false
	}
	c0, c1 := p.Close[i][t], p.Close[i][t+h]
	b0, b1 := p.Bench[t], p.Bench[t+h]
	if !(c0 > 0 && c1 > 0 && b0 > 0 && b1 > 0) {
		return NaN, false
	}
	return math.Log(c1/c0) - math.Log(b1/b0), true
}

// ownReturn is the simple return of instrument i from t to t+h.
func (p *Panel) ownReturn(i, t, h int) (float64, bool) {
	if t+h >= len(p.TS) {
		return NaN, false
	}
	c0, c1 := p.Close[i][t], p.Close[i][t+h]
	if !(c0 > 0 && c1 > 0) {
		return NaN, false
	}
	return c1/c0 - 1, true
}

// benchReturn is the simple return of the benchmark from t to t+h.
func (p *Panel) benchReturn(t, h int) (float64, bool) {
	if t+h >= len(p.TS) {
		return NaN, false
	}
	b0, b1 := p.Bench[t], p.Bench[t+h]
	if !(b0 > 0 && b1 > 0) {
		return NaN, false
	}
	return b1/b0 - 1, true
}
