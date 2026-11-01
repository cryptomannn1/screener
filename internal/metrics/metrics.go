// Package metrics holds the pure arithmetic of the screener: every function
// takes plain slices and returns numbers, so the definitions can be unit
// tested and later replayed over stored history for the strength backtest.
//
// Conventions: candle slices are NEWEST FIRST (as OKX returns them, index 0 =
// the forming bar). Returns are simple ratios (0.012 = +1.2 %) unless the
// name says "log".
package metrics

import (
	"math"
	"sort"
	"time"
)

// Bar is the OHLCV subset the metrics need.
type Bar struct {
	TS         int64
	O, H, L, C float64
	QuoteVol   float64 // volume in quote currency
}

// NaN marks a metric that cannot be computed yet.
var NaN = math.NaN()

// Change returns last / close n bars ago − 1. bars newest first, bars[n] is the
// bar that CLOSED n bars ago (bars[0] is forming). NaN if not enough history.
func Change(last float64, bars []Bar, n int) float64 {
	if n <= 0 || n >= len(bars) || bars[n].C <= 0 || last <= 0 {
		return NaN
	}
	return last/bars[n].C - 1
}

// ATRPct is the Wilder ATR over the last n COMPLETE bars divided by last:
// the average true range as a share of price. Needs n+1 bars after the forming
// one.
func ATRPct(last float64, bars []Bar, n int) float64 {
	if last <= 0 || len(bars) < n+2 {
		return NaN
	}
	// Complete bars are bars[1:]; walk oldest → newest for Wilder smoothing.
	trs := make([]float64, 0, n)
	for i := n; i >= 1; i-- {
		trs = append(trs, trueRange(bars[i], bars[i+1]))
	}
	atr := 0.0
	for _, tr := range trs {
		atr += tr
	}
	atr /= float64(len(trs))
	return atr / last
}

func trueRange(b, prev Bar) float64 {
	hl := b.H - b.L
	hc := math.Abs(b.H - prev.C)
	lc := math.Abs(b.L - prev.C)
	return math.Max(hl, math.Max(hc, lc))
}

// LogReturns returns the log returns of the last n complete bars, oldest
// first: r_i = ln(C_i / C_{i-1}). bars newest first; the forming bar[0] is
// skipped. Fewer than n+2 bars → returns what is available.
func LogReturns(bars []Bar, n int) []float64 {
	if len(bars) < 3 {
		return nil
	}
	if n+2 > len(bars) {
		n = len(bars) - 2
	}
	out := make([]float64, 0, n)
	for i := n; i >= 1; i-- {
		a, b := bars[i+1].C, bars[i].C
		if a > 0 && b > 0 {
			out = append(out, math.Log(b/a))
		}
	}
	return out
}

// RealizedVol is the annualised standard deviation of hourly log returns:
// std(r) × sqrt(24 × 365). rs oldest first.
func RealizedVol(rs []float64, barsPerYear float64) float64 {
	if len(rs) < 2 {
		return NaN
	}
	_, sd := meanStd(rs)
	return sd * math.Sqrt(barsPerYear)
}

func meanStd(xs []float64) (mean, sd float64) {
	n := float64(len(xs))
	for _, x := range xs {
		mean += x
	}
	mean /= n
	for _, x := range xs {
		sd += (x - mean) * (x - mean)
	}
	sd = math.Sqrt(sd / (n - 1))
	return
}

// VolRegime compares today's true range with the median true range of the
// previous window complete days: < 0.6 reads as a squeeze, > 1.6 as an
// expansion. daily newest first (daily[0] = today, forming).
func VolRegime(daily []Bar, window int) float64 {
	if len(daily) < 4 {
		return NaN
	}
	if window+2 > len(daily) {
		window = len(daily) - 2
	}
	trs := make([]float64, 0, window)
	for i := 1; i <= window; i++ {
		trs = append(trs, trueRange(daily[i], daily[i+1])/daily[i].C)
	}
	med := median(trs)
	if med <= 0 || daily[0].C <= 0 {
		return NaN
	}
	today := trueRange(daily[0], daily[1]) / daily[0].C
	return today / med
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return NaN
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	m := len(s) / 2
	if len(s)%2 == 1 {
		return s[m]
	}
	return (s[m-1] + s[m]) / 2
}

// RangeToday is today's (high − low) / low.
func RangeToday(daily []Bar) float64 {
	if len(daily) == 0 || daily[0].L <= 0 {
		return NaN
	}
	return (daily[0].H - daily[0].L) / daily[0].L
}

