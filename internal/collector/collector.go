// Package collector polls the OKX public API for the whole linear-swap
// universe, keeps the latest state per instrument and computes the screener
// rows. Batch endpoints (tickers, mark, index, OI, funding) cover every
// instrument in one call; per-instrument endpoints (candles, books) are swept
// continuously under the client's rate limiters.
package collector

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

	"screener/internal/metrics"
	"screener/internal/okx"
)

// Config tunes the poll cadence.
type Config struct {
	FastEvery       time.Duration // tickers + mark + index, default 5s
	OIEvery         time.Duration // open interest, default 15s
	FundingEvery    time.Duration // funding rates, default 60s
	UniverseEvery   time.Duration // instruments, default 10m
	HourlyEvery     time.Duration // refresh pass of 1H candles, default 2m
	DailyEvery      time.Duration // refresh pass of 1Dutc candles, default 15m
	BooksEvery      time.Duration // minimum period of one books sweep, default 30s
	HourlyBackfill  int           // bars on first load, default 300
	DailyBackfill   int           // bars on first load, default 40
	OIKeep          time.Duration // OI history window, default 26h
	Settle          []string      // settle currencies, default USDT, USDC
	BenchmarkInstID string        // default BTC-USDT-SWAP
	BooksTop        int           // sweep books only for the top N by vol24; 0 = all
}

func (c Config) withDefaults() Config {
	def := func(d *time.Duration, v time.Duration) {
		if *d <= 0 {
			*d = v
		}
	}
	def(&c.FastEvery, 5*time.Second)
	def(&c.OIEvery, 15*time.Second)
	def(&c.FundingEvery, 60*time.Second)
	def(&c.UniverseEvery, 10*time.Minute)
	def(&c.HourlyEvery, 2*time.Minute)
	def(&c.DailyEvery, 15*time.Minute)
	def(&c.BooksEvery, 30*time.Second)
	def(&c.OIKeep, 26*time.Hour)
	if c.HourlyBackfill <= 0 {
		c.HourlyBackfill = 300
	}
	if c.DailyBackfill <= 0 {
		c.DailyBackfill = 40
	}
	if len(c.Settle) == 0 {
		c.Settle = []string{"USDT", "USDC"}
	}
	if c.BenchmarkInstID == "" {
		c.BenchmarkInstID = "BTC-USDT-SWAP"
	}
	return c
}

// Sink receives raw observations for persistence. Calls come from the poll
// goroutines; implementations must be quick (queue, don't block).
type Sink interface {
	OnTick(ts time.Time, rows []Tick)
	OnCandles(instID, bar string, candles []okx.Candle)
	OnDepth(ts time.Time, instID string, d metrics.Depth)
	OnRows(ts time.Time, rows []Row)
}

// Tick is the per-minute snapshot of the batch endpoints for one instrument.
type Tick struct {
	InstID      string
	Last        float64
	Mark        float64
	Index       float64
	Bid, Ask    float64
	VolCcy24h   float64
	VolQuote24h float64
	OICcy       float64
	OIUsd       float64
	Funding     float64
	NextFunding float64
}

type oiPoint struct {
	ts  int64 // ms
	usd float64
}

type state struct {
	inst              okx.Instrument
	tk                okx.Ticker
	mark              float64
	index             float64
	oi                okx.OpenInterest
	fund              okx.Funding
	hourly            []metrics.Bar // newest first
	daily             []metrics.Bar // newest first
	hourlyAt, dailyAt time.Time
	oiHist            []oiPoint // oldest first
	depth             metrics.Depth
	depthAt           time.Time
	hourlyRet         []float64 // cached log returns, oldest first
}

// Collector is the service state.
type Collector struct {
	cli   *okx.Client
	cfg   Config
	log   *slog.Logger
	sinks []Sink

	mu             sync.RWMutex
	states         map[string]*state
	rows           []Row
	rowsAt         time.Time
	health         Health
	started        time.Time
	lastRowsStored time.Time
	lastTickStored time.Time
}

