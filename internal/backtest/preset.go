package backtest

import "math"

// Preset is a screener preset replayed as a portfolio: at every H-th hour
// take the instruments whose signals match, hold them equal weight for H
// hours, then rebalance. Short presets book the negative of the basket's
// excess return (a short basket hedged with a long benchmark).
type Preset struct {
	Name  string
	Short bool
	Match func(Signals) bool
}

// DefaultPresets are the two strength presets of the screener prototype
// (screener-v1.html): Strong vs BTC and its mirror Weak vs BTC. The weak
// preset keeps beta ≥ 1 as the prototype does: it looks for high-beta names
// that lag, not for low-beta ones.
var DefaultPresets = []Preset{
	{Name: "Strong vs BTC", Match: func(s Signals) bool {
		return s[SigVsBtc24h] > 0 && s[SigPresence] > 0.6 && s[SigBeta] > 1
	}},
	{Name: "Weak vs BTC", Short: true, Match: func(s Signals) bool {
		return s[SigVsBtc24h] < 0 && s[SigPresence] < 0.45 && s[SigBeta] > 1
	}},
}

// PresetResult is the replay of one preset at one horizon.
type PresetResult struct {
	Name     string  `json:"name"`
	Short    bool    `json:"short"`
	Window   int     `json:"windowDays"`
	Horizon  int     `json:"horizonHours"`
	Periods  int     `json:"periods"`  // rebalance periods evaluated
	NonEmpty int     `json:"nonEmpty"` // periods with at least one member
	AvgSize  float64 `json:"avgSize"`  // average members over non-empty periods

	Gross   float64   `json:"grossExcess"` // cumulative excess log return before fees
	Fees    float64   `json:"fees"`        // cumulative fees (log units, approx.)
	Net     float64   `json:"netExcess"`   // Gross − Fees
	MaxDD   float64   `json:"maxDrawdown"` // of the net excess curve
	HitRate float64   `json:"hitRate"`     // share of non-empty periods with net excess > 0
	Half1   float64   `json:"netHalf1"`    // net excess accumulated in the first half of the range
	Half2   float64   `json:"netHalf2"`
	Curve   []float64 `json:"-"` // net cumulative excess after each period
}

// RunPreset replays pr on the panel. windowDays sizes the signal window,
// horizon the holding period and rebalance step, feeBps the per-side fee
// charged on turnover Σ|Δw| (buying a new name and selling an old one are
// one unit of turnover each). mid is the slot index that splits the range in
// halves for the robustness numbers.
func RunPreset(p *Panel, pr Preset, windowDays, horizon int, feeBps float64, mid int) PresetResult {
	n := windowDays * 24
	r := PresetResult{Name: pr.Name, Short: pr.Short, Window: windowDays, Horizon: horizon}
	fee := feeBps / 1e4
	prev := map[int]float64{} // weights held into this period
	cum := 0.0
	wins := 0
	sizeSum := 0
	for t := n; t+horizon < p.Slots(); t += horizon {
		var members []int
		for i := range p.Inst {
			s, ok := p.signalsAt(i, t, n)
			if !ok || !pr.Match(s) {
				continue
			}
			if _, ok := p.ownReturn(i, t, horizon); !ok {
				continue
			}
			members = append(members, i)
		}
		bench, okB := p.benchReturn(t, horizon)
		if !okB {
			// No benchmark return: cannot measure excess; skip the period
			// without touching the book.
			continue
		}
		r.Periods++
		next := map[int]float64{}
		if len(members) > 0 {
			w := 1 / float64(len(members))
			for _, i := range members {
				next[i] = w
			}
		}
		// Period excess of the equal-weight basket.
		ex := 0.0
		if len(members) > 0 {
			sum := 0.0
			for _, i := range members {
				rr, _ := p.ownReturn(i, t, horizon)
				sum += rr
			}
			basket := sum / float64(len(members))
			ex = math.Log1p(basket) - math.Log1p(bench)
			if pr.Short {
				ex = -ex
			}
			r.NonEmpty++
			sizeSum += len(members)
			if ex > 0 {
				wins++
			}
		}
		turnover := 0.0
		for i, w := range next {
			turnover += math.Abs(w - prev[i])
		}
		for i, w := range prev {
			if _, ok := next[i]; !ok {
				turnover += w
			}
		}
		f := turnover * fee
		r.Gross += ex
		r.Fees += f
		cum += ex - f
		r.Curve = append(r.Curve, cum)
		if t < mid {
			r.Half1 = cum
		}
		prev = next
	}
	r.Net = r.Gross - r.Fees
	r.Half2 = r.Net - r.Half1
	r.MaxDD = MaxDrawdown(append([]float64{0}, r.Curve...))
	if r.NonEmpty > 0 {
		r.AvgSize = float64(sizeSum) / float64(r.NonEmpty)
		r.HitRate = float64(wins) / float64(r.NonEmpty)
	} else {
		r.AvgSize, r.HitRate = 0, NaN
	}
	return r
}
