package venue

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestBybitGroups(t *testing.T) {
	syms := []string{"AAAUSDT", "BBBUSDT", "CCCUSDT", "DDDUSDT", "EEEUSDT"} // a topic name is 15 characters
	if got := fmt.Sprint(bybitGroups(syms, 30)); got != "[[AAAUSDT BBBUSDT] [CCCUSDT DDDUSDT] [EEEUSDT]]" {
		t.Errorf("two topics per connection: %s", got)
	}
	if got := bybitGroups(syms, 1000); len(got) != 1 || len(got[0]) != 5 {
		t.Errorf("one connection is enough: %v", got)
	}
	if got := fmt.Sprint(bybitGroups(syms[:2], 10)); got != "[[AAAUSDT] [BBBUSDT]]" {
		t.Errorf("a topic longer than the budget still gets a connection: %s", got)
	}
	if got := bybitGroups(nil, 30); len(got) != 0 {
		t.Errorf("no symbols, no connections: %v", got)
	}
	if !sameGroups(bybitGroups(syms, 30), bybitGroups(syms, 30)) || sameGroups(bybitGroups(syms, 30), bybitGroups(syms[:4], 30)) {
		t.Error("sameGroups")
	}
}

// streams serves a fake of each exchange's WebSocket. Bybit's first connection
// is dropped when drop closes; the second one sends a newer snapshot.
func streams(t *testing.T, drop <-chan struct{}) (url string, topics func() []string) {
	t.Helper()
	up := websocket.Upgrader{}
	var mu sync.Mutex
	var subs []string
	conns := 0
	text := func(c *websocket.Conn, s string) { _ = c.WriteMessage(websocket.TextMessage, []byte(s)) }
	hold := func(c *websocket.Conn) {
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/bn", func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		// The full key set of a real ticker row: the upper-case twins (C close time, p absolute change, Q last
		// quantity) must not leak into the lower-case fields.
		text(c, `{"stream":"!ticker@arr","data":[`+
			`{"e":"24hrTicker","E":1,"s":"BTCUSDT","ps":"BTCUSDT","p":"1.5","P":"2.5","w":"100","c":"101","Q":"7","o":"99","h":"102","l":"98","v":"30","q":"3000","O":2,"C":3,"F":4,"L":5,"n":6,"st":1},`+
			`{"e":"24hrTicker","E":1,"s":"NEWUSDT","p":"0","P":"1","c":"5","Q":"1","q":"9","C":3}]}`)
		text(c, `{"stream":"!markPrice@arr@1s","data":[{"e":"markPriceUpdate","E":1,"s":"BTCUSDT","p":"101.5","ap":"101.5","P":"100.9","i":"101.4","r":"0.0003","T":9,"st":1}]}`)
		hold(c)
	})
	mux.HandleFunc("/bb", func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		var sub struct {
			Op   string   `json:"op"`
			Args []string `json:"args"`
		}
		if err := c.ReadJSON(&sub); err != nil || sub.Op != "subscribe" {
			return
		}
		mu.Lock()
		conns++
		n := conns
		subs = append([]string(nil), sub.Args...)
		mu.Unlock()
		text(c, `{"success":true,"ret_msg":"","op":"subscribe"}`)
		if n == 1 {
			text(c, `{"topic":"tickers.BTCUSDT","type":"snapshot","data":{"symbol":"BTCUSDT","lastPrice":"100.5","markPrice":"100.6","price24hPcnt":"0.03","turnover24h":"710","openInterestValue":"910","fundingRate":"0.00007","fundingIntervalHour":"4","prevPrice1h":"99.5","bid1Price":"100.4"}}`)
			text(c, `{"topic":"tickers.BTCUSDT","type":"delta","data":{"symbol":"BTCUSDT","bid1Price":"100.6","ask1Price":"100.8"}}`)
			text(c, `{"topic":"tickers.BTCUSDT","type":"delta","data":{"symbol":"BTCUSDT","lastPrice":"100.7"}}`)
			<-drop
			return
		}
		text(c, `{"topic":"tickers.BTCUSDT","type":"snapshot","data":{"symbol":"BTCUSDT","lastPrice":"100.9","markPrice":"100.9","price24hPcnt":"0.04","turnover24h":"720","openInterestValue":"920","fundingRate":"0.00008","fundingIntervalHour":"4","prevPrice1h":"99.6"}}`)
		hold(c)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http"), func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := append([]string(nil), subs...)
		sort.Strings(out)
		return out
	}
}

