package venue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Public market-data streams of Binance and Bybit.
//
// REST polling alone moves a price once per Config.TickersEvery (5 s), and the
// exchange a row takes its price from - the busiest one - is Binance for most
// assets. The streams bring both exchanges to the one-second cadence of the
// OKX collector. REST stays as the safety net: while a stream is healthy it
// only reconciles (Config.ReconcileEvery), without one it is the feed again.
const (
	// BinanceWSURL carries, for every symbol, the 24 h ticker once a second
	// (only the symbols that traded in that second) and the mark price with
	// the funding rate once a second. Market streams live under /market: the
	// plain /ws and /stream paths accept a connection and stay silent
	// (checked 2026-11-10). The server pings at the protocol level and the
	// library answers; a connection lasts at most 24 h.
	BinanceWSURL = "wss://fstream.binance.com/market/stream?streams=!ticker@arr/!markPrice@arr@1s"
	// BybitWSURL takes one topic per instrument, tickers.<symbol>: a snapshot
	// first, then deltas with only the fields that changed (nine in ten carry
	// just the best bid and ask). It wants {"op":"ping"} every 20 s.
	BybitWSURL = "wss://stream.bybit.com/v5/public/linear"

	streamIdle     = 30 * time.Second // no frame for this long: the socket is dead, reconnect
	streamFreshFor = 5 * time.Second  // a stream that spoke this recently is the feed; past that REST takes over
	bybitArgBudget = 12000            // characters of topic names per connection; Bybit caps a connection at 21,000
	bybitSubChunk  = 200              // topics per subscribe request
)

// errIdle is returned by a feed's open when it has nothing to subscribe to.
var errIdle = errors.New("nothing to subscribe")

// StreamHealth is the state of one exchange's stream in /health.
type StreamHealth struct {
	Live       bool   `json:"live"`                 // every connection is up
	Conns      int    `json:"conns"`                // connections the exchange's symbols need
	Subscribed int    `json:"subscribed,omitempty"` // topics asked for (Bybit; Binance streams carry every symbol)
	Msgs       uint64 `json:"msgs"`                 // frames received
	Updates    uint64 `json:"updates"`              // ticker rows applied
	LastMsgAt  int64  `json:"lastMsgAt"`
	Reconnects uint64 `json:"reconnects"`
	LastErr    string `json:"lastErr,omitempty"`
}

// feed keeps one public WebSocket open: it dials, subscribes, hands every
// frame to msg and reconnects with a backoff.
type feed struct {
	name string
	url  string
	log  *slog.Logger
	// open runs on every connect before reading and sends the subscriptions; nil when the URL carries them.
	open func(send func(any) error) error
	// msg handles one frame and reports how many ticker rows it applied.
	msg func(data []byte) int
	// heartbeat, when set, is sent every 20 s.
	heartbeat any

	mu sync.Mutex
	st StreamHealth
	ws *websocket.Conn
}

func (f *feed) health() StreamHealth {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.st
}

// fresh reports whether the socket is up and spoke within maxAge.
func (f *feed) fresh(maxAge time.Duration) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.st.Live && f.st.LastMsgAt > 0 && time.Since(time.UnixMilli(f.st.LastMsgAt)) < maxAge
}

// restart drops the current connection; run reconnects and subscribes anew.
func (f *feed) restart() {
	f.mu.Lock()
	ws := f.ws
	f.mu.Unlock()
	if ws != nil {
		_ = ws.Close()
	}
}

// run connects until ctx ends.
func (f *feed) run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		started := time.Now()
		err := f.session(ctx)
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, errIdle) { // no symbols for this connection now; look again later
			if !sleep(ctx, 30*time.Second) {
				return
			}
			continue
		}
		f.mu.Lock()
		f.st.Live, f.ws = false, nil
		f.st.Reconnects++
		if err != nil {
			f.st.LastErr = err.Error()
		}
		f.mu.Unlock()
		if time.Since(started) > time.Minute {
			backoff = time.Second // the session held: this is a fresh failure, not a repeated one
		}
		f.log.Warn("venue stream ended", "stream", f.name, "err", err, "retryIn", backoff)
		if !sleep(ctx, backoff) {
			return
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (f *feed) session(ctx context.Context) error {
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	ws, _, err := dialer.DialContext(ctx, f.url, nil)
	if err != nil {
		return err
	}
	defer ws.Close()
	var wmu sync.Mutex
	send := func(v any) error {
		wmu.Lock()
		defer wmu.Unlock()
		_ = ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return ws.WriteJSON(v)
	}
	if f.open != nil {
		if err := f.open(send); err != nil {
			return err
		}
	}
	f.mu.Lock()
	f.ws, f.st.Live = ws, true
	f.mu.Unlock()
	f.log.Info("venue stream connected", "stream", f.name)

	errc := make(chan error, 1)
	go func() {
		for {
			_ = ws.SetReadDeadline(time.Now().Add(streamIdle))
			_, data, err := ws.ReadMessage()
			if err != nil {
				errc <- err
				return
			}
			n := f.msg(data)
			f.mu.Lock()
			f.st.Msgs++
			f.st.Updates += uint64(n)
			f.st.LastMsgAt = time.Now().UnixMilli()
			f.mu.Unlock()
		}
	}()
	var beat <-chan time.Time
	if f.heartbeat != nil {
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		beat = t.C
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errc:
			return err
		case <-beat:
			if err := send(f.heartbeat); err != nil {
				return err
			}
		}
	}
}

