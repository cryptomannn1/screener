package backtest

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Markdown renders the result as the report described in the test plan:
// range and universe, one IC table per horizon, the preset replays, the
// first-half / second-half split and a plain-language reading.
func Markdown(r *Result) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	w("# Strength vs BTC backtest\n\n")
	w("- Range: %s to %s UTC (%d hourly slots, %.1f days)\n", r.From.Format("2006-01-02 15:04"), r.To.Format("2006-01-02 15:04"), r.Slots, float64(r.Slots)/24)
	w("- Benchmark: %s\n", r.Benchmark)
	w("- Universe: %d instruments, %d bars\n", r.Universe, r.Bars)
	w("- Windows: %s days, horizons: %s hours, min cross-section: %d, fee: %.1f bps per side, preset window: %d days\n\n",
		joinInts(r.Config.Windows), joinInts(r.Config.Horizons), r.Config.MinCross, r.Config.FeeBps, r.Config.PresetWindow)

	w("## How to read\n\n")
	w("IC is the Spearman rank correlation between a signal and the excess log return over the benchmark for the next H hours, ")
	w("computed across instruments at every hour and averaged. t (overlapping) treats every hour as an observation; ")
	w("consecutive hours share most of their forward window, so it overstates significance. t (non-overlapping) uses every H-th hour only; ")
	w("|t| > 2 there is the threshold for weak evidence. Hit rate is the share of hours with IC > 0. Q5−Q1 is the average excess log return ")
	w("of the top signal quintile minus the bottom quintile per horizon (and the same number naively scaled to a year). ")
	w("vsBtc4h, vsBtc24h and the control mom24 do not depend on the window; their rows differ only by which instruments have enough history. ")
	w("Note that vsBtc24h and mom24 have IDENTICAL rows by construction: at a given hour BTC's return is the same number for every instrument, ")
	w("so subtracting it shifts every value by a constant and leaves the cross-sectional ranks unchanged; vsBtc4h is likewise rank-identical to plain 4 h momentum. ")
	w("The vs-BTC columns can matter as thresholds (presets) or over time, not as a cross-sectional ranking.\n\n")

	for _, h := range r.Config.Horizons {
		w("## Horizon %dh\n\n", h)
		w("| signal | W (days) | hours | avg N | mean IC | IC std | t (overlap) | t (non-overlap, n) | hit rate | Q5−Q1 per %dh | Q5−Q1 ann. |\n", h)
		w("|---|---|---|---|---|---|---|---|---|---|---|\n")
		for _, c := range r.Cells {
			if c.Horizon != h {
				continue
			}
			name := c.Signal
			if c.Control {
				name += " (control)"
			}
			w("| %s | %d | %d | %s | %s | %s | %s | %s (%d) | %s | %s | %s |\n",
				name, c.Window, c.N, f1(c.AvgCross), f3(c.MeanIC), f3(c.StdIC), f2(c.TOverlap), f2(c.TNonOverlap), c.NNonOverlap,
				pct(c.HitRate, 0), pct(c.Spread, 2), pct(c.SpreadAnn, 0))
		}
		w("\n")
	}

	w("## Preset portfolios (W = %d days, equal weight, rebalanced every H hours, %.1f bps per side)\n\n", r.Config.PresetWindow, r.Config.FeeBps)
	w("Strong vs BTC: vsBtc24h > 0, presence > 0.6, beta > 1. Weak vs BTC (short, booked as negative excess): vsBtc24h < 0, presence < 0.45, beta > 1. ")
	w("An empty basket earns zero excess for that period. Fees are charged on turnover Σ|Δw|. ")
	w("With a 7-day window, presence is the share of 168 hours above BTC; in practice it rarely exceeds 0.6 for any name, so the Strong basket can be empty for long stretches.\n\n")
	w("| preset | H | periods | non-empty | avg size | gross excess | fees | net excess | max DD | hit rate | net 1st half | net 2nd half |\n")
	w("|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, p := range r.Presets {
		w("| %s | %dh | %d | %d | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			p.Name, p.Horizon, p.Periods, p.NonEmpty, f1(p.AvgSize), pct(p.Gross, 2), pct(p.Fees, 2), pct(p.Net, 2), pct(p.MaxDD, 2), pct(p.HitRate, 0), pct(p.Half1, 2), pct(p.Half2, 2))
	}
	w("\n")

	w("## Robustness: first half vs second half of the range\n\n")
	for _, h := range r.Config.Horizons {
		w("Horizon %dh:\n\n", h)
		w("| signal | W | IC 1st half | IC 2nd half | same sign | Q5−Q1 1st | Q5−Q1 2nd |\n|---|---|---|---|---|---|---|\n")
		for _, c := range r.Cells {
			if c.Horizon != h {
				continue
			}
			same := "n/a"
			if finite(c.ICHalf1) && finite(c.ICHalf2) {
				if (c.ICHalf1 > 0) == (c.ICHalf2 > 0) {
					same = "yes"
				} else {
					same = "no"
				}
			}
			w("| %s | %d | %s | %s | %s | %s | %s |\n", c.Signal, c.Window, f3(c.ICHalf1), f3(c.ICHalf2), same, pct(c.SpreadHalf1, 2), pct(c.SpreadHalf2, 2))
		}
		w("\n")
	}

	w("## What this says\n\n")
	for _, line := range Reading(r) {
		w("- %s\n", line)
	}
	w("\n")

	w("## Limitations\n\n")
	w("- One venue (OKX) and one period: a single market regime, not a general result.\n")
	w("- Survivorship: only instruments listed today are in the store; delisted names of the period are missing, and ranking by today's volume favours recent winners.\n")
	w("- Hourly closes only: no intra-hour fills, no funding, no slippage beyond the flat fee; the preset replay is a diagnostic, not a strategy test.\n")
	w("- Overlapping forward windows inflate the per-hour t-stat; rely on the non-overlapping column, which has few observations at 72h.\n")
	w("- Signals are computed on paired hours only: an instrument missing a bar in the window is skipped for that hour.\n")
	return b.String()
}

