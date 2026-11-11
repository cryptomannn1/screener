package venue

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strconv"

	"screener/internal/metrics"
)

// BybitClient reads the v5 public REST API, category linear. One tickers call
// carries price, 24 h change, turnover, open interest in USD, funding rate and
// its interval for every symbol.
type BybitClient struct{ g getter }

// NewBybit builds the client; base "" is the production API.
func NewBybit(base string, perSec int) *BybitClient {
	if base == "" {
		base = "https://api.bybit.com"
	}
	if perSec < 1 {
		perSec = 8
	}
	return &BybitClient{g: newGetter(base, perSec)}
}

type bybitResp[T any] struct {
	RetCode int    `json:"retCode"`
	RetMsg  string `json:"retMsg"`
	Result  struct {
		List   []T    `json:"list"`
		Cursor string `json:"nextPageCursor"`
	} `json:"result"`
}

func bybitGet[T any](ctx context.Context, g getter, path string, q url.Values) ([]T, string, error) {
	var r bybitResp[T]
	if err := g.get(ctx, path, q, &r); err != nil {
		return nil, "", err
	}
	if r.RetCode != 0 {
		return nil, "", fmt.Errorf("%s: retCode %d: %s", path, r.RetCode, r.RetMsg)
	}
	return r.Result.List, r.Result.Cursor, nil
}

// Instruments lists the trading linear perpetuals, crypto and TradFi.
func (b *BybitClient) Instruments(ctx context.Context) ([]Instrument, error) {
	type item struct {
		Symbol       string  `json:"symbol"`
		ContractType string  `json:"contractType"`
		Status       string  `json:"status"`
		BaseCoin     string  `json:"baseCoin"`
		QuoteCoin    string  `json:"quoteCoin"`
		SymbolType   string  `json:"symbolType"`
		FundingMin   float64 `json:"fundingInterval"`
	}
	var out []Instrument
	cursor := ""
	for page := 0; page < 20; page++ {
		q := url.Values{"category": {"linear"}, "limit": {"1000"}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		list, next, err := bybitGet[item](ctx, b.g, "/v5/market/instruments-info", q)
		if err != nil {
			return nil, err
		}
		for _, s := range list {
			if s.Status != "Trading" || s.ContractType != "LinearPerpetual" || (s.QuoteCoin != "USDT" && s.QuoteCoin != "USDC") {
				continue
			}
			class, kind := bybitClass(s.SymbolType)
			base, mult := Canonical(class, s.BaseCoin)
			out = append(out, Instrument{Venue: Bybit, Symbol: s.Symbol, Base: base, Mult: mult, Quote: s.QuoteCoin, Class: class, Kind: kind, FundingH: s.FundingMin / 60})
		}
		if next == "" || len(list) == 0 {
			break
		}
		cursor = next
	}
	return out, nil
}

// bybitClass maps Bybit's symbolType; "" and "innovation" are coins.
func bybitClass(t string) (class, kind string) {
	switch t {
	case "stock":
		return ClassStock, "Stock"
	case "ETF":
		return ClassStock, "ETF"
	case "commodity":
		return ClassCommodity, "Commodity"
	case "forex":
		return ClassFX, "FX"
	}
	return ClassCrypto, ""
}

// Tickers returns the tickers of every linear symbol.
func (b *BybitClient) Tickers(ctx context.Context) (map[string]Ticker, error) {
	type item struct {
		Symbol   string `json:"symbol"`
		Last     string `json:"lastPrice"`
		Mark     string `json:"markPrice"`
		Chg      string `json:"price24hPcnt"`
		Turnover string `json:"turnover24h"`
		OIValue  string `json:"openInterestValue"`
		Funding  string `json:"fundingRate"`
		FundingH string `json:"fundingIntervalHour"`
		Prev1h   string `json:"prevPrice1h"`
	}
	list, _, err := bybitGet[item](ctx, b.g, "/v5/market/tickers", url.Values{"category": {"linear"}})
	if err != nil {
		return nil, err
	}
	out := make(map[string]Ticker, len(list))
	for _, r := range list {
		h := num(r.FundingH)
		if math.IsNaN(h) {
			h = 0
		}
		out[r.Symbol] = Ticker{Symbol: r.Symbol, Last: num(r.Last), Mark: num(r.Mark), Chg24: num(r.Chg), Vol24: num(r.Turnover), OIUsd: num(r.OIValue), Funding: num(r.Funding), FundingH: h, Prev1h: num(r.Prev1h)}
	}
	return out, nil
}

// Klines returns the last limit candles, newest first (the forming bar is
// bars[0]), in the exchange's price units. interval is "1", "60" or "D" (UTC days).
func (b *BybitClient) Klines(ctx context.Context, symbol, interval string, limit int) ([]metrics.Bar, error) {
	q := url.Values{"category": {"linear"}, "symbol": {symbol}, "interval": {interval}, "limit": {strconv.Itoa(limit)}}
	list, _, err := bybitGet[[]string](ctx, b.g, "/v5/market/kline", q)
	if err != nil {
		return nil, err
	}
	out := make([]metrics.Bar, 0, len(list))
	for _, k := range list {
		if len(k) < 7 {
			return nil, fmt.Errorf("kline %s: short row", symbol)
		}
		out = append(out, metrics.Bar{TS: int64(num(k[0])), O: num(k[1]), H: num(k[2]), L: num(k[3]), C: num(k[4]), QuoteVol: num(k[6])})
	}
	return out, nil
}
