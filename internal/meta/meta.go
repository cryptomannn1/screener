// Package meta resolves asset metadata that the venue does not carry: market
// capitalisation, fully diluted valuation, market-cap rank and a sector for the
// crypto part of the universe. The source is the CoinGecko REST API, which
// works without a key at a few calls per minute; a demo or pro key raises the
// budget. Stocks, ETFs, indices and commodities are not covered here: their
// class comes from the venue (okx.Instrument.AssetClass) and their
// capitalisation stays unknown.
//
// Matching is by ticker symbol against the top coins by market cap, best rank
// first, with a short override list for tickers that collide (several coins
// share MEME, ONE, AI, …). A sector is the first entry of a fixed taxonomy
// whose CoinGecko categories contain the coin, so every coin has exactly one
// sector and the list of sectors is ours to keep stable.
package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Asset is what the resolver knows about one base symbol.
type Asset struct {
	Symbol  string    `json:"symbol"` // venue base, e.g. BTC
	ID      string    `json:"id"`     // CoinGecko id, e.g. bitcoin
	Name    string    `json:"name"`
	Mcap    float64   `json:"mcap"`   // USD; NaN when unknown
	FDV     float64   `json:"fdv"`    // USD; NaN when unknown
	Rank    int       `json:"rank"`   // market-cap rank; 0 when unknown
	Sector  string    `json:"sector"` // one of Sectors, or "" before sectors loaded
	Updated time.Time `json:"updated"`
}

// Sector names in priority order: a coin that sits in several categories
// takes the first sector of this list that contains it.
var Sectors = []string{"Stable", "Meme", "Exchange", "Privacy", "L2", "L1", "AI", "RWA", "DePIN", "Gaming", "DeFi", "Infra"}

// SectorOther is the sector of a matched coin that belongs to no category above.
const SectorOther = "Other"

// taxonomy maps each sector to the CoinGecko categories (and how many pages of
// 250 to read for each) that define it.
var taxonomy = []struct {
	sector string
	cats   []catPages
}{
	{"Stable", []catPages{{"stablecoins", 1}}},
	{"Meme", []catPages{{"meme-token", 2}}},
	{"Exchange", []catPages{{"centralized-exchange-token-cex", 1}}},
	{"Privacy", []catPages{{"privacy-coins", 1}}},
	{"L2", []catPages{{"layer-2", 1}}},
	// CoinGecko files DOT / ATOM / KSM under Layer 0 and the older chains
	// (NEO, QTUM, ONT, ZIL) only under Smart Contract Platform.
	{"L1", []catPages{{"layer-1", 2}, {"layer-0-l0", 1}, {"smart-contract-platform", 2}}},
	{"AI", []catPages{{"artificial-intelligence", 2}, {"ai-agents", 1}}},
	{"RWA", []catPages{{"real-world-assets-rwa", 1}}},
	{"DePIN", []catPages{{"depin", 1}}},
	{"Gaming", []catPages{{"gaming", 2}}},
	{"DeFi", []catPages{{"decentralized-finance-defi", 2}, {"decentralized-exchange", 1}, {"lending-borrowing", 1}, {"liquid-staking-tokens", 1}, {"restaking", 1}}},
	{"Infra", []catPages{{"infrastructure", 1}, {"oracle", 1}, {"data-availability", 1}, {"zero-knowledge-zk", 1}, {"bridge-governance-tokens", 1}, {"interoperability", 1}}},
}

type catPages struct {
	id    string
	pages int
}

// idOverrides pins a venue symbol to a CoinGecko id where the best-ranked coin
// with that symbol is not the one the venue lists.
var idOverrides = map[string]string{
	"MEME": "memecoin-2",
	"ONE":  "harmony",
	"AI":   "sleepless-ai",
	"PROS": "prosper",
	"MASK": "mask-network",
	"EDGE": "definitive",
	"ARX":  "arcium",
}

// sectorOverrides fixes the few majors where the category order would mislead.
var sectorOverrides = map[string]string{
	"TAO": "AI", "RENDER": "AI", "FET": "AI", "WLD": "AI", "VIRTUAL": "AI",
	"OKB": "Exchange", "BNB": "Exchange", "CRO": "Exchange",
	"BTC": "L1", "ETH": "L1",
	"LINK": "Infra",                // oracle, not the RWA tag CoinGecko also gives it
	"FIL":  "DePIN", "AR": "DePIN", // storage networks, filed under L1 by CoinGecko
	"DASH": "Privacy", "ZEC": "Privacy", "XMR": "Privacy",
}

