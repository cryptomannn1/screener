package metrics

import (
	"math"
	"testing"
	"time"
)

func approx(t *testing.T, name string, got, want, eps float64) {
	t.Helper()
	if math.IsNaN(got) || math.Abs(got-want) > eps {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func TestChange(t *testing.T) {
	bars := []Bar{{C: 105}, {C: 100}, {C: 90}}
	approx(t, "1 bar", Change(105, bars, 1), 0.05, 1e-9)
	approx(t, "2 bars", Change(105, bars, 2), 105.0/90-1, 1e-9)
	if !math.IsNaN(Change(105, bars, 3)) {
		t.Fatal("expected NaN without history")
	}
}

func TestATRPct(t *testing.T) {
	// Constant 2-point ranges, no gaps → ATR = 2.
	bars := make([]Bar, 20)
	for i := range bars {
		bars[i] = Bar{O: 100, H: 101, L: 99, C: 100}
	}
	approx(t, "atr", ATRPct(100, bars, 14), 0.02, 1e-9)
}

func TestStrength(t *testing.T) {
	b := make([]float64, 48)
	r := make([]float64, 48)
	for i := range b {
		b[i] = math.Sin(float64(i)) * 0.01
		r[i] = 2*b[i] + 0.001 // perfect beta 2, alpha 0.001
	}
	s := StrengthVs(r, b)
	approx(t, "beta", s.Beta, 2, 1e-9)
	approx(t, "corr", s.Corr, 1, 1e-9)
	approx(t, "alpha", s.Alpha, 0.001, 1e-9)
	// r > b whenever b > −0.001.
	if s.Presence <= 0.4 || s.Presence > 1 {
		t.Fatalf("presence %v", s.Presence)
	}
}

func TestDepthBands(t *testing.T) {
	bids := []BookLevel{{Px: 99.95, Qty: 1}, {Px: 99.6, Qty: 1}, {Px: 99.2, Qty: 1}, {Px: 98, Qty: 1}}
	asks := []BookLevel{{Px: 100.05, Qty: 1}, {Px: 100.4, Qty: 1}, {Px: 100.8, Qty: 1}, {Px: 102, Qty: 1}}
	d := DepthBands(bids, asks, false, false)
	approx(t, "mid", d.Mid, 100, 1e-9)
	approx(t, "spread", d.SpreadBps, 10, 1e-6)
	approx(t, "bid01", d.Bid01, 99.95, 1e-9)
	approx(t, "bid05", d.Bid05, 99.95+99.6, 1e-9)
	approx(t, "bid1", d.Bid1, 99.95+99.6+99.2, 1e-9)
	approx(t, "ask1", d.Ask1, 100.05+100.4+100.8, 1e-9)

	// A bid side cut at 0.4 % below mid leaves the 0.5 % and 1 % bands unknown.
	short := []BookLevel{{Px: 99.95, Qty: 1}, {Px: 99.6, Qty: 1}}
	d = DepthBands(short, asks, true, false)
	approx(t, "bid01 trunc", d.Bid01, 99.95, 1e-9)
	if !math.IsNaN(d.Bid05) || !math.IsNaN(d.Bid1) || !math.IsNaN(d.Imbalance05) {
		t.Fatalf("truncated bands must be NaN: %+v", d)
	}
	approx(t, "reach", d.BidReach, 0.004, 1e-9)
}

func TestVolRegime(t *testing.T) {
	daily := make([]Bar, 25)
	for i := range daily {
		daily[i] = Bar{H: 102, L: 98, C: 100}
	}
	hourly := make([]Bar, 30)
	for i := range hourly {
		hourly[i] = Bar{H: 100.5, L: 99.5, C: 100}
	}
	hourly[3].H, hourly[17].L = 104, 96 // the last 24 hours span twice a usual day
	approx(t, "regime", VolRegime(hourly, daily, 20), 2, 1e-9)
	// The forming day does not enter: right after midnight it has no range yet, the last 24 hours still do.
	daily[0] = Bar{H: 100, L: 100, C: 100}
	approx(t, "regime at the start of a day", VolRegime(hourly, daily, 20), 2, 1e-9)
	// A gap against the close 24 hours ago counts, as in a true range.
	calm := make([]Bar, 30)
	for i := range calm {
		calm[i] = Bar{H: 100.5, L: 99.5, C: 100}
	}
	calm[24].C = 92
	approx(t, "regime with a gap", VolRegime(calm, daily, 20), (100.5-92)/100/0.04, 1e-9)
	if !math.IsNaN(VolRegime(hourly[:24], daily, 20)) {
		t.Fatal("expected NaN without 24 hours of hourly bars and the close before them")
	}
}

// barsOf builds n bars of length step, newest first; the forming one began at
// t0. Closes come from px(i) and a bar opens where the one before it closed.
func barsOf(step, t0 int64, n int, px func(i int) float64, vol float64) []Bar {
	bars := make([]Bar, n)
	for i := range bars {
		o, c := px(i+1), px(i)
		bars[i] = Bar{TS: t0 - int64(i)*step, O: o, C: c, H: math.Max(o, c), L: math.Min(o, c), QuoteVol: vol}
	}
	return bars
}

func hourBars(t0 int64, n int, px func(i int) float64, vol float64) []Bar {
	return barsOf(HourMs, t0, n, px, vol)
}

func flat(int) float64 { return 100 }

func TestVolumeIn(t *testing.T) {
	t0 := 1000 * HourMs
	bars := barsOf(MinuteMs, t0, 120, flat, 10) // 10 a minute
	bars[0].QuoteVol = 5                        // the forming minute is half through
	to := t0 + MinuteMs/2
	// The forming bar whole, 59 complete ones and half of the bar the window begins in.
	approx(t, "an hour ending mid-minute", VolumeIn(bars, MinuteMs, to-HourMs, to), 5+59*10+5, 1e-9)
	approx(t, "an hour ending on the minute", VolumeIn(bars, MinuteMs, t0-HourMs, t0), 600, 1e-9)
	approx(t, "inside the forming bar", VolumeIn(bars, MinuteMs, to-MinuteMs/4, to), 2.5, 1e-9)
	// The newest bar has not appeared yet: the window ends where the data ends.
	approx(t, "newest bar missing", VolumeIn(bars[1:], MinuteMs, to-HourMs, to), 59*10+5, 1e-9)
	gap := append(append([]Bar(nil), bars[:20]...), bars[21:]...)
	for name, v := range map[string]float64{
		"stale bars":         VolumeIn(bars[3:], MinuteMs, to-HourMs, to),
		"not enough history": VolumeIn(bars[:30], MinuteMs, to-HourMs, to),
		"a gap":              VolumeIn(gap, MinuteMs, to-HourMs, to),
		"no bars":            VolumeIn(nil, MinuteMs, to-HourMs, to),
	} {
		if !math.IsNaN(v) {
			t.Fatalf("%s: got %v, expected NaN", name, v)
		}
	}
}

func TestChangeHours(t *testing.T) {
	t0 := 1000 * HourMs
	tm := t0 + 17*MinuteMs // the hour is 17 minutes old
	now := time.UnixMilli(tm).Add(30 * time.Second)
	hourly := hourBars(t0, 200, flat, 60)
	minute := barsOf(MinuteMs, tm, 300, func(i int) float64 { return 100 + 0.1*float64(i) }, 1) // 0.1 lower every minute
	// An hour ago is half way through minute bar 60: it opened at 106.1 and closed at 106.0.
	approx(t, "1h off the minute bars", ChangeHours(100, minute, hourly, now, 1), 100/106.05-1, 1e-9)
	approx(t, "4h off the minute bars", ChangeHours(100, minute, hourly, now, 4), 100/124.05-1, 1e-9)
	// The minute bars do not reach that far: the hourly bars stand in (flat here).
	approx(t, "1h off the hourly bars", ChangeHours(100, minute[:30], hourly, now, 1), 0, 1e-9)
	approx(t, "24h off the hourly bars", ChangeHours(100, minute, hourly, now, 24), 0, 1e-9)
	if !math.IsNaN(ChangeHours(100, nil, hourly[:1], now, 1)) {
		t.Fatal("expected NaN without history")
	}
}

func TestVolVsHours(t *testing.T) {
	t0 := 1000 * HourMs
	tm := t0 + 17*MinuteMs
	hourly := hourBars(t0, 200, flat, 60) // the usual hour trades 60
	for _, sec := range []int{0, 1, 20, 59} {
		minute := barsOf(MinuteMs, tm, 300, flat, 1)
		minute[0].QuoteVol = float64(sec) / 60
		hourly[0].QuoteVol = 17 + float64(sec)/60 // 17 minutes and sec seconds of the hour have traded
		asOf := time.UnixMilli(tm).Add(time.Duration(sec) * time.Second)
		approx(t, "1h, even market", VolVsHours(minute, asOf, hourly, asOf, 1, 168), 1, 1e-9)
		approx(t, "4h, even market", VolVsHours(minute, asOf, hourly, asOf, 4, 168), 1, 1e-9)
		// A day does not fit into five hours of minute bars: the hourly bars answer.
		approx(t, "24h, even market", VolVsHours(minute, asOf, hourly, asOf, 24, 168), 1, 1e-9)
	}
	// Ten busy minutes, five times the usual, show at once and exactly.
	minute := barsOf(MinuteMs, tm, 300, flat, 1)
	for i := 1; i <= 10; i++ {
		minute[i].QuoteVol = 5
	}
	minute[0].QuoteVol = 0
	asOf := time.UnixMilli(tm)
	approx(t, "burst", VolVsHours(minute, asOf, hourly, asOf, 1, 168), (50+50.0)/60, 1e-9)
	// Without minute bars, or before they were ever read, the hourly estimate stands in.
	want := VolVsPeriod(hourly, 1, 168, asOf)
	approx(t, "no minute bars", VolVsHours(nil, asOf, hourly, asOf, 1, 168), want, 1e-9)
	approx(t, "minute bars never read", VolVsHours(minute, time.Time{}, hourly, asOf, 1, 168), want, 1e-9)
	approx(t, "minute bars too short", VolVsHours(minute[:30], asOf, hourly, asOf, 1, 168), want, 1e-9)
}

func TestPriceAt(t *testing.T) {
	t0 := 1000 * HourMs
	bars := hourBars(t0, 5, func(i int) float64 { return 100 - 10*float64(i) }, 1) // closes 100, 90, 80, 70, 60
	approx(t, "first moment of the previous bar: its open", PriceAt(bars, HourMs, t0-HourMs), 80, 1e-9)
	approx(t, "half way through it", PriceAt(bars, HourMs, t0-HourMs/2), 85, 1e-9)
	approx(t, "its last millisecond: nearly the close", PriceAt(bars, HourMs, t0-1), 90, 1e-5)
	approx(t, "inside the forming bar", PriceAt(bars, HourMs, t0+HourMs/4), 92.5, 1e-9)
	approx(t, "the oldest bar", PriceAt(bars, HourMs, t0-4*HourMs), 50, 1e-9)
	if !math.IsNaN(PriceAt(bars, HourMs, t0-4*HourMs-1)) || !math.IsNaN(PriceAt(bars, HourMs, t0+HourMs)) || !math.IsNaN(PriceAt(nil, HourMs, t0)) {
		t.Fatal("expected NaN outside the bars")
	}
	// A missing bar: the others are still found by their time, the hole itself has no price.
	gap := append(append([]Bar(nil), bars[:2]...), bars[3:]...)
	approx(t, "past a gap", PriceAt(gap, HourMs, t0-3*HourMs+HourMs/2), 65, 1e-9)
	if !math.IsNaN(PriceAt(gap, HourMs, t0-2*HourMs+HourMs/2)) {
		t.Fatal("expected NaN inside a gap")
	}
}

func TestChangeOver(t *testing.T) {
	t0 := 1000 * HourMs
	// closes, newest first: 110 (forming, opened at 100), 100 (opened at 80), 80, 80
	px := []float64{110, 100, 80, 80, 80, 80}
	bars := hourBars(t0, 4, func(i int) float64 { return px[i] }, 1)
	at := func(min int) time.Time { return time.UnixMilli(t0).Add(time.Duration(min) * time.Minute) }
	// One minute into the hour. An hour ago the price was one minute into the previous bar: 80 + 20/60.
	approx(t, "1h, a minute after the hour began", ChangeOver(110, bars, HourMs, at(1), time.Hour), 110/(80+20.0/60)-1, 1e-9)
	// The same moment measured from the hourly close covers only that one minute.
	approx(t, "from the hourly close", Change(110, bars, 1), 0.10, 1e-9)
	approx(t, "1h at half past", ChangeOver(110, bars, HourMs, at(30), time.Hour), 110.0/90-1, 1e-9)
	approx(t, "2h at half past", ChangeOver(110, bars, HourMs, at(30), 2*time.Hour), 110.0/80-1, 1e-9)
	if !math.IsNaN(ChangeOver(110, bars, HourMs, at(30), 10*time.Hour)) || !math.IsNaN(ChangeOver(0, bars, HourMs, at(30), time.Hour)) {
		t.Fatal("expected NaN without history or without a price")
	}
}

func TestChangeDays(t *testing.T) {
	t0 := 1000 * HourMs
	now := time.UnixMilli(t0).Add(30 * time.Minute)
	hourly := hourBars(t0, 200, func(i int) float64 { return 100 + float64(i) }, 1) // 1 lower every hour going forward
	daily := []Bar{{C: 100}, {C: 300}, {C: 300}, {C: 300}, {C: 300}, {C: 300}, {C: 300}, {C: 250}}
	// 7 days back is half way through hourly bar 168: (268 + 269) / 2.
	approx(t, "7 days off the hourly bars", ChangeDays(100, hourly, daily, now, 7), 100/268.5-1, 1e-9)
	// The hourly bars do not reach 7 days back: the daily close 7 days ago stands in.
	approx(t, "7 days off the daily close", ChangeDays(100, hourly[:100], daily, now, 7), 100.0/250-1, 1e-9)
	if !math.IsNaN(ChangeDays(100, hourly[:100], daily, now, 30)) {
		t.Fatal("expected NaN without history")
	}
}

func TestVolVsPeriodRolling(t *testing.T) {
	t0 := 1000 * HourMs
	// A market that trades evenly, 60 an hour. Whatever the minute, the last hour is exactly one norm.
	for _, min := range []int{0, 1, 15, 30, 59} {
		bars := hourBars(t0, 200, func(int) float64 { return 100 }, 60)
		bars[0].QuoteVol = float64(min) // the forming bar has traded for min minutes
		asOf := time.UnixMilli(t0).Add(time.Duration(min) * time.Minute)
		approx(t, "1h, even market", VolVsPeriod(bars, 1, 168, asOf), 1, 1e-9)
		approx(t, "4h, even market", VolVsPeriod(bars, 4, 168, asOf), 1, 1e-9)
		approx(t, "24h, even market", VolVsPeriod(bars, 24, 168, asOf), 1, 1e-9)
		// Without the moment of reading the forming bar counts as a whole hour: the old, shrinking figure.
		approx(t, "1h, forming bar as whole", VolVsPeriod(bars, 1, 168, time.Time{}), float64(min)/60, 1e-9)
	}
	// A burst shows at once: 120 traded in the first 10 minutes on top of 50 minutes of the usual hour.
	bars := hourBars(t0, 200, func(int) float64 { return 100 }, 60)
	bars[0].QuoteVol = 120
	approx(t, "burst", VolVsPeriod(bars, 1, 168, time.UnixMilli(t0).Add(10*time.Minute)), (120+50.0)/60, 1e-9)
	// The norm comes from the complete bars before the window, not from the window itself.
	bars[1].QuoteVol = 6000
	approx(t, "norm excludes the window", VolVsPeriod(bars, 1, 168, time.UnixMilli(t0).Add(10*time.Minute)), (120+6000*50.0/60)/60, 1e-9)
	if !math.IsNaN(VolVsPeriod(bars[:2], 1, 168, time.Time{})) || !math.IsNaN(VolVsPeriod(bars[:8], 4, 168, time.Time{})) {
		t.Fatal("expected NaN without bars for the norm")
	}
}

func TestFundingAPR(t *testing.T) {
	approx(t, "8h", FundingAPR(0.0001, 8*3600*1000), 0.0001*3*365, 1e-12)
}

func TestSMAAndOpens(t *testing.T) {
	bars := []Bar{{C: 10}, {C: 20}, {C: 30}, {C: 40}}
	approx(t, "sma3", SMA(bars, 3), 20, 1e-12)
	if !math.IsNaN(SMA(bars, 5)) {
		t.Fatal("sma without history must be NaN")
	}
	approx(t, "dist", Distance(110, 100), 0.1, 1e-12)

	// Wednesday 2026-10-28 15:00 UTC → day 28th, week Monday 26th, month 1st.
	at := time.Date(2026, 10, 28, 15, 0, 0, 0, time.UTC)
	d, w, m := PeriodStarts(at)
	if d != time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC).UnixMilli() ||
		w != time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC).UnixMilli() ||
		m != time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).UnixMilli() {
		t.Fatalf("period starts %d %d %d", d, w, m)
	}
	daily := []Bar{{TS: d, O: 3}, {TS: d - 86_400_000, O: 2}, {TS: w, O: 1}}
	approx(t, "open day", OpenAt(daily, d), 3, 0)
	approx(t, "open week", OpenAt(daily, w), 1, 0)
	if !math.IsNaN(OpenAt(daily, m)) {
		t.Fatal("missing month open must be NaN")
	}
	// Sunday belongs to the week that started the previous Monday.
	_, w2, _ := PeriodStarts(time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC))
	if w2 != time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC).UnixMilli() {
		t.Fatalf("sunday week start %d", w2)
	}
}
