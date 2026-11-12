package venue

import (
	"io"
	"log/slog"
	"testing"
)

// Contracts that share a ticker but not the price are not summed into one row.
func TestContractsApartByPrice(t *testing.T) {
	rows := fakeRows{
		okxRow("BTC-USDT-SWAP", "BTC", "Bitcoin", 100, 0.01, 800, 600, 0.0001),
		okxRow("ANTHROPIC-USDT-SWAP", "ANTHROPIC", "", 105, 0, 20, 10, 0),
	}
	rows[0].Class, rows[1].Class = "crypto", "premarket"
	s := New(Config{}, rows, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	add := func(venue, sym, base, class string, last, vol float64) {
		s.inst[venue][sym] = Instrument{Venue: venue, Symbol: sym, Base: base, Mult: 1, Quote: "USDT", Class: class, FundingH: 8}
		s.tick[venue][sym] = Ticker{Last: last, Vol24: vol, OIUsd: 5, Funding: 0.0001, FundingH: 8}
	}
	add(Binance, "BTCUSDT", "BTC", ClassCrypto, 101, 900)
	add(Bybit, "BTCUSDT", "BTC", ClassCrypto, 99, 500)
	add(Binance, "ANTHROPICUSDT", "ANTHROPIC", ClassStock, 1380, 120) // ten times OKX's unit
	add(Bybit, "ANTHROPICUSDT", "ANTHROPIC", ClassStock, 1390, 40)    // agrees with Binance
	add(Binance, "XYZUSDT", "XYZ", ClassCrypto, 2, 50)                // not on OKX: the busiest one leads
	add(Bybit, "XYZUSDT", "XYZ", ClassCrypto, 9, 10)

	list, _ := s.Assets()
	by := map[string]Asset{}
	for _, a := range list {
		by[a.K] = a
	}
	want := map[string][]string{
		"c:BTC":          {OKX, Binance, Bybit},
		"t:ANTHROPIC":    {OKX},
		"t:ANTHROPIC@bn": {Binance, Bybit},
		"c:XYZ":          {Binance},
		"c:XYZ@bb":       {Bybit},
	}
	if len(list) != len(want) {
		t.Fatalf("assets: %v", keysOf(by))
	}
	for k, venues := range want {
		a, ok := by[k]
		if !ok || len(a.V) != len(venues) {
			t.Errorf("%s: exchanges %v, want %v", k, a.V, venues)
			continue
		}
		for _, v := range venues {
			if _, ok := a.V[v]; !ok {
				t.Errorf("%s: no block of %s", k, v)
			}
		}
	}
	if a := by["t:ANTHROPIC@bn"]; a.S != "ANTHROPIC" || a.N != "Binance contract" {
		t.Errorf("split row: ticker %q, name %q", a.S, a.N)
	}

	// A split row gets candles of its own, also where OKX lists the ticker.
	keys := map[string]string{}
	for _, tg := range s.candleTargets() {
		keys[tg.key] = tg.inst.Venue + ":" + tg.inst.Symbol
	}
	for k, v := range map[string]string{"t:ANTHROPIC@bn": "bb:ANTHROPICUSDT", "c:XYZ": "bn:XYZUSDT", "c:XYZ@bb": "bb:XYZUSDT"} {
		if keys[k] != v {
			t.Errorf("candle target %s = %q, want %q", k, keys[k], v)
		}
	}
	if _, ok := keys["t:ANTHROPIC"]; ok {
		t.Errorf("OKX's own row needs no candles from elsewhere: %v", keys)
	}
}