// Config of the resolver.
type Config struct {
	BaseURL      string        // default https://api.coingecko.com/api/v3
	APIKey       string        // demo or pro key; empty = public limits
	Pages        int           // market pages of 250 to scan, default 8 (top 2000)
	Spacing      time.Duration // pause between calls; default 13s public, 2.5s with a key
	McapEvery    time.Duration // market-cap refresh, default 30m
	SectorsEvery time.Duration // sector refresh, default 24h
}

// Cache persists resolved assets between runs so a restart needs no calls.
type Cache interface {
	SaveAssets(ctx context.Context, assets []Asset) error
	LoadAssets(ctx context.Context) ([]Asset, error)
}

// Health of the resolver for /health.
type Health struct {
	Symbols   int            `json:"symbols"` // crypto symbols asked for
	Matched   int            `json:"matched"` // with a CoinGecko coin
	Sectored  int            `json:"sectored"`
	McapAt    int64          `json:"mcapAt"`    // ms of the last market-cap pass
	SectorsAt int64          `json:"sectorsAt"` // ms of the last sector pass
	Calls     int            `json:"calls"`
	Errors    int            `json:"errors"`
	LastErr   string         `json:"lastErr,omitempty"`
	LastErrAt int64          `json:"lastErrAt,omitempty"`
	Sectors   map[string]int `json:"sectors"`
	Unmatched []string       `json:"unmatched,omitempty"`
}

// Service resolves and refreshes assets for a symbol set.
type Service struct {
	cfg   Config
	log   *slog.Logger
	http  *http.Client
	cache Cache

	mu       sync.RWMutex
	assets   map[string]Asset // by venue symbol
	symbols  []string
	health   Health
	lastCall time.Time
}

// New returns a resolver; cache may be nil.
func New(cfg Config, log *slog.Logger, cache Cache) *Service {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.coingecko.com/api/v3"
	}
	if cfg.Pages <= 0 {
		cfg.Pages = 8
	}
	if cfg.Spacing <= 0 {
		if cfg.APIKey != "" {
			cfg.Spacing = 2500 * time.Millisecond
		} else {
			cfg.Spacing = 13 * time.Second
		}
	}
	if cfg.McapEvery <= 0 {
		cfg.McapEvery = 30 * time.Minute
	}
	if cfg.SectorsEvery <= 0 {
		cfg.SectorsEvery = 24 * time.Hour
	}
	return &Service{
		cfg: cfg, log: log, cache: cache,
		http:   &http.Client{Timeout: 20 * time.Second},
		assets: map[string]Asset{},
		health: Health{Sectors: map[string]int{}},
	}
}

// Lookup returns the asset for a venue base symbol.
func (s *Service) Lookup(symbol string) (Asset, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.assets[symbol]
	return a, ok
}

// Health reports the resolver state.
func (s *Service) Health() Health {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h := s.health
	h.Sectors = map[string]int{}
	for _, a := range s.assets {
		if a.Sector != "" {
			h.Sectors[a.Sector]++
		}
	}
	return h
}

