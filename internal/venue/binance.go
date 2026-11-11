package venue

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strconv"

	"screener/internal/metrics"
)

// BinanceClient reads the USDⓈ-M futures public REST API.
//
// Request weights (limit 2400 per minute per IP): ticker/24hr for all symbols
// 40, premiumIndex for all 10, openInterest 1 per symbol, klines 1–5 by limit.
// The service keeps well under that: see Config.
type BinanceClient struct {
	g    getter // bulk endpoints
	slow getter // per-symbol endpoints (open interest, candles)
}

// NewBinance builds the client; base "" is the production API.
func NewBinance(base string, perSymbolPerSec int) *BinanceClient {
	if base == "" {
		base = "https://fapi.binance.com"
	}
	if perSymbolPerSec < 1 {
		perSymbolPerSec = 4
	}
	return &BinanceClient{g: newGetter(base, 4), slow: newGetter(base, perSymbolPerSec)}
}

// Instruments lists the trading linear perpetuals, crypto and TradFi.
func (b *BinanceClient) Instruments(ctx context.Context) ([]Instrument, error) {
	var info struct {
		Symbols []struct {
			Symbol            string   `json:"symbol"`
			ContractType      string   `json:"contractType"`
			Status            string   `json:"status"`
			BaseAsset         string   `json:"baseAsset"`
			QuoteAsset        string   `json:"quoteAsset"`
			UnderlyingType    string   `json:"underlyingType"`
			UnderlyingSubType []string `json:"underlyingSubType"`
		} `json:"symbols"`
	}
	if err := b.g.get(ctx, "/fapi/v1/exchangeInfo", nil, &info); err != nil {
		return nil, err
	}
	// Funding intervals: only symbols that differ from the 8 h default are listed.
	var fi []struct {
		Symbol string  `json:"symbol"`
		Hours  float64 `json:"fundingIntervalHours"`
	}
	hours := map[string]float64{}
	if err := b.g.get(ctx, "/fapi/v1/fundingInfo", nil, &fi); err == nil {
		for _, f := range fi {
			hours[f.Symbol] = f.Hours
		}
	}
	out := make([]Instrument, 0, len(info.Symbols))
	for _, s := range info.Symbols {
		if s.Status != "TRADING" || (s.ContractType != "PERPETUAL" && s.ContractType != "TRADIFI_PERPETUAL") {
			continue
		}
		if s.QuoteAsset != "USDT" && s.QuoteAsset != "USDC" {
			continue
		}
		class, kind := binanceClass(s.UnderlyingType)
		base, mult := Canonical(class, s.BaseAsset)
		h := hours[s.Symbol]
		if h <= 0 {
			h = 8
		}
		out = append(out, Instrument{Venue: Binance, Symbol: s.Symbol, Base: base, Mult: mult, Quote: s.QuoteAsset, Class: class, Kind: kind, Tags: s.UnderlyingSubType, FundingH: h})
	}
	return out, nil
}

// binanceClass maps Binance's underlyingType. A crypto index (BTCDOM) stays
// with the coins; regional equity types are all stocks.
func binanceClass(t string) (class, kind string) {
	switch t {
	case "", "COIN":
		return ClassCrypto, ""
	case "INDEX":
		return ClassCrypto, "Index"
	case "COMMODITY":
		return ClassCommodity, "Commodity"
	case "FX":
		return ClassFX, "FX"
	case "PREMARKET":
		return ClassStock, "Pre-IPO"
	}
	return ClassStock, "Stock"
}

// Tickers returns the 24 h tickers of every symbol.
func (b *BinanceClient) Tickers(ctx context.Context) (map[string]Ticker, error) {
	var raw []struct {
		Symbol string `json:"symbol"`
		Last   string `json:"lastPrice"`
		ChgPct string `json:"priceChangePercent"`
		Quote  string `json:"quoteVolume"`
	}
	if err := b.g.get(ctx, "/fapi/v1/ticker/24hr", nil, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]Ticker, len(raw))
	nan := math.NaN()
	for _, r := range raw {
		out[r.Symbol] = Ticker{Symbol: r.Symbol, Last: num(r.Last), Mark: nan, Chg24: num(r.ChgPct) / 100, Vol24: num(r.Quote), OIUsd: nan, Funding: nan, Prev1h: nan}
	}
	return out, nil
}

// Premium is the mark price and the current funding rate of one symbol.
type Premium struct{ Mark, Funding float64 }

// Premiums returns mark price and funding rate of every symbol.
func (b *BinanceClient) Premiums(ctx context.Context) (map[string]Premium, error) {
	var raw []struct {
		Symbol  string `json:"symbol"`
		Mark    string `json:"markPrice"`
		Funding string `json:"lastFundingRate"`
	}
	if err := b.g.get(ctx, "/fapi/v1/premiumIndex", nil, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]Premium, len(raw))
	for _, r := range raw {
		out[r.Symbol] = Premium{Mark: num(r.Mark), Funding: num(r.Funding)}
	}
	return out, nil
}

// OpenInterest returns the open interest of one symbol in contracts of that
// symbol (multiply by its mark price for USD). Binance has no bulk endpoint.
func (b *BinanceClient) OpenInterest(ctx context.Context, symbol string) (float64, error) {
	var raw struct {
		OI string `json:"openInterest"`
	}
	if err := b.slow.get(ctx, "/fapi/v1/openInterest", url.Values{"symbol": {symbol}}, &raw); err != nil {
		return math.NaN(), err
	}
	return num(raw.OI), nil
}

// Klines returns the last limit candles, newest first (the forming bar is
// bars[0]), in the exchange's price units. interval is "1m", "1h" or "1d" (UTC days).
func (b *BinanceClient) Klines(ctx context.Context, symbol, interval string, limit int) ([]metrics.Bar, error) {
	var raw [][]any
	q := url.Values{"symbol": {symbol}, "interval": {interval}, "limit": {strconv.Itoa(limit)}}
	if err := b.slow.get(ctx, "/fapi/v1/klines", q, &raw); err != nil {
		return nil, err
	}
	out := make([]metrics.Bar, 0, len(raw))
	for i := len(raw) - 1; i >= 0; i-- {
		k := raw[i]
		if len(k) < 8 {
			return nil, fmt.Errorf("klines %s: short row", symbol)
		}
		out = append(out, metrics.Bar{TS: int64(anyNum(k[0])), O: anyNum(k[1]), H: anyNum(k[2]), L: anyNum(k[3]), C: anyNum(k[4]), QuoteVol: anyNum(k[7])})
	}
	return out, nil
}
