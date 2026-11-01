package backtest

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestRanks(t *testing.T) {
	cases := []struct {
		in, want []float64
	}{
		{[]float64{10, 30, 20}, []float64{1, 3, 2}},
		{[]float64{5, 5, 1}, []float64{2.5, 2.5, 1}},
		{[]float64{2, 2, 2}, []float64{2, 2, 2}},
		{[]float64{1}, []float64{1}},
	}
	for _, c := range cases {
		got := Ranks(c.in)
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("Ranks(%v) = %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
}

func TestSpearman(t *testing.T) {
	cases := []struct {
		name string
		x, y []float64
		want float64
		ok   bool
	}{
		{"perfect", []float64{1, 2, 3, 4, 5}, []float64{10, 20, 30, 40, 50}, 1, true},
		{"inverse", []float64{1, 2, 3, 4, 5}, []float64{5, 4, 3, 2, 1}, -1, true},
		// Known textbook example: ranks (1,2,3,4,5) vs (2,1,4,3,5): d² = 1+1+1+1+0 = 4,
		// ρ = 1 − 6·4 / (5·24) = 0.8.
		{"textbook", []float64{1, 2, 3, 4, 5}, []float64{2, 1, 4, 3, 5}, 0.8, true},
		// Monotone but non-linear: rank correlation is still 1.
		{"nonlinear", []float64{1, 2, 3, 4}, []float64{1, 10, 100, 1000}, 1, true},
		{"constant", []float64{1, 2, 3}, []float64{7, 7, 7}, 0, false},
		{"short", []float64{1, 2}, []float64{1, 2}, 0, false},
	}
	for _, c := range cases {
		got, ok := Spearman(c.x, c.y)
		if ok != c.ok {
			t.Errorf("%s: ok = %v, want %v", c.name, ok, c.ok)
			continue
		}
		if ok && !near(got, c.want, 1e-12) {
			t.Errorf("%s: Spearman = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestQuintileSpread(t *testing.T) {
	cases := []struct {
		name string
		sig  []float64
		tgt  []float64
		want float64
		ok   bool
	}{
		// 10 names: k = 2. Top two by signal (9,10) have targets 0.09, 0.10; bottom two (1,2) 0.01, 0.02.
		{"aligned", seq(1, 10), scale(seq(1, 10), 0.01), (0.19 - 0.03) / 2, true},
		// Same targets, reversed signal ordering → negative spread.
		{"reversed", rev(seq(1, 10)), scale(seq(1, 10), 0.01), -(0.19 - 0.03) / 2, true},
		// 5 names: k = 1, top minus bottom.
		{"five", []float64{3, 1, 2, 5, 4}, []float64{0.3, 0.1, 0.2, 0.5, 0.4}, 0.4, true},
		{"too few", []float64{1, 2, 3, 4}, []float64{1, 2, 3, 4}, 0, false},
	}
	for _, c := range cases {
		got, ok := QuintileSpread(c.sig, c.tgt)
		if ok != c.ok {
			t.Errorf("%s: ok = %v, want %v", c.name, ok, c.ok)
			continue
		}
		if ok && !near(got, c.want, 1e-12) {
			t.Errorf("%s: spread = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestMeanStdTStatHit(t *testing.T) {
	xs := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	m, sd := MeanStd(xs)
	if m != 5 || !near(sd, 2.138089935, 1e-9) {
		t.Errorf("MeanStd = %v, %v", m, sd)
	}
	if got := TStat(xs); !near(got, 5/(2.138089935/math.Sqrt(8)), 1e-9) {
		t.Errorf("TStat = %v", got)
	}
	if got := TStat([]float64{1, 1, 1}); !math.IsNaN(got) {
		t.Errorf("TStat of constant should be NaN, got %v", got)
	}
	if got := HitRate([]float64{1, -1, 0, 2}); got != 0.5 {
		t.Errorf("HitRate = %v, want 0.5", got)
	}
}

func TestMaxDrawdown(t *testing.T) {
	cases := []struct {
		curve []float64
		want  float64
	}{
		{[]float64{0, 1, 2, 3}, 0},
		{[]float64{0, 2, 1, 3, 0.5}, 2.5},
		{[]float64{0, -1, -2}, 2},
		{nil, 0},
	}
	for _, c := range cases {
		if got := MaxDrawdown(c.curve); !near(got, c.want, 1e-12) {
			t.Errorf("MaxDrawdown(%v) = %v, want %v", c.curve, got, c.want)
		}
	}
}

func seq(a, b int) []float64 {
	out := make([]float64, 0, b-a+1)
	for i := a; i <= b; i++ {
		out = append(out, float64(i))
	}
	return out
}

func scale(xs []float64, k float64) []float64 {
	out := make([]float64, len(xs))
	for i, x := range xs {
		out[i] = x * k
	}
	return out
}

func rev(xs []float64) []float64 {
	out := make([]float64, len(xs))
	for i, x := range xs {
		out[len(xs)-1-i] = x
	}
	return out
}

func TestJSONNaNSafe(t *testing.T) {
	c := Cell{Signal: "beta", Window: 3, Horizon: 4, MeanIC: NaN, N: 0}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"meanIC":null`) || !strings.Contains(s, `"signal":"beta"`) || !strings.Contains(s, `"windowDays":3`) {
		t.Errorf("json = %s", s)
	}
	p := PresetResult{Name: "x", HitRate: NaN, Curve: []float64{1}}
	b, err = json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if s := string(b); !strings.Contains(s, `"hitRate":null`) || strings.Contains(s, "Curve") {
		t.Errorf("json = %s", s)
	}
}
