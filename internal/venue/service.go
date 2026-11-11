package venue

import (
	"context"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"screener/internal/collector"
	"screener/internal/meta"
	"screener/internal/metrics"
)

// Config tunes the polling. Zero values take the defaults.
type Config struct {
	BinanceURL    string        // "" = https://fapi.binance.com
	BybitURL      string        // "" = https://api.bybit.com
	TickersEvery  time.Duration // bulk tickers of both exchanges; default 5 s
	PremiumEvery  time.Duration // Binance mark prices and funding rates; default 30 s
	UniverseEvery time.Duration // instrument lists; default 30 min
	HourlyEvery   time.Duration // refresh of the hourly candles of assets OKX does not list; default 10 min
	DailyEvery    time.Duration // refresh of their daily candles; default 30 min
	BinancePerSec int           // per-symbol Binance requests per second (open interest, candles); default 4
	BybitPerSec   int           // Bybit requests per second; default 8

	// Streams: tickers and mark prices of both exchanges over WebSocket, about once a second (see stream.go).
	NoStreams      bool          // true = REST polling only
	BinanceWS      string        // "" = BinanceWSURL
	BybitWS        string        // "" = BybitWSURL
	ReconcileEvery time.Duration // REST tickers while the exchange's stream is healthy; default 60 s
}

func (c Config) withDefaults() Config {
	def := func(d *time.Duration, v time.Duration) {
		if *d <= 0 {
			*d = v
		}
	}
	def(&c.TickersEvery, 5*time.Second)
	def(&c.PremiumEvery, 30*time.Second)
	def(&c.UniverseEvery, 30*time.Minute)
	def(&c.HourlyEvery, 10*time.Minute)
	def(&c.DailyEvery, 30*time.Minute)
	def(&c.ReconcileEvery, time.Minute)
	if c.BinanceWS == "" {
		c.BinanceWS = BinanceWSURL
	}
	if c.BybitWS == "" {
		c.BybitWS = BybitWSURL
	}
	if c.BinancePerSec < 1 {
		c.BinancePerSec = 4
	}
	if c.BybitPerSec < 1 {
		c.BybitPerSec = 8
	}
	return c
}

// RowSource is the OKX collector: its rows carry OKX's block and the metrics
// of every asset OKX lists.
type RowSource interface {
	Rows() ([]collector.Row, time.Time)
}

// MetaSource resolves a coin's name, market cap, rank and sector.
type MetaSource interface {
	Lookup(symbol string) (meta.Asset, bool)
}

// Block is one exchange's figures for an asset. The USDT and USDC contracts of
// the asset on that exchange add up in Vol and OI; price, change and funding
// are those of the busier contract.
type Block struct {
	P   collector.F `json:"p"`   // last price of one coin, USD
	C   collector.F `json:"c"`   // 24 h change, fraction
	Vol collector.F `json:"vol"` // 24 h volume, USD
	OI  collector.F `json:"oi"`  // open interest, USD
	F   collector.F `json:"f"`   // funding rate per interval
	FI  float64     `json:"fi"`  // funding interval, hours
}

// Metrics is the asset-level block: price-derived metrics are the same on
// every exchange, so they are computed once, from OKX when it lists the asset
// and from Bybit or Binance candles otherwise. Flow, order-book and
// open-interest history exist only for assets on OKX.
type Metrics struct {
	Spark    []collector.F          `json:"spark"` // last 24 hourly closes, oldest first
	Chg      collector.PerPeriod    `json:"chg"`   // 1h, 4h (24h is in each exchange block)
	Chg7d    collector.F            `json:"chg7d"`
	Chg30d   collector.F            `json:"chg30d"`
	VolX7    collector.F            `json:"volx7"`
	VolX     collector.PerPeriod    `json:"volx"`
	OID      collector.PerPeriod    `json:"oiD"`
	ATR      collector.F            `json:"atr"`
	Regime   collector.F            `json:"regime"`
	TakerBuy collector.PerPeriod    `json:"takerBuy"`
	Delta    collector.PerPeriod    `json:"delta"`
	Tape     collector.F            `json:"tape"`
	VsBtc    collector.PerPeriod    `json:"vsBtc"` // 1h, 4h
	Beta     collector.F            `json:"beta"`
	Corr     collector.F            `json:"corr"`
	Spread   collector.F            `json:"spread"`
	Depth05  collector.F            `json:"depth05"`
	Lv       map[string]collector.F `json:"lv"` // level prices: do, wo, mo, ma50, ma100, ma200
}