// DistanceToHigh is last / max(high over the last n days incl. today) − 1 (≤ 0).
func DistanceToHigh(last float64, daily []Bar, n int) float64 {
	if last <= 0 || len(daily) == 0 {
		return NaN
	}
	if n > len(daily) {
		n = len(daily)
	}
	hi := 0.0
	for i := 0; i < n; i++ {
		hi = math.Max(hi, daily[i].H)
	}
	if hi <= 0 {
		return NaN
	}
	return last/hi - 1
}

// DistanceToLow is last / min(low over the last n days incl. today) − 1 (≥ 0).
func DistanceToLow(last float64, daily []Bar, n int) float64 {
	if last <= 0 || len(daily) == 0 {
		return NaN
	}
	if n > len(daily) {
		n = len(daily)
	}
	lo := math.Inf(1)
	for i := 0; i < n; i++ {
		if daily[i].L > 0 {
			lo = math.Min(lo, daily[i].L)
		}
	}
	if math.IsInf(lo, 1) {
		return NaN
	}
	return last/lo - 1
}

// VolVsDays is vol24 / average quote volume of the last n COMPLETE days.
func VolVsDays(vol24 float64, daily []Bar, n int) float64 {
	if vol24 <= 0 || len(daily) < 2 {
		return NaN
	}
	if n+1 > len(daily) {
		n = len(daily) - 1
	}
	sum := 0.0
	for i := 1; i <= n; i++ {
		sum += daily[i].QuoteVol
	}
	if sum <= 0 {
		return NaN
	}
	return vol24 / (sum / float64(n))
}

// VolVsPeriod is the quote volume of the last h hours (complete bars plus the
// forming one) divided by the average h-hour volume over the previous lookback
// hours. hourly newest first.
func VolVsPeriod(hourly []Bar, h, lookback int) float64 {
	if h <= 0 || len(hourly) < h+1 {
		return NaN
	}
	recent := 0.0
	for i := 0; i < h; i++ {
		recent += hourly[i].QuoteVol
	}
	avail := len(hourly) - h
	if lookback > avail {
		lookback = avail
	}
	if lookback < h {
		return NaN
	}
	base := 0.0
	for i := h; i < h+lookback; i++ {
		base += hourly[i].QuoteVol
	}
	if base <= 0 {
		return NaN
	}
	return recent / (base / float64(lookback) * float64(h))
}

// Strength is the relative-strength block versus a benchmark over paired
// hourly log returns (oldest first, same length).
type Strength struct {
	Beta     float64 // regression slope of r on b
	Corr     float64 // Pearson correlation
	Alpha    float64 // mean(r − β·b) per bar
	Presence float64 // share of bars with r > b
	Stay     float64 // P(r_{t+1} > b_{t+1} | r_t > b_t)
	N        int
}

// StrengthVs computes the block. Fewer than 24 pairs → all NaN.
func StrengthVs(r, b []float64) Strength {
	n := len(r)
	if len(b) < n {
		n = len(b)
	}
	s := Strength{Beta: NaN, Corr: NaN, Alpha: NaN, Presence: NaN, Stay: NaN, N: n}
	if n < 24 {
		return s
	}
	r, b = r[len(r)-n:], b[len(b)-n:]
	mr, sr := meanStd(r)
	mb, sb := meanStd(b)
	cov := 0.0
	for i := 0; i < n; i++ {
		cov += (r[i] - mr) * (b[i] - mb)
	}
	cov /= float64(n - 1)
	if sb > 0 {
		s.Beta = cov / (sb * sb)
	}
	if sb > 0 && sr > 0 {
		s.Corr = cov / (sr * sb)
	}
	if !math.IsNaN(s.Beta) {
		s.Alpha = mr - s.Beta*mb
	}
	beat := 0
	stayNum, stayDen := 0, 0
	for i := 0; i < n; i++ {
		win := r[i] > b[i]
		if win {
			beat++
		}
		if i+1 < n && win {
			stayDen++
			if r[i+1] > b[i+1] {
				stayNum++
			}
		}
	}
	s.Presence = float64(beat) / float64(n)
	if stayDen > 0 {
		s.Stay = float64(stayNum) / float64(stayDen)
	}
	return s
}

// Depth is the order-book block in quote currency. A band the snapshot did
// not reach (the side was truncated before that distance) is NaN rather
// than understated.
type Depth struct {
	Mid         float64
	SpreadBps   float64
	Bid01       float64 // bid notional within 0.1 % below mid
	Ask01       float64
	Bid05       float64 // within 0.5 %
	Ask05       float64
	Bid1        float64 // within 1 %
	Ask1        float64
	Imbalance05 float64 // (Bid05 − Ask05) / (Bid05 + Ask05)
	BidReach    float64 // distance of the farthest bid as a share of mid (+Inf if complete)
	AskReach    float64
}