// Health is the /health payload.
type Health struct {
	Universe  int             `json:"universe"`
	Rows      int             `json:"rows"`
	RowsAt    int64           `json:"rowsAt"`
	UptimeSec int64           `json:"uptimeSec"`
	Loops     map[string]Loop `json:"loops"`
	Coverage  map[string]int  `json:"coverage"`
	Benchmark string          `json:"benchmark"`
}

// Loop is the state of one poll loop.
type Loop struct {
	LastOK    int64  `json:"lastOk"`
	LastErr   string `json:"lastErr,omitempty"`
	LastErrAt int64  `json:"lastErrAt,omitempty"`
	Passes    uint64 `json:"passes"`
	Errors    uint64 `json:"errors"`
	Items     int    `json:"items"`
}

// New builds a collector.
func New(cli *okx.Client, cfg Config, log *slog.Logger, sinks ...Sink) *Collector {
	return &Collector{
		cli: cli, cfg: cfg.withDefaults(), log: log, sinks: sinks,
		states:  map[string]*state{},
		health:  Health{Loops: map[string]Loop{}, Coverage: map[string]int{}},
		started: time.Now(),
	}
}

// SeedOI preloads OI history (e.g. from the store) before Run.
func (c *Collector) SeedOI(instID string, ts int64, usd float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.states[instID]
	if st == nil {
		st = &state{inst: okx.Instrument{ID: instID}}
		c.states[instID] = st
	}
	st.oiHist = append(st.oiHist, oiPoint{ts: ts, usd: usd})
}

// Run starts every loop and blocks until ctx ends.
func (c *Collector) Run(ctx context.Context) error {
	// The universe must exist before anything else can be keyed.
	for {
		if err := c.refreshUniverse(ctx); err == nil {
			break
		} else {
			c.log.Warn("universe", "err", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
			}
		}
	}
	var wg sync.WaitGroup
	run := func(name string, every time.Duration, fn func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.loop(ctx, name, every, fn)
		}()
	}
	run("fast", c.cfg.FastEvery, c.pollFast)
	run("oi", c.cfg.OIEvery, c.pollOI)
	run("funding", c.cfg.FundingEvery, c.pollFunding)
	run("universe", c.cfg.UniverseEvery, c.refreshUniverse)
	run("hourly", c.cfg.HourlyEvery, func(ctx context.Context) error { return c.sweepCandles(ctx, "1H") })
	run("daily", c.cfg.DailyEvery, func(ctx context.Context) error { return c.sweepCandles(ctx, "1Dutc") })
	run("books", c.cfg.BooksEvery, c.sweepBooks)
	wg.Wait()
	return ctx.Err()
}

