package backtest

import "time"

// Config of one run.
type Config struct {
	Windows      []int   `json:"windowsDays"`   // signal windows, days of hourly returns
	Horizons     []int   `json:"horizonsHours"` // forward horizons, hours
	MinCross     int     `json:"minCross"`      // minimum instruments per cross-section, default 10
	FeeBps       float64 `json:"feeBps"`        // per side, preset portfolio
	PresetWindow int     `json:"presetWindow"`  // days, default 7
	Benchmark    string  `json:"benchmark"`     // for the report only
}

func (c Config) withDefaults() Config {
	if c.MinCross <= 0 {
		c.MinCross = 10
	}
	if c.PresetWindow <= 0 {
		c.PresetWindow = 7
	}
	if len(c.Windows) == 0 {
		c.Windows = []int{3, 7, 14}
	}
	if len(c.Horizons) == 0 {
		c.Horizons = []int{4, 24, 72}
	}
	return c
}

// Cell is the cross-sectional result of one signal × window × horizon.
type Cell struct {
	Signal  string `json:"signal"`
	Window  int    `json:"windowDays"`
	Horizon int    `json:"horizonHours"`
	Control bool   `json:"control"`

	N           int     `json:"n"`           // hours with a cross-section
	AvgCross    float64 `json:"avgCross"`    // average instruments per hour
	MeanIC      float64 `json:"meanIC"`      // mean Spearman IC across hours
	StdIC       float64 `json:"stdIC"`       // its standard deviation
	TOverlap    float64 `json:"tOverlap"`    // t of the mean, every hour an observation
	TNonOverlap float64 `json:"tNonOverlap"` // t on every H-th hour only
	NNonOverlap int     `json:"nNonOverlap"`
	HitRate     float64 `json:"hitRate"`   // share of hours with IC > 0
	Spread      float64 `json:"spread"`    // mean Q5 − Q1 excess log return per horizon
	SpreadAnn   float64 `json:"spreadAnn"` // Spread × 8760 / H
	ICHalf1     float64 `json:"icHalf1"`   // mean IC, first half of the range
	ICHalf2     float64 `json:"icHalf2"`
	SpreadHalf1 float64 `json:"spreadHalf1"`
	SpreadHalf2 float64 `json:"spreadHalf2"`
}

// Result of a run.
type Result struct {
	From      time.Time      `json:"from"`
	To        time.Time      `json:"to"`
	Slots     int            `json:"slots"`
	Universe  int            `json:"universe"`
	Bars      int            `json:"bars"`
	Benchmark string         `json:"benchmark"`
	Config    Config         `json:"config"`
	Cells     []Cell         `json:"cells"`
	Presets   []PresetResult `json:"presets"`
}

// obs is one hour's cross-sectional measurement.
type obs struct {
	t      int
	ic     float64
	spread float64
	cross  int
}

// Run computes every cell and the preset portfolios.
func Run(p *Panel, cfg Config) *Result {
	cfg = cfg.withDefaults()
	res := &Result{Slots: p.Slots(), Universe: len(p.Inst), Config: cfg, Benchmark: cfg.Benchmark}
	if p.Slots() > 0 {
		res.From, res.To = p.Time(0), p.Time(p.Slots()-1)
	}
	for i := range p.Inst {
		res.Bars += p.Bars(i)
	}
	mid := p.Slots() / 2
	nH := len(cfg.Horizons)
	for _, w := range cfg.Windows {
		n := w * 24
		// series[s][h] = observations across hours.
		series := make([][][]obs, NumSignals)
		for s := range series {
			series[s] = make([][]obs, nH)
		}
		sigs := make([]Signals, 0, len(p.Inst))
		tgts := make([][]float64, 0, len(p.Inst))
		for t := n; t < p.Slots(); t++ {
			sigs, tgts = sigs[:0], tgts[:0]
			for i := range p.Inst {
				s, ok := p.signalsAt(i, t, n)
				if !ok {
					continue
				}
				tg := make([]float64, nH)
				got := false
				for h, hh := range cfg.Horizons {
					if v, ok := p.excess(i, t, hh); ok {
						tg[h], got = v, true
					} else {
						tg[h] = NaN
					}
				}
				if !got {
					continue
				}
				sigs = append(sigs, s)
				tgts = append(tgts, tg)
			}
			if len(sigs) < cfg.MinCross {
				continue
			}
			var xs, ys []float64
			for s := 0; s < NumSignals; s++ {
				for h := 0; h < nH; h++ {
					xs, ys = xs[:0], ys[:0]
					for k := range sigs {
						if x, y := sigs[k][s], tgts[k][h]; finite(x) && finite(y) {
							xs = append(xs, x)
							ys = append(ys, y)
						}
					}
					if len(xs) < cfg.MinCross {
						continue
					}
					ic, ok := Spearman(xs, ys)
					if !ok {
						continue
					}
					sp, _ := QuintileSpread(xs, ys)
					series[s][h] = append(series[s][h], obs{t: t, ic: ic, spread: sp, cross: len(xs)})
				}
			}
		}
		for s := 0; s < NumSignals; s++ {
			for h, hh := range cfg.Horizons {
				res.Cells = append(res.Cells, summarise(SignalNames[s], IsControl(s), w, hh, series[s][h], mid))
			}
		}
	}
	SortCells(res.Cells)
	for _, pr := range DefaultPresets {
		for _, hh := range cfg.Horizons {
			res.Presets = append(res.Presets, RunPreset(p, pr, cfg.PresetWindow, hh, cfg.FeeBps, mid))
		}
	}
	return res
}

func summarise(name string, control bool, w, h int, o []obs, mid int) Cell {
	c := Cell{Signal: name, Control: control, Window: w, Horizon: h, N: len(o),
		MeanIC: NaN, StdIC: NaN, TOverlap: NaN, TNonOverlap: NaN, HitRate: NaN, Spread: NaN, SpreadAnn: NaN,
		ICHalf1: NaN, ICHalf2: NaN, SpreadHalf1: NaN, SpreadHalf2: NaN}
	if len(o) == 0 {
		return c
	}
	ics := make([]float64, 0, len(o))
	var nonOverlap, sps, ic1, ic2, sp1, sp2 []float64
	cross := 0
	for _, x := range o {
		ics = append(ics, x.ic)
		cross += x.cross
		if (x.t-o[0].t)%h == 0 {
			nonOverlap = append(nonOverlap, x.ic)
		}
		if finite(x.spread) {
			sps = append(sps, x.spread)
		}
		if x.t < mid {
			ic1 = append(ic1, x.ic)
			if finite(x.spread) {
				sp1 = append(sp1, x.spread)
			}
		} else {
			ic2 = append(ic2, x.ic)
			if finite(x.spread) {
				sp2 = append(sp2, x.spread)
			}
		}
	}
	c.AvgCross = float64(cross) / float64(len(o))
	c.MeanIC, c.StdIC = MeanStd(ics)
	c.TOverlap = TStat(ics)
	c.TNonOverlap = TStat(nonOverlap)
	c.NNonOverlap = len(nonOverlap)
	c.HitRate = HitRate(ics)
	if len(sps) > 0 {
		c.Spread = mean(sps)
		c.SpreadAnn = c.Spread * 8760 / float64(h)
	}
	c.ICHalf1, c.ICHalf2 = mean(ic1), mean(ic2)
	c.SpreadHalf1, c.SpreadHalf2 = mean(sp1), mean(sp2)
	return c
}