// Asset is one line of GET /assets.
type Asset struct {
	K    string           `json:"k"`    // asset id: c:BTC, t:NVDA
	S    string           `json:"s"`    // ticker
	N    string           `json:"n"`    // name, when known
	Cls  string           `json:"cls"`  // crypto | stock | commodity | fx
	Sec  string           `json:"sec"`  // coin sector, or the kind of a TradFi instrument
	Mcap collector.F      `json:"mcap"` // market cap, USD; coins only
	Rank collector.F      `json:"rank"` // market-cap rank; coins only
	V    map[string]Block `json:"v"`    // per exchange: okx, bn, bb
	M    Metrics          `json:"m"`
}

// LoopHealth is the state of one polling loop.
type LoopHealth struct {
	LastOK    int64  `json:"lastOk"`
	LastErr   string `json:"lastErr,omitempty"`
	LastErrAt int64  `json:"lastErrAt,omitempty"`
	Passes    int64  `json:"passes"`
}

// Health is the block of /health.
type Health struct {
	Instruments map[string]int          `json:"instruments"` // linear perpetuals per exchange
	Assets      int                     `json:"assets"`
	OutsideOKX  int                     `json:"outsideOkx"` // assets OKX does not list
	Candles     int                     `json:"candles"`    // of them, with hourly and daily candles loaded
	BinanceOI   int                     `json:"binanceOi"`  // Binance symbols with an open-interest reading
	Loops       map[string]LoopHealth   `json:"loops"`
	Streams     map[string]StreamHealth `json:"streams,omitempty"` // bn, bb; absent when the streams are off
}

type oiReading struct {
	contracts float64
	at        time.Time
}

// series holds the candles of the instrument that stands for an asset outside
// OKX, newest first, in prices of one coin.
type series struct {
	inst     Instrument
	hourly   []metrics.Bar
	daily    []metrics.Bar
	hourlyAt time.Time
	dailyAt  time.Time
	ret      []float64 // hourly log returns, oldest first
}

const benchKey = "#bench"

// Service reads Binance and Bybit - streams for the fast figures, REST for the
// rest - and builds the asset rows.
type Service struct {
	cfg  Config
	log  *slog.Logger
	bn   *BinanceClient
	bb   *BybitClient
	rows RowSource
	meta MetaSource // may be nil

	mu      sync.RWMutex
	inst    map[string]map[string]Instrument // exchange -> symbol
	tick    map[string]map[string]Ticker     // exchange -> symbol
	premium map[string]Premium               // Binance symbol
	bnOI    map[string]oiReading             // Binance symbol
	series  map[string]*series               // asset key, plus benchKey
	loops   map[string]LoopHealth
	restAt  map[string]time.Time // loop name -> last REST answer, for the reconciliation pace

	runCtx   context.Context // set by startStreams
	bnFeed   *feed
	bbFeeds  []*feed
	bbGroups [][]string // Bybit symbols per connection

	cacheMu sync.Mutex
	cacheAt time.Time
	cache   []Asset
}

// New builds the service. rows is the OKX collector; md may be nil.
func New(cfg Config, rows RowSource, md MetaSource, log *slog.Logger) *Service {
	cfg = cfg.withDefaults()
	return &Service{
		cfg: cfg, log: log, rows: rows, meta: md,
		bn:      NewBinance(cfg.BinanceURL, cfg.BinancePerSec),
		bb:      NewBybit(cfg.BybitURL, cfg.BybitPerSec),
		inst:    map[string]map[string]Instrument{Binance: {}, Bybit: {}},
		tick:    map[string]map[string]Ticker{Binance: {}, Bybit: {}},
		premium: map[string]Premium{}, bnOI: map[string]oiReading{}, series: map[string]*series{},
		loops: map[string]LoopHealth{}, restAt: map[string]time.Time{},
	}
}