// BookLevel is a price level with size already in BASE coin (sz × ctVal).
type BookLevel struct{ Px, Qty float64 }

// Bands are the thresholds of the depth block.
var Bands = [3]float64{0.001, 0.005, 0.01}

// DepthBands sums notional within ±0.1 / 0.5 / 1 % of mid. bids descend,
// asks ascend. bidsTrunc / asksTrunc say the side was cut by the request
// size, so bands beyond its last level are unknown (NaN).
func DepthBands(bids, asks []BookLevel, bidsTrunc, asksTrunc bool) Depth {
	d := Depth{Mid: NaN, SpreadBps: NaN, Imbalance05: NaN, BidReach: math.Inf(1), AskReach: math.Inf(1)}
	if len(bids) == 0 || len(asks) == 0 || bids[0].Px <= 0 || asks[0].Px <= 0 {
		return d
	}
	d.Mid = (bids[0].Px + asks[0].Px) / 2
	d.SpreadBps = (asks[0].Px - bids[0].Px) / d.Mid * 1e4
	var bid, ask [3]float64
	for _, l := range bids {
		dist := (d.Mid - l.Px) / d.Mid
		if dist > Bands[2] {
			break
		}
		n := l.Px * l.Qty
		for i, th := range Bands {
			if dist <= th {
				bid[i] += n
			}
		}
	}
	for _, l := range asks {
		dist := (l.Px - d.Mid) / d.Mid
		if dist > Bands[2] {
			break
		}
		n := l.Px * l.Qty
		for i, th := range Bands {
			if dist <= th {
				ask[i] += n
			}
		}
	}
	if bidsTrunc {
		d.BidReach = (d.Mid - bids[len(bids)-1].Px) / d.Mid
	}
	if asksTrunc {
		d.AskReach = (asks[len(asks)-1].Px - d.Mid) / d.Mid
	}
	for i, th := range Bands {
		if th > d.BidReach {
			bid[i] = NaN
		}
		if th > d.AskReach {
			ask[i] = NaN
		}
	}
	d.Bid01, d.Bid05, d.Bid1 = bid[0], bid[1], bid[2]
	d.Ask01, d.Ask05, d.Ask1 = ask[0], ask[1], ask[2]
	if tot := d.Bid05 + d.Ask05; tot > 0 {
		d.Imbalance05 = (d.Bid05 - d.Ask05) / tot
	}
	return d
}

// Reach is the smaller of the two sides' reach.
func (d Depth) Reach() float64 { return math.Min(d.BidReach, d.AskReach) }

// FundingAPR annualises a funding rate paid every interval ms.
func FundingAPR(rate float64, intervalMs int64) float64 {
	if intervalMs <= 0 {
		intervalMs = 8 * 3600 * 1000
	}
	perDay := float64(24*3600*1000) / float64(intervalMs)
	return rate * perDay * 365
}

// SMA is the simple moving average of the last n closes including the forming
// bar (bars newest first). NaN with fewer than n bars.
func SMA(bars []Bar, n int) float64 {
	if n <= 0 || len(bars) < n {
		return NaN
	}
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += bars[i].C
	}
	return sum / float64(n)
}

// Distance is last / level − 1, NaN for a missing level.
func Distance(last, level float64) float64 {
	if last <= 0 || level <= 0 || math.IsNaN(level) {
		return NaN
	}
	return last/level - 1
}

// OpenAt returns the open of the daily bar that starts at startMs (UTC day
// boundary), NaN if that day is not in the series. daily newest first.
func OpenAt(daily []Bar, startMs int64) float64 {
	for _, b := range daily {
		if b.TS == startMs {
			return b.O
		}
		if b.TS < startMs {
			break
		}
	}
	return NaN
}

// PeriodStarts returns the UTC millisecond starts of the current day, the
// current ISO week (Monday) and the current month for a moment t.
func PeriodStarts(t time.Time) (day, week, month int64) {
	u := t.UTC()
	d := time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
	wd := int(d.Weekday()) // Sunday = 0
	if wd == 0 {
		wd = 7
	}
	w := d.AddDate(0, 0, -(wd - 1))
	m := time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
	return d.UnixMilli(), w.UnixMilli(), m.UnixMilli()
}