// startStreams opens the streams of both exchanges. Run calls it unless
// Config.NoStreams is set.
func (s *Service) startStreams(ctx context.Context) {
	bn := &feed{name: "binance", url: s.cfg.BinanceWS, log: s.log, msg: s.onBinanceWS}
	s.mu.Lock()
	s.runCtx, s.bnFeed = ctx, bn
	s.mu.Unlock()
	go bn.run(ctx)
	s.planBybit()
}

// bybitGroups splits the sorted symbols into runs whose topic names fit one
// connection.
func bybitGroups(syms []string, budget int) [][]string {
	var out [][]string
	start, used := 0, 0
	for i, sym := range syms {
		n := len("tickers.") + len(sym)
		if used+n > budget && i > start {
			out = append(out, syms[start:i])
			start, used = i, 0
		}
		used += n
	}
	if start < len(syms) {
		out = append(out, syms[start:])
	}
	return out
}

func sameGroups(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				return false
			}
		}
	}
	return true
}

// planBybit spreads Bybit's symbols over as many connections as their topic
// names need. When the set changes (a listing, a delisting) the connections
// are dropped and subscribe anew; REST covers the second that takes.
func (s *Service) planBybit() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runCtx == nil {
		return // streams are off, or Run has not started them yet
	}
	syms := make([]string, 0, len(s.inst[Bybit]))
	for sym := range s.inst[Bybit] {
		syms = append(syms, sym)
	}
	sort.Strings(syms)
	groups := bybitGroups(syms, bybitArgBudget)
	if sameGroups(groups, s.bbGroups) {
		return
	}
	s.bbGroups = groups
	for i := len(s.bbFeeds); i < len(groups); i++ {
		i := i
		f := &feed{name: fmt.Sprintf("bybit.%d", i), url: s.cfg.BybitWS, log: s.log, msg: s.onBybitWS, heartbeat: map[string]string{"op": "ping"}}
		f.open = func(send func(any) error) error { return s.subscribeBybit(i, send) }
		s.bbFeeds = append(s.bbFeeds, f)
		go f.run(s.runCtx)
	}
	for _, f := range s.bbFeeds {
		f.restart()
	}
}

func (s *Service) subscribeBybit(i int, send func(any) error) error {
	s.mu.RLock()
	var syms []string
	if i < len(s.bbGroups) {
		syms = s.bbGroups[i]
	}
	s.mu.RUnlock()
	if len(syms) == 0 {
		return errIdle
	}
	for a := 0; a < len(syms); a += bybitSubChunk {
		b := min(a+bybitSubChunk, len(syms))
		args := make([]string, 0, b-a)
		for _, sym := range syms[a:b] {
			args = append(args, "tickers."+sym)
		}
		if err := send(map[string]any{"op": "subscribe", "args": args}); err != nil {
			return err
		}
	}
	return nil
}

// streamFresh reports whether an exchange's stream is the live feed now.
func (s *Service) streamFresh(venue string) bool {
	s.mu.RLock()
	bn, feeds, groups := s.bnFeed, s.bbFeeds, len(s.bbGroups)
	s.mu.RUnlock()
	switch venue {
	case Binance:
		return bn != nil && bn.fresh(streamFreshFor)
	case Bybit:
		if groups == 0 || len(feeds) < groups {
			return false
		}
		for _, f := range feeds[:groups] {
			if !f.fresh(streamFreshFor) {
				return false
			}
		}
		return true
	}
	return false
}

// restDue reports whether a REST poll should go out now: always without a
// healthy stream, and only to reconcile with one.
func (s *Service) restDue(name, venue string) bool {
	if !s.streamFresh(venue) {
		return true
	}
	s.mu.RLock()
	at := s.restAt[name]
	s.mu.RUnlock()
	return time.Since(at) >= s.cfg.ReconcileEvery
}

// streamHealth is the /health block; the caller holds s.mu.
func (s *Service) streamHealth() map[string]StreamHealth {
	if s.bnFeed == nil {
		return nil
	}
	bn := s.bnFeed.health()
	bn.Conns = 1
	bb := StreamHealth{Conns: len(s.bbGroups), Live: len(s.bbGroups) > 0 && len(s.bbFeeds) >= len(s.bbGroups)}
	for _, g := range s.bbGroups {
		bb.Subscribed += len(g)
	}
	for i, f := range s.bbFeeds {
		h := f.health()
		bb.Msgs += h.Msgs
		bb.Updates += h.Updates
		bb.Reconnects += h.Reconnects
		if h.LastMsgAt > bb.LastMsgAt {
			bb.LastMsgAt = h.LastMsgAt
		}
		if h.LastErr != "" {
			bb.LastErr = h.LastErr
		}
		if i < len(s.bbGroups) && !h.Live {
			bb.Live = false
		}
	}
	return map[string]StreamHealth{Binance: bn, Bybit: bb}
}