func (c *Collector) loop(ctx context.Context, name string, every time.Duration, fn func(context.Context) error) {
	if name == "universe" {
		// Already refreshed once in Run.
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
	}
	for {
		start := time.Now()
		err := fn(ctx)
		c.mu.Lock()
		l := c.health.Loops[name]
		if err != nil {
			l.Errors++
			l.LastErr = err.Error()
			l.LastErrAt = time.Now().UnixMilli()
			if ctx.Err() == nil {
				c.log.Warn("loop", "name", name, "err", err)
			}
		} else {
			l.Passes++
			l.LastOK = time.Now().UnixMilli()
		}
		c.health.Loops[name] = l
		c.mu.Unlock()
		if ctx.Err() != nil {
			return
		}
		wait := every - time.Since(start)
		if wait < time.Second {
			wait = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func (c *Collector) setItems(name string, n int) {
	c.mu.Lock()
	l := c.health.Loops[name]
	l.Items = n
	c.health.Loops[name] = l
	c.mu.Unlock()
}

// refreshUniverse loads SWAP instruments and keeps the linear ones settled in
// the configured currencies and in state live.
func (c *Collector) refreshUniverse(ctx context.Context) error {
	insts, err := c.cli.Instruments(ctx, "SWAP")
	if err != nil {
		return err
	}
	settle := map[string]bool{}
	for _, s := range c.cfg.Settle {
		settle[s] = true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	seen := map[string]bool{}
	for _, in := range insts {
		if in.CtType != "linear" || in.State != "live" || !settle[in.SettleCcy] {
			continue
		}
		seen[in.ID] = true
		st := c.states[in.ID]
		if st == nil {
			st = &state{}
			c.states[in.ID] = st
		}
		st.inst = in
	}
	for id, st := range c.states {
		if !seen[id] && st.inst.CtType != "" {
			delete(c.states, id) // delisted or suspended
		}
	}
	c.health.Universe = len(seen)
	c.health.Loops["universe"] = Loop{Items: len(seen), Passes: c.health.Loops["universe"].Passes, LastOK: time.Now().UnixMilli()}
	return nil
}

// pollFast fetches tickers, mark and index prices and recomputes the rows.
func (c *Collector) pollFast(ctx context.Context) error {
	tks, err := c.cli.Tickers(ctx, "SWAP")
	if err != nil {
		return err
	}
	marks, err := c.cli.MarkPrices(ctx, "SWAP")
	if err != nil {
		return err
	}
	idx := map[string]float64{}
	for _, q := range c.cfg.Settle {
		its, err := c.cli.IndexTickers(ctx, q)
		if err != nil {
			return err
		}
		for _, it := range its {
			idx[it.InstID] = it.IdxPx
		}
	}
	now := time.Now()
	c.mu.Lock()
	n := 0
	for _, tk := range tks {
		if st := c.states[tk.InstID]; st != nil {
			st.tk = tk
			n++
		}
	}
	for _, m := range marks {
		if st := c.states[m.InstID]; st != nil {
			st.mark = m.MarkPx
		}
	}
	for id, st := range c.states {
		if st.inst.Uly != "" {
			if px, ok := idx[st.inst.Uly]; ok {
				st.index = px
			}
		}
		_ = id
	}
	c.mu.Unlock()
	c.setItems("fast", n)
	c.compute(now)
	return nil
}

// pollOI fetches open interest for the universe and appends to the history.
func (c *Collector) pollOI(ctx context.Context) error {
	ois, err := c.cli.OpenInterest(ctx, "SWAP")
	if err != nil {
		return err
	}
	now := time.Now()
	cut := now.Add(-c.cfg.OIKeep).UnixMilli()
	c.mu.Lock()
	n := 0
	for _, oi := range ois {
		st := c.states[oi.InstID]
		if st == nil {
			continue
		}
		st.oi = oi
		usd := oi.OIUsd
		if usd <= 0 && st.tk.Last > 0 {
			usd = oi.OICcy * st.tk.Last
		}
		// One point per minute is enough for hourly deltas.
		if l := len(st.oiHist); l == 0 || now.UnixMilli()-st.oiHist[l-1].ts >= 60_000 {
			st.oiHist = append(st.oiHist, oiPoint{ts: now.UnixMilli(), usd: usd})
		}
		for len(st.oiHist) > 0 && st.oiHist[0].ts < cut {
			st.oiHist = st.oiHist[1:]
		}
		n++
	}
	c.mu.Unlock()
	c.setItems("oi", n)
	return nil
}

func (c *Collector) pollFunding(ctx context.Context) error {
	fs, err := c.cli.FundingRates(ctx)
	if err != nil {
		return err
	}
	c.mu.Lock()
	n := 0
	for _, fr := range fs {
		if st := c.states[fr.InstID]; st != nil {
			st.fund = fr
			n++
		}
	}
	c.mu.Unlock()
	c.setItems("funding", n)
	return nil
}

// universeIDs returns the instrument ids sorted by 24h quote volume, the
// benchmark first so its series is always the freshest.
func (c *Collector) universeIDs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	type kv struct {
		id  string
		vol float64
	}
	kvs := make([]kv, 0, len(c.states))
	for id, st := range c.states {
		if st.inst.CtType == "" {
			continue
		}
		kvs = append(kvs, kv{id, st.tk.VolCcy24h * st.tk.Last})
	}
	sort.Slice(kvs, func(i, j int) bool {
		if kvs[i].id == c.cfg.BenchmarkInstID {
			return true
		}
		if kvs[j].id == c.cfg.BenchmarkInstID {
			return false
		}
		return kvs[i].vol > kvs[j].vol
	})
	out := make([]string, len(kvs))
	for i, k := range kvs {
		out[i] = k.id
	}
	return out
}

// sweepWorkers is how many per-instrument requests run at once inside one
// sweep; the client's limiters still cap the rate, workers only hide latency.
const sweepWorkers = 4

// sweep runs fn over ids with a few workers and returns the first error and
// the number of successes.
func sweep(ctx context.Context, ids []string, fn func(context.Context, string) error) (int, error) {
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
		n        int
	)
	jobs := make(chan string)
	for w := 0; w < sweepWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				err := fn(ctx, id)
				mu.Lock()
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
				} else {
					n++
				}
				mu.Unlock()
			}
		}()
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		jobs <- id
	}
	close(jobs)
	wg.Wait()
	if ctx.Err() != nil {
		return n, ctx.Err()
	}
	return n, firstErr
}

