package venue

import (
	"math"
	"sort"
	"time"

	"screener/internal/collector"
	"screener/internal/metrics"
)

type f = collector.F

var nan = math.NaN()

// blockAcc folds the contracts of one asset on one exchange.
type blockAcc struct {
	vol, oi      float64
	hasOI, has   bool
	top          float64
	p, c, fr, fi float64
}

func (b *blockAcc) add(p, c, vol, oi, fr, fi float64) {
	if math.IsNaN(vol) || vol < 0 {
		vol = 0
	}
	b.vol += vol
	if oi > 0 { // NaN and zero both mean "no reading"
		b.oi += oi
		b.hasOI = true
	}
	if !b.has || vol > b.top {
		b.has, b.top = true, vol
		b.p, b.c, b.fr, b.fi = p, c, fr, fi
	}
}

func (b *blockAcc) block() Block {
	oi := nan
	if b.hasOI {
		oi = b.oi
	}
	fi := b.fi
	if !(fi > 0) {
		fi = 8
	}
	return Block{P: f(b.p), C: f(b.c), Vol: f(b.vol), OI: f(oi), F: f(b.fr), FI: fi}
}

type assetAcc struct {
	key, base string
	classes   map[string]bool
	kinds     map[string]bool
	tags      []string
	blocks    map[string]*blockAcc
	okx       *collector.Row // OKX's busiest contract of the asset
}

func (a *assetAcc) blk(venue string) *blockAcc {
	b := a.blocks[venue]
	if b == nil {
		b = &blockAcc{}
		a.blocks[venue] = b
	}
	return b
}

// okxClass maps the collector's class onto the shared ones.
func okxClass(c, base string) (class, kind string) {
	switch c {
	case "crypto":
		return ClassCrypto, ""
	case "commodity":
		if indexTickers[base] {
			return ClassStock, "Index" // OKX files the Hang Seng index under commodities
		}
		return ClassCommodity, "Commodity"
	case "premarket":
		return ClassStock, "Pre-IPO"
	}
	return ClassStock, "Stock"
}

// build folds the three exchanges into asset rows, busiest first.
func (s *Service) build(now time.Time) []Asset {
	rows, _ := s.rows.Rows()
	s.mu.RLock()
	defer s.mu.RUnlock()

	accs := map[string]*assetAcc{}
	get := func(class, kind, base string, tags []string) *assetAcc {
		k := Key(class, base)
		a := accs[k]
		if a == nil {
			a = &assetAcc{key: k, base: base, classes: map[string]bool{}, kinds: map[string]bool{}, blocks: map[string]*blockAcc{}}
			accs[k] = a
		}
		a.classes[class] = true
		if kind != "" {
			a.kinds[kind] = true
		}
		a.tags = append(a.tags, tags...)
		return a
	}

	for i := range rows {
		r := &rows[i]
		class, kind := okxClass(string(r.Class), r.Symbol)
		a := get(class, kind, r.Symbol, nil)
		// The 24 h change is the exchange's own rolling figure, as on Binance and Bybit; the row's chg["24h"] is
		// measured from an hourly close and can differ by the move inside that hour.
		chg := float64(r.ChgRoll)
		if math.IsNaN(chg) {
			chg = float64(r.Chg["24h"])
		}
		a.blk(OKX).add(float64(r.Last), chg, float64(r.Vol24), float64(r.OI), float64(r.Funding), float64(r.FundingInterval))
		if a.okx == nil || float64(r.Vol24) > float64(a.okx.Vol24) {
			a.okx = r
		}
	}
	for sym, in := range s.inst[Binance] {
		t, ok := s.tick[Binance][sym]
		if !ok || !(t.Last > 0) {
			continue
		}
		fr, mark := nan, t.Last
		if p, ok := s.premium[sym]; ok {
			fr = p.Funding
			if p.Mark > 0 {
				mark = p.Mark
			}
		}
		oi := nan
		if r, ok := s.bnOI[sym]; ok {
			oi = r.contracts * mark // contracts of the listed symbol at its own mark price
		}
		get(in.Class, in.Kind, in.Base, in.Tags).blk(Binance).add(t.Last/in.Mult, t.Chg24, t.Vol24, oi, fr, in.FundingH)
	}
	for sym, in := range s.inst[Bybit] {
		t, ok := s.tick[Bybit][sym]
		if !ok || !(t.Last > 0) {
			continue
		}
		fi := t.FundingH
		if !(fi > 0) {
			fi = in.FundingH
		}
		get(in.Class, in.Kind, in.Base, nil).blk(Bybit).add(t.Last/in.Mult, t.Chg24, t.Vol24, t.OIUsd, t.Funding, fi)
	}

	bench := s.series[benchKey]
	benchLast := nan
	if bench != nil {
		if t, ok := s.tick[bench.inst.Venue][bench.inst.Symbol]; ok {
			benchLast = t.Last / bench.inst.Mult
		}
	}

	out := make([]Asset, 0, len(accs))
	for _, a := range accs {
		as := Asset{K: a.key, S: a.base, Mcap: f(nan), Rank: f(nan), V: make(map[string]Block, len(a.blocks))}
		last, top := nan, -1.0 // the live price: that of the busiest exchange
		for v, b := range a.blocks {
			as.V[v] = b.block()
			if b.vol > top && b.p > 0 {
				top, last = b.vol, b.p
			}
		}
		kind := ""
		for _, k := range kindOrder {
			if a.kinds[k] || (k == "Index" && indexTickers[a.base] && !a.classes[ClassCrypto]) {
				kind = k
				break
			}
		}
		switch {
		case a.classes[ClassCrypto]:
			as.Cls = ClassCrypto
			if s.meta != nil {
				if m, ok := s.meta.Lookup(a.base); ok {
					as.N, as.Sec = m.Name, m.Sector
					if m.Mcap > 0 {
						as.Mcap = f(m.Mcap)
					}
					if m.Rank > 0 {
						as.Rank = f(float64(m.Rank))
					}
				}
			}
			if a.kinds["Index"] {
				as.Sec = "Index"
			}
			if as.Sec == "" {
				if as.Sec = SectorFromTags(a.tags); as.Sec == "" {
					as.Sec = "Other"
				}
			}
		default:
			as.Cls = ClassStock
			if a.classes[ClassCommodity] && kind != "Index" {
				as.Cls = ClassCommodity
			} else if a.classes[ClassFX] {
				as.Cls = ClassFX
			}
			if kind == "" {
				kind = "Stock"
			}
			as.Sec = kind
			if a.okx != nil {
				as.N = a.okx.Name
			}
		}
		if a.okx != nil {
			as.M = fromRow(a.okx)
		} else {
			se := s.series[a.key]
			vol := nan
			if se != nil {
				vol = s.tick[se.inst.Venue][se.inst.Symbol].Vol24
			}
			as.M = candleMetrics(se, bench, last, benchLast, vol, now)
		}
		out = append(out, as)
	}
	vol := func(a Asset) float64 {
		t := 0.0
		for _, b := range a.V {
			if v := float64(b.Vol); v > 0 {
				t += v
			}
		}
		return t
	}
	sort.Slice(out, func(i, j int) bool {
		vi, vj := vol(out[i]), vol(out[j])
		if vi != vj {
			return vi > vj
		}
		return out[i].K < out[j].K
	})
	return out
}