// Binance rows. Its keys differ only by case (c / C, p / P, q / Q), and
// encoding/json falls back to a case-insensitive match, so every such pair
// gets a field of its own: otherwise the close time would land in the price.
type bnWSTicker struct {
	Symbol string          `json:"s"`
	Last   string          `json:"c"`
	Pct    string          `json:"P"` // 24 h change, percent
	Quote  string          `json:"q"` // 24 h volume, quote currency
	XC     json.RawMessage `json:"C"`
	Xp     json.RawMessage `json:"p"`
	XQ     json.RawMessage `json:"Q"`
}

type bnWSMark struct {
	Symbol string          `json:"s"`
	Mark   string          `json:"p"`
	Rate   string          `json:"r"`
	XP     json.RawMessage `json:"P"`
}

func (s *Service) onBinanceWS(data []byte) int {
	var m struct {
		Stream string          `json:"stream"`
		Data   json.RawMessage `json:"data"`
	}
	if json.Unmarshal(data, &m) != nil || len(m.Data) == 0 {
		return 0
	}
	switch {
	case strings.HasPrefix(m.Stream, "!ticker"):
		var rows []bnWSTicker
		if json.Unmarshal(m.Data, &rows) != nil {
			return 0
		}
		n := 0
		s.mu.Lock()
		for _, r := range rows {
			last := num(r.Last)
			if !(last > 0) {
				continue
			}
			t, ok := s.tick[Binance][r.Symbol]
			if !ok {
				t = Ticker{Symbol: r.Symbol, Mark: nan, OIUsd: nan, Funding: nan, Prev1h: nan}
			}
			t.Last, t.Chg24, t.Vol24 = last, num(r.Pct)/100, num(r.Quote)
			s.tick[Binance][r.Symbol] = t
			n++
		}
		s.mu.Unlock()
		return n
	case strings.HasPrefix(m.Stream, "!markPrice"):
		var rows []bnWSMark
		if json.Unmarshal(m.Data, &rows) != nil {
			return 0
		}
		s.mu.Lock()
		for _, r := range rows {
			if mark := num(r.Mark); mark > 0 {
				s.premium[r.Symbol] = Premium{Mark: mark, Funding: num(r.Rate)}
			}
		}
		s.mu.Unlock()
	}
	return 0
}

type bbWSMsg struct {
	Topic   string `json:"topic"`
	Type    string `json:"type"` // snapshot | delta
	Op      string `json:"op"`   // set on answers to subscribe and ping
	Success *bool  `json:"success"`
	RetMsg  string `json:"ret_msg"`
	Data    struct {
		Symbol   string  `json:"symbol"`
		Last     *string `json:"lastPrice"`
		Mark     *string `json:"markPrice"`
		Chg      *string `json:"price24hPcnt"`
		Turnover *string `json:"turnover24h"`
		OIValue  *string `json:"openInterestValue"`
		Funding  *string `json:"fundingRate"`
		FundingH *string `json:"fundingIntervalHour"`
		Prev1h   *string `json:"prevPrice1h"`
	} `json:"data"`
}

func (s *Service) onBybitWS(data []byte) int {
	var m bbWSMsg
	if json.Unmarshal(data, &m) != nil {
		return 0
	}
	if m.Op != "" {
		if m.Success != nil && !*m.Success {
			s.log.Warn("venue stream refused", "stream", "bybit", "op", m.Op, "msg", m.RetMsg)
		}
		return 0
	}
	d := &m.Data
	if d.Symbol == "" || !strings.HasPrefix(m.Topic, "tickers.") {
		return 0
	}
	snapshot := m.Type == "snapshot"
	if !snapshot && d.Last == nil && d.Mark == nil && d.Chg == nil && d.Turnover == nil && d.OIValue == nil && d.Funding == nil && d.FundingH == nil && d.Prev1h == nil {
		return 0 // best bid and ask only
	}
	set := func(dst *float64, src *string) {
		if src != nil {
			*dst = num(*src)
		}
	}
	s.mu.Lock()
	t, ok := s.tick[Bybit][d.Symbol]
	if !ok || snapshot {
		t = Ticker{Symbol: d.Symbol, Last: nan, Mark: nan, Chg24: nan, Vol24: nan, OIUsd: nan, Funding: nan, FundingH: t.FundingH, Prev1h: nan}
	}
	set(&t.Last, d.Last)
	set(&t.Mark, d.Mark)
	set(&t.Chg24, d.Chg)
	set(&t.Vol24, d.Turnover)
	set(&t.OIUsd, d.OIValue)
	set(&t.Funding, d.Funding)
	set(&t.Prev1h, d.Prev1h)
	if d.FundingH != nil {
		if h := num(*d.FundingH); !math.IsNaN(h) {
			t.FundingH = h
		}
	}
	s.tick[Bybit][d.Symbol] = t
	s.mu.Unlock()
	return 1
}