// Prime loads the instrument lists once, so the metadata resolver sees the
// whole universe on its first pass. Errors are left to the regular loop.
func (s *Service) Prime(ctx context.Context) {
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	s.mark("universe", s.syncUniverse(cctx))
}

// Run polls until ctx ends.
func (s *Service) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	start := func(fn func()) {
		wg.Add(1)
		go func() { defer wg.Done(); fn() }()
	}
	if !s.cfg.NoStreams {
		s.startStreams(ctx)
	}
	start(func() { s.loop(ctx, "universe", s.cfg.UniverseEvery, s.syncUniverse, true) })
	start(func() { s.loop(ctx, "binance.tickers", s.cfg.TickersEvery, s.pollBinance, false) })
	start(func() { s.loop(ctx, "binance.premium", s.cfg.PremiumEvery, s.pollPremium, false) })
	start(func() { s.loop(ctx, "bybit.tickers", s.cfg.TickersEvery, s.pollBybit, false) })
	start(func() { s.oiLoop(ctx) })
	start(func() { s.candleLoop(ctx) })
	wg.Wait()
	return ctx.Err()
}

func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// loop runs fn every period; waitFirst skips the immediate run (Prime did it).
func (s *Service) loop(ctx context.Context, name string, every time.Duration, fn func(context.Context) error, waitFirst bool) {
	if waitFirst && !sleep(ctx, every) {
		return
	}
	for {
		s.mark(name, fn(ctx))
		if !sleep(ctx, every) {
			return
		}
	}
}

// mark records a pass and logs a failure once, when it starts or changes.
func (s *Service) mark(name string, err error) {
	if err != nil && (err == context.Canceled || strings.Contains(err.Error(), "context canceled")) {
		return
	}
	now := time.Now().UnixMilli()
	s.mu.Lock()
	h := s.loops[name]
	h.Passes++
	changed := false
	if err != nil {
		changed = h.LastErr != err.Error() || h.LastOK > h.LastErrAt
		h.LastErr, h.LastErrAt = err.Error(), now
	} else {
		h.LastOK = now
	}
	s.loops[name] = h
	s.mu.Unlock()
	if err != nil && changed {
		s.log.Warn("venue loop", "loop", name, "err", err)
	}
}

func (s *Service) syncUniverse(ctx context.Context) error {
	bn, errBn := s.bn.Instruments(ctx)
	bb, errBb := s.bb.Instruments(ctx)
	index := func(list []Instrument) map[string]Instrument {
		m := make(map[string]Instrument, len(list))
		for _, i := range list {
			m[i.Symbol] = i
		}
		return m
	}
	s.mu.Lock()
	if errBn == nil && len(bn) > 0 {
		s.inst[Binance] = index(bn)
	}
	if errBb == nil && len(bb) > 0 {
		s.inst[Bybit] = index(bb)
	}
	nBn, nBb := len(s.inst[Binance]), len(s.inst[Bybit])
	s.mu.Unlock()
	if errBb == nil {
		s.planBybit() // a changed symbol set re-subscribes the stream
	}
	if errBn != nil {
		return errBn
	}
	if errBb != nil {
		return errBb
	}
	s.log.Info("venue universe", "binance", nBn, "bybit", nBb)
	return nil
}