// fromRow takes the asset-level metrics from an OKX row.
func fromRow(r *collector.Row) Metrics {
	two := func(p collector.PerPeriod) collector.PerPeriod {
		out := collector.PerPeriod{}
		for _, k := range []string{"1h", "4h"} {
			if v, ok := p[k]; ok {
				out[k] = v
			} else {
				out[k] = f(nan)
			}
		}
		return out
	}
	return Metrics{
		Spark: r.Spark, Chg: two(r.Chg), Chg7d: r.Chg7d, Chg30d: r.Chg30d, VolX7: r.VolX7, VolX: r.VolX, OID: r.OID,
		ATR: r.ATR, Regime: r.Regime, TakerBuy: r.TakerBuy, Delta: r.Delta, Tape: r.Tape, VsBtc: two(r.VsBtc),
		Beta: r.Beta, Corr: r.Corr, Spread: r.Spread, Depth05: r.Depth05,
		Lv: map[string]f{"do": r.DO.Px, "wo": r.WO.Px, "mo": r.MO.Px, "ma50": r.MA50.Px, "ma100": r.MA100.Px, "ma200": r.MA200.Px},
	}
}

// candleMetrics computes the price metrics of an asset outside OKX from its
// candles, with the formulas the collector uses for OKX rows. last is the
// asset's live price, vol24 the 24 h volume of the instrument the candles
// belong to. What needs trades, the order book or open-interest history stays
// empty.
func candleMetrics(se, bench *series, last, benchLast, vol24 float64, now time.Time) Metrics {
	m := Metrics{
		Chg: collector.PerPeriod{}, VolX: collector.PerPeriod{}, VsBtc: collector.PerPeriod{},
		Chg7d: f(nan), Chg30d: f(nan), VolX7: f(nan), ATR: f(nan), Regime: f(nan), Tape: f(nan),
		Beta: f(nan), Corr: f(nan), Spread: f(nan), Depth05: f(nan),
	}
	for _, p := range []string{"1h", "4h"} {
		m.Chg[p], m.VolX[p], m.VsBtc[p] = f(nan), f(nan), f(nan)
	}
	if se == nil {
		return m
	}
	for _, p := range []string{"1h", "4h"} {
		h := collector.PeriodHours[p]
		chg := metrics.ChangeHours(last, se.minute, se.hourly, now, h)
		m.Chg[p] = f(chg)
		m.VolX[p] = f(metrics.VolVsHours(se.minute, se.minuteAt, se.hourly, se.hourlyAt, h, 168))
		if bench != nil {
			m.VsBtc[p] = f(chg - metrics.ChangeHours(benchLast, bench.minute, bench.hourly, now, h))
		}
	}
	if n := len(se.hourly); n > 1 {
		k := 24
		if k > n {
			k = n
		}
		m.Spark = make([]f, 0, k)
		for i := k - 1; i >= 0; i-- {
			m.Spark = append(m.Spark, f(se.hourly[i].C))
		}
	}
	m.ATR = f(metrics.ATRPct(last, se.hourly, 14))
	if bench != nil {
		st := metrics.StrengthVs(se.ret, bench.ret)
		m.Beta, m.Corr = f(st.Beta), f(st.Corr)
	}
	m.Chg7d = f(metrics.ChangeDays(last, se.hourly, se.daily, now, 7))
	m.Chg30d = f(metrics.ChangeDays(last, se.hourly, se.daily, now, 30))
	m.VolX7 = f(metrics.VolVsDays(vol24, se.daily, 7))
	m.Regime = f(metrics.VolRegime(se.hourly, se.daily, 20))
	day, week, month := metrics.PeriodStarts(now)
	m.Lv = map[string]f{
		"do": f(metrics.OpenAt(se.daily, day)), "wo": f(metrics.OpenAt(se.daily, week)), "mo": f(metrics.OpenAt(se.daily, month)),
		"ma50": f(metrics.SMA(se.daily, 50)), "ma100": f(metrics.SMA(se.daily, 100)), "ma200": f(metrics.SMA(se.daily, 200)),
	}
	return m
}