// sweepCandles refreshes one bar size for every instrument: a backfill on the
// first visit, then the last 3 bars (forming + 2 closed, so a bar closed while
// we were away is still caught).
func (c *Collector) sweepCandles(ctx context.Context, bar string) error {
	ids := c.universeIDs()
	n, err := sweep(ctx, ids, func(ctx context.Context, id string) error {
		c.mu.RLock()
		st := c.states[id]
		var have int
		if st != nil {
			if bar == "1H" {
				have = len(st.hourly)
			} else {
				have = len(st.daily)
			}
		}
		c.mu.RUnlock()
		if st == nil {
			return nil
		}
		limit := 3
		if have == 0 {
			if bar == "1H" {
				limit = c.cfg.HourlyBackfill
			} else {
				limit = c.cfg.DailyBackfill
			}
		}
		cs, err := c.cli.Candles(ctx, id, bar, limit)
		if err != nil {
			return err
		}
		c.mergeCandles(id, bar, cs)
		for _, s := range c.sinks {
			s.OnCandles(id, bar, cs)
		}
		return nil
	})
	name := "hourly"
	if bar != "1H" {
		name = "daily"
	}
	return c.sweepResult(name, n, len(ids), err)
}

// sweepResult records a sweep: a pass that covered some instruments counts as
// a pass and keeps the error as a note; nothing covered is a failed pass.
func (c *Collector) sweepResult(name string, n, total int, err error) error {
	c.mu.Lock()
	l := c.health.Loops[name]
	l.Items = n
	if err != nil && n > 0 {
		l.LastErr = fmt.Sprintf("%d/%d ok, last: %v", n, total, err)
		l.LastErrAt = time.Now().UnixMilli()
		err = nil
	}
	c.health.Loops[name] = l
	c.mu.Unlock()
	return err
}

// mergeCandles folds newest-first candles into the state series.
func (c *Collector) mergeCandles(id, bar string, cs []okx.Candle) {
	if len(cs) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.states[id]
	if st == nil {
		return
	}
	series := &st.hourly
	max := c.cfg.HourlyBackfill
	if bar != "1H" {
		series = &st.daily
		max = c.cfg.DailyBackfill
	}
	byTS := map[int64]metrics.Bar{}
	for _, b := range *series {
		byTS[b.TS] = b
	}
	for _, k := range cs {
		byTS[k.TS] = metrics.Bar{TS: k.TS, O: k.Open, H: k.High, L: k.Low, C: k.Close, QuoteVol: k.VolCcyQuote}
	}
	out := make([]metrics.Bar, 0, len(byTS))
	for _, b := range byTS {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TS > out[j].TS })
	if len(out) > max {
		out = out[:max]
	}
	*series = out
	if bar == "1H" {
		st.hourlyAt = time.Now()
		st.hourlyRet = metrics.LogReturns(out, 168)
	} else {
		st.dailyAt = time.Now()
	}
}

