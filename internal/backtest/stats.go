package backtest

import (
	"math"
	"sort"
)

// Ranks returns average ranks (1-based, ties share the mean rank) of xs.
func Ranks(xs []float64) []float64 {
	n := len(xs)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return xs[idx[a]] < xs[idx[b]] })
	r := make([]float64, n)
	for i := 0; i < n; {
		j := i
		for j+1 < n && xs[idx[j+1]] == xs[idx[i]] {
			j++
		}
		avg := float64(i+j)/2 + 1
		for k := i; k <= j; k++ {
			r[idx[k]] = avg
		}
		i = j + 1
	}
	return r
}

// Pearson correlation of two equal-length slices; ok is false when either
// side is constant or n < 3.
func Pearson(x, y []float64) (float64, bool) {
	n := len(x)
	if n < 3 || len(y) != n {
		return NaN, false
	}
	mx, my := mean(x), mean(y)
	var sxy, sxx, syy float64
	for i := 0; i < n; i++ {
		dx, dy := x[i]-mx, y[i]-my
		sxy += dx * dy
		sxx += dx * dx
		syy += dy * dy
	}
	if sxx == 0 || syy == 0 {
		return NaN, false
	}
	return sxy / math.Sqrt(sxx*syy), true
}

// Spearman is the rank information coefficient: Pearson correlation of the
// average ranks of x and y.
func Spearman(x, y []float64) (float64, bool) {
	if len(x) != len(y) || len(x) < 3 {
		return NaN, false
	}
	return Pearson(Ranks(x), Ranks(y))
}

// QuintileSpread sorts by signal and returns mean(target of the top fifth) −
// mean(target of the bottom fifth), equal weight. Needs at least 5 pairs; the
// basket size is floor(n/5), at least 1. Ties at a boundary are broken by
// index, which is good enough for a diagnostic.
func QuintileSpread(sig, tgt []float64) (float64, bool) {
	n := len(sig)
	if n < 5 || len(tgt) != n {
		return NaN, false
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return sig[idx[a]] < sig[idx[b]] })
	k := n / 5
	var lo, hi float64
	for i := 0; i < k; i++ {
		lo += tgt[idx[i]]
		hi += tgt[idx[n-1-i]]
	}
	return (hi - lo) / float64(k), true
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return NaN
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

// MeanStd returns the mean and the sample standard deviation (n − 1).
func MeanStd(xs []float64) (m, sd float64) {
	n := len(xs)
	if n == 0 {
		return NaN, NaN
	}
	m = mean(xs)
	if n < 2 {
		return m, NaN
	}
	for _, x := range xs {
		sd += (x - m) * (x - m)
	}
	return m, math.Sqrt(sd / float64(n-1))
}

// TStat of the mean: mean / (sd / sqrt(n)). NaN when n < 2 or sd = 0.
func TStat(xs []float64) float64 {
	m, sd := MeanStd(xs)
	if len(xs) < 2 || !(sd > 0) {
		return NaN
	}
	return m / (sd / math.Sqrt(float64(len(xs))))
}

// HitRate is the share of positive values.
func HitRate(xs []float64) float64 {
	if len(xs) == 0 {
		return NaN
	}
	k := 0
	for _, x := range xs {
		if x > 0 {
			k++
		}
	}
	return float64(k) / float64(len(xs))
}

// MaxDrawdown of a cumulative curve (log units): the largest drop from a
// running peak, as a positive number. 0 for a non-decreasing curve.
func MaxDrawdown(curve []float64) float64 {
	peak := math.Inf(-1)
	dd := 0.0
	for _, v := range curve {
		if v > peak {
			peak = v
		}
		if d := peak - v; d > dd {
			dd = d
		}
	}
	return dd
}
