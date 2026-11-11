package venue

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"screener/internal/collector"
)

type fakeRows []collector.Row

func (f fakeRows) Rows() ([]collector.Row, time.Time) { return f, time.Now() }

func okxRow(id, sym, class string, last, chg, vol, oi, funding float64) collector.Row {
	return collector.Row{InstID: id, Symbol: sym, Settle: "USDT", Class: "", Last: collector.F(last), Vol24: collector.F(vol), OI: collector.F(oi),
		Funding: collector.F(funding), FundingInterval: 8, ChgRoll: collector.F(chg), Chg: collector.PerPeriod{"1h": 0.001, "4h": 0.002, "24h": collector.F(chg) + 0.5},
		DO: collector.Level{Px: collector.F(last * 0.99)}, Name: class}
}

// exchanges serves the endpoints of both APIs with a small fixed universe.
func exchanges(t *testing.T) *httptest.Server {
	t.Helper()
	hourStart := time.Now().Truncate(time.Hour).UnixMilli()
	dayStart := time.Now().UTC().Truncate(24 * time.Hour).UnixMilli()
	// i bars back (0 = forming): close falls 0.1% per bar going back, open of the day's first bar is 1.9
	px := func(i int) float64 { return 2 * (1 - 0.001*float64(i)) }
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }
	type m = map[string]any

	mux.HandleFunc("/fapi/v1/exchangeInfo", func(w http.ResponseWriter, r *http.Request) {
		sym := func(s, ct, status, base, quote, ut string, tags ...string) m {
			return m{"symbol": s, "contractType": ct, "status": status, "baseAsset": base, "quoteAsset": quote, "underlyingType": ut, "underlyingSubType": tags}
		}
		write(w, m{"symbols": []m{
			sym("BTCUSDT", "PERPETUAL", "TRADING", "BTC", "USDT", "COIN", "PoW"),
			sym("BTCUSDC", "PERPETUAL", "TRADING", "BTC", "USDC", "COIN"),
			sym("1000PEPEUSDT", "PERPETUAL", "TRADING", "1000PEPE", "USDT", "COIN", "Meme"),
			sym("NATGASUSDT", "TRADIFI_PERPETUAL", "TRADING", "NATGAS", "USDT", "COMMODITY", "TradFi"),
			sym("XYZUSDT", "PERPETUAL", "TRADING", "XYZ", "USDT", "COIN", "Layer-1"),
			sym("OLDUSDT", "PERPETUAL", "SETTLING", "OLD", "USDT", "COIN"),
			sym("BTCUSDT_260327", "CURRENT_QUARTER", "TRADING", "BTC", "USDT", "COIN"),
			sym("ETHBTC", "PERPETUAL", "TRADING", "ETH", "BTC", "COIN"),
		}})
	})
	mux.HandleFunc("/fapi/v1/fundingInfo", func(w http.ResponseWriter, r *http.Request) {
		write(w, []m{{"symbol": "NATGASUSDT", "fundingIntervalHours": 4}})
	})
	mux.HandleFunc("/fapi/v1/ticker/24hr", func(w http.ResponseWriter, r *http.Request) {
		tk := func(s, last, pct, qv string) m {
			return m{"symbol": s, "lastPrice": last, "priceChangePercent": pct, "quoteVolume": qv}
		}
		write(w, []m{tk("BTCUSDT", "100", "1.0", "1000"), tk("BTCUSDC", "99", "0.9", "500"), tk("1000PEPEUSDT", "0.005", "4", "200"),
			tk("NATGASUSDT", "3.4", "8", "50"), tk("XYZUSDT", "2", "-3", "10"), tk("OLDUSDT", "1", "0", "1")})
	})
	mux.HandleFunc("/fapi/v1/premiumIndex", func(w http.ResponseWriter, r *http.Request) {
		p := func(s, mark, fr string) m { return m{"symbol": s, "markPrice": mark, "lastFundingRate": fr} }
		write(w, []m{p("BTCUSDT", "100.5", "0.0001"), p("BTCUSDC", "99.5", "0.0002"), p("1000PEPEUSDT", "0.0051", "0.0002"), p("NATGASUSDT", "3.41", "0.004"), p("XYZUSDT", "2", "-0.0001")})
	})
	mux.HandleFunc("/fapi/v1/openInterest", func(w http.ResponseWriter, r *http.Request) {
		oi := map[string]string{"BTCUSDT": "10", "BTCUSDC": "2", "1000PEPEUSDT": "1000000"}[r.URL.Query().Get("symbol")]
		if oi == "" {
			http.Error(w, `{"code":-4108,"msg":"no data"}`, http.StatusBadRequest)
			return
		}
		write(w, m{"openInterest": oi})
	})
	bars := func(r *http.Request, daily bool) (start, step int64, n int) {
		n, _ = strconv.Atoi(r.URL.Query().Get("limit"))
		if daily {
			return dayStart, 86_400_000, n
		}
		return hourStart, 3_600_000, n
	}
	mux.HandleFunc("/fapi/v1/klines", func(w http.ResponseWriter, r *http.Request) {
		start, step, n := bars(r, r.URL.Query().Get("interval") == "1d")
		out := make([][]any, 0, n)
		for i := n - 1; i >= 0; i-- { // oldest first
			c := fmt.Sprint(px(i))
			out = append(out, []any{start - int64(i)*step, "1.9", c, c, c, "5", start - int64(i)*step + step - 1, "10", 1, "2", "4", "0"})
		}
		write(w, out)
	})

	bybit := func(w http.ResponseWriter, list any) {
		write(w, m{"retCode": 0, "retMsg": "OK", "result": m{"list": list, "nextPageCursor": ""}})
	}
	mux.HandleFunc("/v5/market/instruments-info", func(w http.ResponseWriter, r *http.Request) {
		in := func(s, ct, base, quote, st string) m {
			return m{"symbol": s, "contractType": ct, "status": "Trading", "baseCoin": base, "quoteCoin": quote, "symbolType": st, "fundingInterval": 480}
		}
		bybit(w, []m{in("BTCUSDT", "LinearPerpetual", "BTC", "USDT", ""), in("1000PEPEUSDT", "LinearPerpetual", "1000PEPE", "USDT", ""),
			in("SHIB1000USDT", "LinearPerpetual", "SHIB1000", "USDT", "innovation"), in("AMDSTOCKUSDT", "LinearPerpetual", "AMDSTOCK", "USDT", "stock"),
			in("BTC-27MAR26", "LinearFutures", "BTC", "USDT", "")})
	})
	mux.HandleFunc("/v5/market/tickers", func(w http.ResponseWriter, r *http.Request) {
		tk := func(s, last, pct, turn, oi, fr, prev string) m {
			return m{"symbol": s, "lastPrice": last, "markPrice": last, "price24hPcnt": pct, "turnover24h": turn, "openInterestValue": oi, "fundingRate": fr, "fundingIntervalHour": "8", "prevPrice1h": prev}
		}
		bybit(w, []m{tk("BTCUSDT", "100.2", "0.02", "700", "900", "0.00005", "99"), tk("1000PEPEUSDT", "0.0052", "0.05", "100", "300", "0.0001", "0.0051"),
			tk("SHIB1000USDT", "2", "0.01", "40", "20", "", "1.99"), tk("AMDSTOCKUSDT", "500", "0.09", "30", "10", "0", "499")})
	})
	mux.HandleFunc("/v5/market/kline", func(w http.ResponseWriter, r *http.Request) {
		start, step, n := bars(r, r.URL.Query().Get("interval") == "D")
		out := make([][]string, 0, n)
		for i := 0; i < n; i++ { // newest first
			c := fmt.Sprint(px(i))
			out = append(out, []string{strconv.FormatInt(start-int64(i)*step, 10), "1.9", c, c, c, "5", "10"})
		}
		bybit(w, out)
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func near(a, b float64) bool { return math.Abs(a-b) <= 1e-9*math.Max(1, math.Abs(b)) }

func TestAssetsOverThreeExchanges(t *testing.T) {
	ts := exchanges(t)
	rows := fakeRows{
		okxRow("BTC-USDT-SWAP", "BTC", "Bitcoin", 100.1, 0.015, 800, 600, 0.0001),
		okxRow("NG-USDT-SWAP", "NG", "", 3.39, 0.08, 20, 5, 0.006),
		okxRow("AMD-USDT-SWAP", "AMD", "", 501, 0.09, 60, 8, 0),
	}
	rows[0].Class, rows[1].Class, rows[2].Class = "crypto", "commodity", "stock"
	ctx := context.Background()
	s := New(Config{BinanceURL: ts.URL, BybitURL: ts.URL, BinancePerSec: 50, BybitPerSec: 50}, rows, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.Prime(ctx)
	for name, fn := range map[string]func(context.Context) error{"binance": s.pollBinance, "premium": s.pollPremium, "bybit": s.pollBybit} {
		if err := fn(ctx); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for _, sym := range []string{"BTCUSDT", "BTCUSDC", "1000PEPEUSDT"} {
		v, err := s.bn.OpenInterest(ctx, sym)
		if err != nil {
			t.Fatalf("open interest %s: %v", sym, err)
		}
		s.bnOI[sym] = oiReading{contracts: v, at: time.Now()}
	}
	targets := s.candleTargets()
	keys := map[string]string{}
	for _, tg := range targets {
		keys[tg.key] = tg.inst.Venue + ":" + tg.inst.Symbol
		if err := s.refreshSeries(ctx, tg); err != nil {
			t.Fatalf("candles %s: %v", tg.key, err)
		}
	}
	// PEPE is on both Binance and Bybit and not on OKX: Bybit's candles stand for it; XYZ exists only on Binance.
	want := map[string]string{benchKey: "bb:BTCUSDT", "c:PEPE": "bb:1000PEPEUSDT", "c:SHIB": "bb:SHIB1000USDT", "c:XYZ": "bn:XYZUSDT"}
	if len(keys) != len(want) {
		t.Fatalf("candle targets: %v", keys)
	}
	for k, v := range want {
		if keys[k] != v {
			t.Errorf("candle target %s = %q, want %q", k, keys[k], v)
		}
	}

	list, _ := s.Assets()
	by := map[string]Asset{}
	for _, a := range list {
		by[a.K] = a
	}
	if len(list) != 6 {
		t.Fatalf("assets: got %d %v", len(list), keysOf(by))
	}
	if list[0].K != "c:BTC" {
		t.Errorf("busiest first: got %s", list[0].K)
	}

	btc := by["c:BTC"]
	if len(btc.V) != 3 {
		t.Fatalf("BTC exchanges: %v", btc.V)
	}
	if bn := btc.V[Binance]; !near(float64(bn.Vol), 1500) || !near(float64(bn.P), 100) || !near(float64(bn.C), 0.01) || !near(float64(bn.OI), 10*100.5+2*99.5) || !near(float64(bn.F), 0.0001) || bn.FI != 8 {
		t.Errorf("BTC on Binance (USDT + USDC contracts add up, the busier gives price and funding): %+v", bn)
	}
	if bb := btc.V[Bybit]; !near(float64(bb.OI), 900) || !near(float64(bb.P), 100.2) || !near(float64(bb.F), 0.00005) {
		t.Errorf("BTC on Bybit: %+v", bb)
	}
	if ok := btc.V[OKX]; !near(float64(ok.P), 100.1) || !near(float64(ok.Vol), 800) || !near(float64(ok.C), 0.015) {
		t.Errorf("BTC on OKX: %+v", ok)
	}
	if btc.Cls != ClassCrypto || !near(float64(btc.M.Chg["1h"]), 0.001) || !near(float64(btc.M.Lv["do"]), 100.1*0.99) {
		t.Errorf("BTC metrics come from the OKX row: cls %s m %+v", btc.Cls, btc.M)
	}

	pepe := by["c:PEPE"]
	if !near(float64(pepe.V[Binance].P), 0.000005) || !near(float64(pepe.V[Bybit].P), 0.0000052) || !near(float64(pepe.V[Binance].OI), 1_000_000*0.0051) {
		t.Errorf("PEPE: price per coin and OI at the contract's mark: %+v", pepe.V)
	}
	if pepe.Sec != "Meme" {
		t.Errorf("PEPE sector from the exchange tags: %q", pepe.Sec)
	}

	ng := by["t:NG"]
	if ng.Cls != ClassCommodity || ng.Sec != "Commodity" || len(ng.V) != 2 || ng.V[Binance].FI != 4 {
		t.Errorf("NG (NATGAS on Binance joins OKX's NG; funding every 4 h): %+v", ng)
	}
	amd := by["t:AMD"]
	if amd.Cls != ClassStock || len(amd.V) != 2 || !near(float64(amd.V[Bybit].P), 500) {
		t.Errorf("AMD (AMDSTOCK on Bybit joins OKX's AMD): %+v", amd)
	}

	// Outside OKX the metrics come from candles: close falls 0.1% per bar going back, last is 2.
	// SHIB1000 is quoted per thousand coins, so its candles are brought to one coin like its price.
	for k, unit := range map[string]float64{"c:SHIB": 0.001, "c:XYZ": 1} {
		a := by[k]
		if len(a.V) != 1 {
			t.Errorf("%s exchanges: %v", k, a.V)
		}
		if got, want := float64(a.M.Chg["1h"]), 2/(2*(1-0.001))-1; !near(got, want) {
			t.Errorf("%s chg 1h = %v, want %v", k, got, want)
		}
		if got, want := float64(a.M.Chg7d), 2/(2*(1-0.007))-1; !near(got, want) {
			t.Errorf("%s chg 7d = %v, want %v", k, got, want)
		}
		if len(a.M.Spark) != 24 || !near(float64(a.M.Spark[23]), 2*unit) {
			t.Errorf("%s spark: %v", k, a.M.Spark)
		}
		if !near(float64(a.M.Lv["do"]), 1.9*unit) || !(float64(a.M.Lv["ma200"]) > 0) {
			t.Errorf("%s levels: %v", k, a.M.Lv)
		}
		if !math.IsNaN(float64(a.M.Tape)) || !math.IsNaN(float64(a.M.Spread)) {
			t.Errorf("%s: flow and book need OKX, must stay empty", k)
		}
	}
	if by["c:XYZ"].Sec != "L1" || by["c:SHIB"].Sec != "Other" {
		t.Errorf("sectors: XYZ %q SHIB %q", by["c:XYZ"].Sec, by["c:SHIB"].Sec)
	}
	if got := s.CryptoBases(); fmt.Sprint(got) != "[BTC PEPE SHIB XYZ]" {
		t.Errorf("crypto bases: %v", got)
	}
	// NaN must serialise as null, not break the payload.
	if b, err := json.Marshal(list); err != nil || len(b) == 0 {
		t.Fatalf("marshal: %v", err)
	}
	h := s.Health()
	if h.Assets != 6 || h.OutsideOKX != 3 || h.Candles != 3 || h.Instruments[Binance] != 5 || h.Instruments[Bybit] != 4 {
		t.Errorf("health: %+v", h)
	}
}

func keysOf(m map[string]Asset) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