// sweepBooks fetches a 400-level book for every instrument (or the top N) and
// stores the depth bands. When 400 levels stop short of ±1 % of the mid (BTC
// at a 0.1 tick covers ±0.05 %), the 5000-level books-full endpoint is used
// instead; bands still beyond the snapshot read as NaN.
func (c *Collector) sweepBooks(ctx context.Context) error {
	ids := c.universeIDs()
	if c.cfg.BooksTop > 0 && len(ids) > c.cfg.BooksTop {
		ids = ids[:c.cfg.BooksTop]
	}
	n, err := sweep(ctx, ids, func(ctx context.Context, id string) error {
		b, err := c.cli.Books(ctx, id, 400)
		if err != nil {
			return err
		}
		d, ok := c.depthOf(id, b)
		if !ok {
			return nil
		}
		if d.Reach() < metrics.Bands[2] {
			if fb, err := c.cli.BooksFull(ctx, id, 5000); err == nil {
				if fd, ok := c.depthOf(id, fb); ok {
					d = fd
				}
			}
		}
		c.mu.Lock()
		if st := c.states[id]; st != nil {
			st.depth = d
			st.depthAt = time.Now()
		}
		c.mu.Unlock()
		for _, s := range c.sinks {
			s.OnDepth(time.Now(), id, d)
		}
		return nil
	})
	return c.sweepResult("books", n, len(ids), err)
}

// depthOf converts a book to depth bands using the instrument's contract value.
func (c *Collector) depthOf(id string, b okx.Book) (metrics.Depth, bool) {
	c.mu.RLock()
	st := c.states[id]
	var ctVal float64
	if st != nil {
		ctVal = st.inst.CtVal
	}
	c.mu.RUnlock()
	if st == nil {
		return metrics.Depth{}, false
	}
	if ctVal <= 0 {
		ctVal = 1
	}
	bids := make([]metrics.BookLevel, len(b.Bids))
	for i, l := range b.Bids {
		bids[i] = metrics.BookLevel{Px: l.Px, Qty: l.Sz * ctVal}
	}
	asks := make([]metrics.BookLevel, len(b.Asks))
	for i, l := range b.Asks {
		asks[i] = metrics.BookLevel{Px: l.Px, Qty: l.Sz * ctVal}
	}
	bt, at := b.Truncated()
	return metrics.DepthBands(bids, asks, bt, at), true
}