// Reading produces the plain-language lines of the "what this says" section:
// per horizon, the best strength signal by |t (non-overlap)| against the
// momentum control at the same window, and whether anything clears |t| > 2.
func Reading(r *Result) []string {
	var out []string
	const th = 2.0
	anyEvidence := false
	for _, h := range r.Config.Horizons {
		var cells []Cell
		for _, c := range r.Cells {
			if c.Horizon == h && c.N > 0 {
				cells = append(cells, c)
			}
		}
		if len(cells) == 0 {
			out = append(out, fmt.Sprintf("%dh: no cross-sections could be formed (not enough history for the windows), nothing to say.", h))
			continue
		}
		var best *Cell
		for i := range cells {
			c := &cells[i]
			if c.Control || !finite(c.TNonOverlap) {
				continue
			}
			if best == nil || math.Abs(c.TNonOverlap) > math.Abs(best.TNonOverlap) {
				best = c
			}
		}
		if best == nil {
			out = append(out, fmt.Sprintf("%dh: no strength signal has a usable non-overlapping t-stat (too few periods).", h))
			continue
		}
		var ctrl *Cell
		for i := range cells {
			if cells[i].Control && cells[i].Window == best.Window {
				ctrl = &cells[i]
			}
		}
		line := fmt.Sprintf("%dh: the strongest strength signal is %s (W=%d) with mean IC %s, t(non-overlap) %s on %d periods, hit rate %s.",
			h, best.Signal, best.Window, f3(best.MeanIC), f2(best.TNonOverlap), best.NNonOverlap, pct(best.HitRate, 0))
		if ctrl != nil {
			line += fmt.Sprintf(" The momentum control at the same window has mean IC %s, t(non-overlap) %s.", f3(ctrl.MeanIC), f2(ctrl.TNonOverlap))
			beats := math.Abs(best.MeanIC) > math.Abs(ctrl.MeanIC)
			if best.Signal == SignalNames[SigVsBtc24h] {
				line += " (vsBtc24h is rank-identical to mom24, so this is plain 24 h momentum.)"
			} else if best.Signal == SignalNames[SigVsBtc4h] {
				line += " (vsBtc4h is rank-identical to the instrument's own 4 h return, so this is short-term momentum or reversal, not relative strength.)"
			} else if beats {
				line += " The strength signal has the larger |IC| of the two."
			} else {
				line += " Plain momentum has the larger |IC|; subtracting BTC did not add predictive rank information here."
			}
		}
		if math.Abs(best.TNonOverlap) > th {
			anyEvidence = true
			line += fmt.Sprintf(" |t| > %.0f on non-overlapping periods: weak evidence of a real effect in this sample", th)
			if best.MeanIC < 0 {
				line += " with a NEGATIVE sign (strong names underperformed)."
			} else {
				line += "."
			}
		} else {
			line += fmt.Sprintf(" |t| ≤ %.0f on non-overlapping periods: no evidence beyond noise at this horizon.", th)
		}
		out = append(out, line)
	}
	// Presets: one line each on the headline horizon (the middle one).
	hh := r.Config.Horizons
	if len(hh) > 0 {
		head := hh[len(hh)/2]
		for _, p := range r.Presets {
			if p.Horizon != head {
				continue
			}
			if p.Periods == 0 {
				out = append(out, fmt.Sprintf("Preset %s at %dh: no periods evaluated.", p.Name, head))
				continue
			}
			out = append(out, fmt.Sprintf("Preset %s at %dh: net excess %s over %d periods (%d non-empty, avg %s names), max drawdown %s, halves %s / %s.",
				p.Name, head, pct(p.Net, 2), p.Periods, p.NonEmpty, f1(p.AvgSize), pct(p.MaxDD, 2), pct(p.Half1, 2), pct(p.Half2, 2)))
		}
	}
	if !anyEvidence {
		out = append(out, "Overall: no signal × window × horizon clears |t| > 2 on non-overlapping periods. Treat every number above as noise-level until more history is backfilled.")
	} else {
		out = append(out, "Overall: at least one cell clears |t| > 2 on non-overlapping periods. That is weak evidence in one sample; check that the sign agrees in both halves before acting on it.")
	}
	return out
}

func f1(x float64) string { return fx(x, 1) }
func f2(x float64) string { return fx(x, 2) }
func f3(x float64) string { return fx(x, 3) }

func fx(x float64, d int) string {
	if !finite(x) {
		return "n/a"
	}
	return fmt.Sprintf("%.*f", d, x)
}

func pct(x float64, d int) string {
	if !finite(x) {
		return "n/a"
	}
	return fmt.Sprintf("%.*f%%", d, x*100)
}

func joinInts(xs []int) string {
	s := make([]string, len(xs))
	for i, x := range xs {
		s[i] = fmt.Sprint(x)
	}
	return strings.Join(s, ",")
}

// SortCells orders cells by horizon, then signal index, then window, so the
// tables read the same regardless of computation order.
func SortCells(cells []Cell) {
	idx := map[string]int{}
	for i, n := range SignalNames {
		idx[n] = i
	}
	sort.SliceStable(cells, func(a, b int) bool {
		x, y := cells[a], cells[b]
		if x.Horizon != y.Horizon {
			return x.Horizon < y.Horizon
		}
		if idx[x.Signal] != idx[y.Signal] {
			return idx[x.Signal] < idx[y.Signal]
		}
		return x.Window < y.Window
	})
}