// Seed loads the cache so rows carry metadata before the first API pass.
func (s *Service) Seed(ctx context.Context) {
	if s.cache == nil {
		return
	}
	assets, err := s.cache.LoadAssets(ctx)
	if err != nil {
		s.log.Warn("meta seed", "err", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var mcapAt, secAt time.Time
	for _, a := range assets {
		s.assets[a.Symbol] = a
		if a.Updated.After(mcapAt) {
			mcapAt = a.Updated
		}
		if a.Sector != "" && a.Updated.After(secAt) {
			secAt = a.Updated
		}
	}
	if !mcapAt.IsZero() {
		s.health.McapAt = mcapAt.UnixMilli()
	}
	if !secAt.IsZero() {
		s.health.SectorsAt = secAt.UnixMilli()
	}
	s.log.Info("meta seeded", "assets", len(assets))
}

// Run refreshes market caps and sectors until ctx ends. symbols returns the
// crypto base symbols of the current universe.
func (s *Service) Run(ctx context.Context, symbols func() []string) error {
	// Wait for the universe to exist.
	var syms []string
	for len(syms) == 0 {
		syms = symbols()
		if len(syms) > 0 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	for {
		syms = symbols()
		s.mu.Lock()
		s.symbols = syms
		s.health.Symbols = len(syms)
		mcapDue := time.Since(time.UnixMilli(s.health.McapAt)) >= s.cfg.McapEvery
		secDue := time.Since(time.UnixMilli(s.health.SectorsAt)) >= s.cfg.SectorsEvery
		s.mu.Unlock()
		if mcapDue {
			if err := s.refreshMcap(ctx, syms); err != nil && !errors.Is(err, context.Canceled) {
				s.fail(err)
			}
		}
		if secDue {
			if err := s.refreshSectors(ctx, syms); err != nil && !errors.Is(err, context.Canceled) {
				s.fail(err)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Minute):
		}
	}
}

func (s *Service) fail(err error) {
	s.mu.Lock()
	s.health.Errors++
	s.health.LastErr = err.Error()
	s.health.LastErrAt = time.Now().UnixMilli()
	s.mu.Unlock()
	s.log.Warn("meta", "err", err)
}

// coin is one row of /coins/markets.
type coin struct {
	ID     string   `json:"id"`
	Symbol string   `json:"symbol"`
	Name   string   `json:"name"`
	Mcap   *float64 `json:"market_cap"`
	FDV    *float64 `json:"fully_diluted_valuation"`
	Rank   *int     `json:"market_cap_rank"`
}

// refreshMcap scans the market pages and re-resolves every symbol.
func (s *Service) refreshMcap(ctx context.Context, syms []string) error {
	var coins []coin
	for p := 1; p <= s.cfg.Pages; p++ {
		page, err := s.markets(ctx, "", p)
		if err != nil {
			if len(coins) == 0 {
				return err
			}
			s.log.Warn("meta markets page", "page", p, "err", err)
			break
		}
		coins = append(coins, page...)
		if len(page) < 250 {
			break
		}
	}
	matched := Resolve(syms, coins)
	now := time.Now()
	s.mu.Lock()
	var unmatched []string
	for _, sym := range syms {
		c, ok := matched[sym]
		prev := s.assets[sym]
		if !ok {
			if prev.ID == "" {
				unmatched = append(unmatched, sym)
			}
			continue
		}
		a := Asset{Symbol: sym, ID: c.ID, Name: c.Name, Mcap: math.NaN(), FDV: math.NaN(), Sector: prev.Sector, Updated: now}
		if c.Mcap != nil && *c.Mcap > 0 {
			a.Mcap = *c.Mcap
		}
		if c.FDV != nil && *c.FDV > 0 {
			a.FDV = *c.FDV
		}
		if c.Rank != nil {
			a.Rank = *c.Rank
		}
		if prev.ID != "" && prev.ID != c.ID {
			a.Sector = "" // a different coin now: its sector is unknown until the next sector pass
		}
		s.assets[sym] = a
	}
	s.health.Matched = len(matched)
	s.health.Unmatched = unmatched
	s.health.McapAt = now.UnixMilli()
	snapshot := s.snapshotLocked()
	s.mu.Unlock()
	s.log.Info("meta mcap pass", "symbols", len(syms), "matched", len(matched), "coins", len(coins))
	s.persist(ctx, snapshot)
	return nil
}

// refreshSectors reads the taxonomy categories and assigns one sector per coin.
func (s *Service) refreshSectors(ctx context.Context, syms []string) error {
	s.mu.RLock()
	ids := map[string]string{} // coin id -> symbol
	for _, sym := range syms {
		if a, ok := s.assets[sym]; ok && a.ID != "" {
			ids[a.ID] = sym
		}
	}
	s.mu.RUnlock()
	if len(ids) == 0 {
		return errors.New("sectors: no matched coins yet")
	}
	members := map[string]map[string]bool{} // sector -> coin ids
	for _, t := range taxonomy {
		set := map[string]bool{}
		for _, cp := range t.cats {
			for p := 1; p <= cp.pages; p++ {
				page, err := s.markets(ctx, cp.id, p)
				if err != nil {
					return fmt.Errorf("category %s: %w", cp.id, err)
				}
				for _, c := range page {
					set[c.ID] = true
				}
				if len(page) < 250 {
					break
				}
			}
		}
		members[t.sector] = set
	}
	now := time.Now()
	s.mu.Lock()
	n := 0
	for id, sym := range ids {
		a := s.assets[sym]
		a.Sector = SectorOf(sym, id, members)
		a.Updated = now
		s.assets[sym] = a
		n++
	}
	s.health.Sectored = n
	s.health.SectorsAt = now.UnixMilli()
	snapshot := s.snapshotLocked()
	s.mu.Unlock()
	s.log.Info("meta sector pass", "coins", n)
	s.persist(ctx, snapshot)
	return nil
}

// SectorOf picks the sector of a coin: an override by symbol, else the first
// taxonomy sector whose members contain the id, else Other.
func SectorOf(symbol, id string, members map[string]map[string]bool) string {
	if sec, ok := sectorOverrides[symbol]; ok {
		return sec
	}
	for _, sec := range Sectors {
		if members[sec][id] {
			return sec
		}
	}
	return SectorOther
}

// Resolve matches venue symbols to coins: an override id when present in the
// list, else the best-ranked coin with that symbol.
func Resolve(syms []string, coins []coin) map[string]coin {
	byID := map[string]coin{}
	bySym := map[string][]coin{}
	for _, c := range coins {
		byID[c.ID] = c
		bySym[strings.ToUpper(c.Symbol)] = append(bySym[strings.ToUpper(c.Symbol)], c)
	}
	rank := func(c coin) int {
		if c.Rank == nil || *c.Rank <= 0 {
			return math.MaxInt32
		}
		return *c.Rank
	}
	out := map[string]coin{}
	for _, sym := range syms {
		if id, ok := idOverrides[sym]; ok {
			if c, ok := byID[id]; ok {
				out[sym] = c
				continue
			}
		}
		cands := bySym[sym]
		if len(cands) == 0 {
			continue
		}
		sort.SliceStable(cands, func(i, j int) bool { return rank(cands[i]) < rank(cands[j]) })
		out[sym] = cands[0]
	}
	return out
}

func (s *Service) snapshotLocked() []Asset {
	out := make([]Asset, 0, len(s.assets))
	for _, a := range s.assets {
		out = append(out, a)
	}
	return out
}

func (s *Service) persist(ctx context.Context, assets []Asset) {
	if s.cache == nil || len(assets) == 0 {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := s.cache.SaveAssets(cctx, assets); err != nil {
		s.log.Warn("meta cache save", "err", err)
	}
}

// markets calls /coins/markets for one page, optionally within a category.
func (s *Service) markets(ctx context.Context, category string, page int) ([]coin, error) {
	q := url.Values{}
	q.Set("vs_currency", "usd")
	q.Set("order", "market_cap_desc")
	q.Set("per_page", "250")
	q.Set("page", strconv.Itoa(page))
	q.Set("sparkline", "false")
	if category != "" {
		q.Set("category", category)
	}
	var out []coin
	if err := s.get(ctx, "/coins/markets", q, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ErrRateLimited is returned when the API answers 429 twice in a row.
var ErrRateLimited = errors.New("meta: rate limited")

func (s *Service) get(ctx context.Context, path string, q url.Values, dst any) error {
	for attempt := 0; attempt < 2; attempt++ {
		if err := s.pace(ctx); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.BaseURL+path+"?"+q.Encode(), nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		if s.cfg.APIKey != "" {
			if strings.Contains(s.cfg.BaseURL, "pro-api") {
				req.Header.Set("x-cg-pro-api-key", s.cfg.APIKey)
			} else {
				req.Header.Set("x-cg-demo-api-key", s.cfg.APIKey)
			}
		}
		s.mu.Lock()
		s.health.Calls++
		s.mu.Unlock()
		resp, err := s.http.Do(req)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			return err
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			s.log.Debug("meta 429, backing off", "path", path)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(65 * time.Second):
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("meta %s: http %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body[:min(len(body), 160)])))
		}
		return json.Unmarshal(body, dst)
	}
	return ErrRateLimited
}

// pace keeps the configured spacing between calls.
func (s *Service) pace(ctx context.Context) error {
	s.mu.Lock()
	wait := time.Until(s.lastCall.Add(s.cfg.Spacing))
	s.lastCall = time.Now()
	if wait > 0 {
		s.lastCall = s.lastCall.Add(wait)
	}
	s.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
		return nil
	}
}