// compute rebuilds every row from the current state.
func (c *Collector) compute(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	bench := c.states[c.cfg.BenchmarkInstID]
	var bRet []float64
	var bChg = map[string]float64{}
	if bench != nil {
		bRet = bench.hourlyRet
		for _, p := range Periods {
			bChg[p] = metrics.Change(bench.tk.Last, bench.hourly, PeriodHours[p])
		}
	}
	rows := make([]Row, 0, len(c.states))
	cov := map[string]int{"hourly": 0, "daily": 0, "book": 0, "oiHist": 0}
	ticks := make([]Tick, 0, len(c.states))
	for _, st := range c.states {
		if st.inst.CtType == "" || st.tk.Last <= 0 {
			continue
		}
		r := c.buildRow(st, now, bRet, bChg)
		if r.Cov.Hourly {
			cov["hourly"]++
		}
		if r.Cov.Daily {
			cov["daily"]++
		}
		if r.Cov.Book {
			cov["book"]++
		}
		if r.Cov.OIHist {
			cov["oiHist"]++
		}
		rows = append(rows, r)
		ticks = append(ticks, Tick{
			InstID: st.inst.ID, Last: st.tk.Last, Mark: st.mark, Index: st.index,
			Bid: st.tk.BidPx, Ask: st.tk.AskPx, VolCcy24h: st.tk.VolCcy24h,
			VolQuote24h: st.tk.VolCcy24h * st.tk.Last, OICcy: st.oi.OICcy, OIUsd: float64(r.OI),
			Funding: st.fund.Rate, NextFunding: st.fund.NextRate,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Vol24 > rows[j].Vol24 })
	c.rows = rows
	c.rowsAt = now
	c.health.Rows = len(rows)
	c.health.RowsAt = now.UnixMilli()
	c.health.Coverage = cov
	c.health.Benchmark = c.cfg.BenchmarkInstID
	// Persist once a minute: rows for analytics, ticks for OI / price history.
	if now.Sub(c.lastRowsStored) >= time.Minute {
		c.lastRowsStored = now
		c.lastTickStored = now
		for _, s := range c.sinks {
			s.OnTick(now, ticks)
			s.OnRows(now, rows)
		}
	}
}

func (c *Collector) buildRow(st *state, now time.Time, bRet []float64, bChg map[string]float64) Row {
	last := st.tk.Last
	r := Row{
		InstID: st.inst.ID, Symbol: st.inst.Base, Settle: st.inst.SettleCcy,
		Lever: F(st.inst.Lever), Last: F(last), Mark: F(st.mark), Index: F(st.index),
		Bid: F(st.tk.BidPx), Ask: F(st.tk.AskPx), Updated: st.tk.TS,
		Chg: PerPeriod{}, VolX: PerPeriod{}, OID: PerPeriod{}, VsBtc: PerPeriod{},
		TakerBuy: PerPeriod{}, Delta: PerPeriod{},
	}
	if st.inst.ListTime > 0 {
		r.Listed = F(float64(now.UnixMilli()-st.inst.ListTime) / 86_400_000)
	} else {
		r.Listed = F(math.NaN())
	}
	r.Basis = F(math.NaN())
	if st.mark > 0 && st.index > 0 {
		r.Basis = F(st.mark/st.index - 1)
	}
	vol24 := st.tk.VolCcy24h * last
	r.Vol24 = F(vol24)

	// OI
	oiUsd := st.oi.OIUsd
	if oiUsd <= 0 && st.oi.OICcy > 0 {
		oiUsd = st.oi.OICcy * last
	}
	r.OI = F(oiUsd)
	r.OIVol = F(math.NaN())
	if vol24 > 0 && oiUsd > 0 {
		r.OIVol = F(oiUsd / vol24)
	}
	for _, p := range Periods {
		r.OID[p] = F(oiChange(st.oiHist, oiUsd, now.UnixMilli(), int64(PeriodHours[p])*3_600_000))
	}
	if n := len(st.oiHist); n > 0 && now.UnixMilli()-st.oiHist[0].ts >= 3_600_000 {
		r.Cov.OIHist = true
	}

	// Funding
	r.Funding = F(st.fund.Rate)
	r.FundingNext = F(st.fund.NextRate)
	interval := st.fund.NextFundingTime - st.fund.FundingTime
	if interval <= 0 {
		interval = 8 * 3_600_000
	}
	r.FundingInterval = F(float64(interval) / 3_600_000)
	r.FundingAnn = F(metrics.FundingAPR(st.fund.Rate, interval))
	if st.fund.TS == 0 {
		r.Funding, r.FundingNext, r.FundingAnn = F(math.NaN()), F(math.NaN()), F(math.NaN())
	}

	// Hourly block
	r.Cov.Hourly = len(st.hourly) >= 26
	for _, p := range Periods {
		h := PeriodHours[p]
		chg := metrics.Change(last, st.hourly, h)
		r.Chg[p] = F(chg)
		r.VolX[p] = F(metrics.VolVsPeriod(st.hourly, h, 168))
		if b, ok := bChg[p]; ok && !math.IsNaN(chg) && !math.IsNaN(b) {
			r.VsBtc[p] = F(chg - b)
		} else {
			r.VsBtc[p] = F(math.NaN())
		}
	}
	r.ATR = F(metrics.ATRPct(last, st.hourly, 14))
	r.RV24 = F(metrics.RealizedVol(metrics.LogReturns(st.hourly, 24), 24*365))
	s := metrics.StrengthVs(st.hourlyRet, bRet)
	if st.inst.ID == c.cfg.BenchmarkInstID {
		s = metrics.Strength{Beta: 1, Corr: 1, Alpha: 0, Presence: math.NaN(), Stay: math.NaN()}
	}
	r.Beta, r.Corr, r.Alpha, r.Presence, r.Stay = F(s.Beta), F(s.Corr), F(s.Alpha), F(s.Presence), F(s.Stay)

	// Daily block
	r.Cov.Daily = len(st.daily) >= 8
	r.Chg1d = F(metrics.Change(last, st.daily, 1))
	r.Chg7d = F(metrics.Change(last, st.daily, 7))
	r.Chg30d = F(metrics.Change(last, st.daily, 30))
	r.VolX7 = F(metrics.VolVsDays(vol24, st.daily, 7))
	r.Regime = F(metrics.VolRegime(st.daily, 20))
	r.Range1d = F(metrics.RangeToday(st.daily))
	r.ToHigh = F(metrics.DistanceToHigh(last, st.daily, 30))
	r.ToLow = F(metrics.DistanceToLow(last, st.daily, 30))

	// Book block
	if !st.depthAt.IsZero() && now.Sub(st.depthAt) < 10*time.Minute {
		r.Cov.Book = true
		r.Depth01 = F(st.depth.Bid01 + st.depth.Ask01)
		r.Depth05 = F(st.depth.Bid05 + st.depth.Ask05)
		r.Depth1 = F(st.depth.Bid1 + st.depth.Ask1)
		r.Imb = F(st.depth.Imbalance05)
		r.Spread = F(st.depth.SpreadBps)
	} else {
		nan := F(math.NaN())
		r.Depth01, r.Depth05, r.Depth1, r.Imb, r.Spread = nan, nan, nan, nan, nan
	}
	for _, p := range Periods {
		r.TakerBuy[p] = F(math.NaN())
		r.Delta[p] = F(math.NaN())
	}
	return r
}

// oiChange returns oiNow / OI at (now − back) − 1 using the oldest point not
// newer than that moment; NaN when history is too short.
func oiChange(hist []oiPoint, oiNow float64, nowMs, backMs int64) float64 {
	if oiNow <= 0 || len(hist) == 0 {
		return math.NaN()
	}
	target := nowMs - backMs
	if hist[0].ts > target+90_000 { // allow 1.5 min slack on the first point
		return math.NaN()
	}
	// Binary search the last point with ts <= target.
	i := sort.Search(len(hist), func(i int) bool { return hist[i].ts > target })
	if i == 0 {
		i = 1
	}
	ref := hist[i-1].usd
	if ref <= 0 {
		return math.NaN()
	}
	return oiNow/ref - 1
}

// Rows returns the latest rows and their time.
func (c *Collector) Rows() ([]Row, time.Time) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.rows, c.rowsAt
}

// Instruments returns the universe.
func (c *Collector) Instruments() []okx.Instrument {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]okx.Instrument, 0, len(c.states))
	for _, st := range c.states {
		if st.inst.CtType != "" {
			out = append(out, st.inst)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Health returns a copy of the health block.
func (c *Collector) Health() Health {
	c.mu.RLock()
	defer c.mu.RUnlock()
	h := c.health
	h.UptimeSec = int64(time.Since(c.started).Seconds())
	h.Loops = make(map[string]Loop, len(c.health.Loops))
	for k, v := range c.health.Loops {
		h.Loops[k] = v
	}
	h.Coverage = make(map[string]int, len(c.health.Coverage))
	for k, v := range c.health.Coverage {
		h.Coverage[k] = v
	}
	return h
}