func TestStreamsCarryThePrice(t *testing.T) {
	rest := exchanges(t)
	drop := make(chan struct{})
	var once sync.Once
	dropNow := func() { once.Do(func() { close(drop) }) }
	t.Cleanup(dropNow)
	ws, topics := streams(t, drop)

	rows := fakeRows{okxRow("BTC-USDT-SWAP", "BTC", "Bitcoin", 100.1, 0.015, 800, 600, 0.0001)}
	rows[0].Class = "crypto"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := New(Config{BinanceURL: rest.URL, BybitURL: rest.URL, BinancePerSec: 50, BybitPerSec: 50, BinanceWS: ws + "/bn", BybitWS: ws + "/bb"}, rows, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.Prime(ctx)
	if !s.restDue("binance.tickers", Binance) || !s.restDue("bybit.tickers", Bybit) {
		t.Fatal("without a stream REST is the feed")
	}
	for name, fn := range map[string]func(context.Context) error{"binance": s.pollBinance, "premium": s.pollPremium, "bybit": s.pollBybit} {
		if err := fn(ctx); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	asset := func(key string) Asset {
		for _, a := range s.build(time.Now()) {
			if a.K == key {
				return a
			}
		}
		t.Fatalf("no asset %s", key)
		return Asset{}
	}
	wait := func(what string, cond func() bool) {
		t.Helper()
		for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
			if cond() {
				return
			}
		}
		t.Fatalf("timed out: %s (BTC %+v, health %+v)", what, asset("c:BTC").V, s.Health().Streams)
	}

	s.startStreams(ctx)
	wait("stream prices on both exchanges", func() bool {
		v := asset("c:BTC").V
		return near(float64(v[Binance].P), 101) && near(float64(v[Binance].F), 0.0003) && near(float64(v[Bybit].P), 100.7)
	})
	btc := asset("c:BTC").V
	if bn := btc[Binance]; !near(float64(bn.C), 0.025) || !near(float64(bn.Vol), 3000+500) {
		t.Errorf("Binance: change is P (percent), volume is q; the USDC contract keeps its REST figures: %+v", bn)
	}
	if bb := btc[Bybit]; !near(float64(bb.C), 0.03) || !near(float64(bb.Vol), 710) || !near(float64(bb.OI), 910) || !near(float64(bb.F), 0.00007) || bb.FI != 4 {
		t.Errorf("Bybit: a delta changes only its own fields, the rest stay from the snapshot: %+v", bb)
	}
	if pepe := asset("c:PEPE").V; !near(float64(pepe[Binance].P), 0.000005) || !near(float64(pepe[Bybit].P), 0.0000052) {
		t.Errorf("symbols the streams did not mention keep their REST figures: %+v", pepe)
	}
	if got := fmt.Sprint(topics()); got != "[tickers.1000PEPEUSDT tickers.AMDSTOCKUSDT tickers.BTCUSDT tickers.SHIB1000USDT]" {
		t.Errorf("Bybit topics: %s", got)
	}
	if s.restDue("binance.tickers", Binance) || s.restDue("binance.premium", Binance) || s.restDue("bybit.tickers", Bybit) {
		t.Error("with healthy streams REST only reconciles, once per ReconcileEvery")
	}
	h := s.Health().Streams
	if bn := h[Binance]; !bn.Live || bn.Conns != 1 || bn.Msgs != 2 || bn.Updates != 2 {
		t.Errorf("health, Binance: %+v", bn)
	}
	if bb := h[Bybit]; !bb.Live || bb.Conns != 1 || bb.Subscribed != 4 || bb.Updates != 2 || bb.Reconnects != 0 {
		t.Errorf("health, Bybit: %+v", bb)
	}

	// The exchange drops the connection: the feed comes back by itself and subscribes again.
	dropNow()
	wait("Bybit reconnects and takes the new snapshot", func() bool { return near(float64(asset("c:BTC").V[Bybit].P), 100.9) })
	if bb := asset("c:BTC").V[Bybit]; !near(float64(bb.OI), 920) || !near(float64(bb.Vol), 720) {
		t.Errorf("Bybit after the reconnect: %+v", bb)
	}
	if bb := s.Health().Streams[Bybit]; bb.Reconnects != 1 || !bb.Live {
		t.Errorf("health after the reconnect: %+v", bb)
	}

	// A stream that went quiet hands the feed back to REST.
	s.bnFeed.mu.Lock()
	s.bnFeed.st.LastMsgAt = time.Now().Add(-2 * streamFreshFor).UnixMilli()
	s.bnFeed.mu.Unlock()
	if !s.restDue("binance.tickers", Binance) {
		t.Error("a silent stream must put REST back to work")
	}
}

func TestStreamsOff(t *testing.T) {
	rest := exchanges(t)
	s := New(Config{BinanceURL: rest.URL, BybitURL: rest.URL, NoStreams: true}, fakeRows{}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.Prime(context.Background())
	if s.Health().Streams != nil || !s.restDue("bybit.tickers", Bybit) {
		t.Error("with the streams off there is no stream block and REST is always due")
	}
}