// The REST polls below are the whole feed without the streams. While an
// exchange's stream is healthy they go out only once per ReconcileEvery: that
// refreshes the symbols the stream has not mentioned and drops delisted ones.
func (s *Service) pollBinance(ctx context.Context) error {
	if !s.restDue("binance.tickers", Binance) {
		return nil
	}
	t, err := s.bn.Tickers(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.tick[Binance] = t
	s.restAt["binance.tickers"] = time.Now()
	s.mu.Unlock()
	return nil
}

func (s *Service) pollPremium(ctx context.Context) error {
	if !s.restDue("binance.premium", Binance) {
		return nil
	}
	p, err := s.bn.Premiums(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.premium = p
	s.restAt["binance.premium"] = time.Now()
	s.mu.Unlock()
	return nil
}

func (s *Service) pollBybit(ctx context.Context) error {
	if !s.restDue("bybit.tickers", Bybit) {
		return nil
	}
	t, err := s.bb.Tickers(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.tick[Bybit] = t
	s.restAt["bybit.tickers"] = time.Now()
	s.mu.Unlock()
	return nil
}

// symbolsByVolume lists an exchange's instruments, busiest first.
func (s *Service) symbolsByVolume(venue string) []Instrument {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Instrument, 0, len(s.inst[venue]))
	for _, i := range s.inst[venue] {
		out = append(out, i)
	}
	vol := func(sym string) float64 {
		v := s.tick[venue][sym].Vol24
		if math.IsNaN(v) {
			return 0
		}
		return v
	}
	sort.Slice(out, func(a, b int) bool {
		va, vb := vol(out[a].Symbol), vol(out[b].Symbol)
		if va != vb {
			return va > vb
		}
		return out[a].Symbol < out[b].Symbol
	})
	return out
}

// oiLoop walks Binance's symbols for open interest: the exchange publishes it
// one symbol per request. At 4 requests a second a pass over ~800 symbols
// takes a little over three minutes.
func (s *Service) oiLoop(ctx context.Context) {
	for ctx.Err() == nil {
		list := s.symbolsByVolume(Binance)
		s.mu.RLock()
		ticking := len(s.tick[Binance]) > 0 // wait for volumes, so that the first pass already starts with the busiest symbols
		s.mu.RUnlock()
		if len(list) == 0 || !ticking {
			if !sleep(ctx, 3*time.Second) {
				return
			}
			continue
		}
		okN := 0
		var last error
		for _, in := range list {
			if ctx.Err() != nil {
				return
			}
			v, err := s.bn.OpenInterest(ctx, in.Symbol)
			if err != nil {
				last = err
				pause := 500 * time.Millisecond
				if strings.Contains(err.Error(), "http 429") || strings.Contains(err.Error(), "http 418") {
					pause = time.Minute // the exchange asks to slow down: stop for a while rather than earn a ban
				}
				if !sleep(ctx, pause) {
					return
				}
				continue
			}
			okN++
			s.mu.Lock()
			s.bnOI[in.Symbol] = oiReading{contracts: v, at: time.Now()}
			s.mu.Unlock()
		}
		if okN > 0 {
			last = nil
		}
		s.mark("binance.oi", last)
	}
}

// CryptoBases lists the coin tickers of Binance and Bybit, for the metadata
// resolver.
func (s *Service) CryptoBases() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	var out []string
	for _, v := range []string{Binance, Bybit} {
		for _, i := range s.inst[v] {
			if i.Class == ClassCrypto && i.Kind == "" && !seen[i.Base] {
				seen[i.Base] = true
				out = append(out, i.Base)
			}
		}
	}
	sort.Strings(out)
	return out
}

// assetsTTL is how long a built list is served again. It is shorter than the
// one-second push of /stream on purpose: with a full second a push that fires
// a moment early would find the previous build still valid and skip a beat.
const assetsTTL = 750 * time.Millisecond

// Assets returns the asset rows, rebuilt at most once per assetsTTL.
func (s *Service) Assets() ([]Asset, time.Time) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	now := time.Now()
	if s.cache != nil && now.Sub(s.cacheAt) < assetsTTL {
		return s.cache, s.cacheAt
	}
	s.cache, s.cacheAt = s.build(now), now
	return s.cache, s.cacheAt
}

// Health reports the state of the loops and the coverage.
func (s *Service) Health() Health {
	assets, _ := s.Assets()
	h := Health{Instruments: map[string]int{}, Assets: len(assets), Loops: map[string]LoopHealth{}}
	for _, a := range assets {
		if _, ok := a.V[OKX]; ok {
			h.Instruments[OKX]++ // assets, for OKX: its instruments are in the collector's health
		} else {
			h.OutsideOKX++
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	h.Instruments[Binance], h.Instruments[Bybit] = len(s.inst[Binance]), len(s.inst[Bybit])
	for k, se := range s.series {
		if k != benchKey && len(se.hourly) > 0 && len(se.daily) > 0 {
			h.Candles++
		}
	}
	h.BinanceOI = len(s.bnOI)
	h.Streams = s.streamHealth()
	for k, v := range s.loops {
		h.Loops[k] = v
	}
	return h
}
