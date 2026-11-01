package okx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSURL is the OKX public WebSocket endpoint.
const WSURL = "wss://ws.okx.com:8443/ws/v5/public"

// WSTrade is one public trade from the `trades` channel. Sz is in contracts
// for swaps; Count is how many fills the venue aggregated into this message
// (OKX sends one message per taker order on the `trades` channel).
type WSTrade struct {
	InstID string
	TS     int64
	Px     float64
	Sz     float64
	Side   string // buy | sell (taker side)
	Count  int
}

// WSEvent is one parsed public-channel message. Exactly one slice is set,
// matching Channel.
type WSEvent struct {
	Channel string
	Trades  []WSTrade
	Tickers []Ticker
	Marks   []MarkPrice
	Index   []IndexTicker
	OI      []OpenInterest
	Funding []Funding
}

// Stream keeps a set of public channels open for a set of instruments over a
// few connections and hands every message to the handler. It reconnects on
// its own and re-subscribes; Set replaces the instrument set at any time.
//
// Venue limits respected here: one subscribe message carries many args
// (chunks of 150 here, well under the 64 KB frame cap), 480 subscribe
// requests per hour per connection is the cap; ping "ping" after 20 s of
// silence, the server answers "pong" and closes idle sockets after 30 s.
type Stream struct {
	url      string
	log      *slog.Logger
	handler  func(WSEvent)
	perConn  int
	channels []string

	mu      sync.Mutex
	want    []string // sorted
	conns   []*wsConn
	started time.Time
	stats   WSStats
}

// WSStats for /health.
type WSStats struct {
	Connections int               `json:"connections"`
	Live        int               `json:"live"`
	Instruments int               `json:"instruments"`
	Msgs        uint64            `json:"msgs"`
	Trades      uint64            `json:"trades"`
	ByChannel   map[string]uint64 `json:"byChannel,omitempty"`
	Reconnects  uint64            `json:"reconnects"`
	StartedAt   int64             `json:"startedAt"`
	LastMsgAt   int64             `json:"lastMsgAt"`
	LastErr     string            `json:"lastErr,omitempty"`
}

// NewStream builds a stream of the given channels (e.g. "tickers",
// "mark-price", "open-interest", "funding-rate", "trades", "index-tickers")
// delivering to handler. perConn is how many instruments share one socket
// (default 200).
func NewStream(log *slog.Logger, perConn int, channels []string, handler func(WSEvent)) *Stream {
	if perConn <= 0 {
		perConn = 200
	}
	return &Stream{url: WSURL, log: log, handler: handler, perConn: perConn, channels: channels, stats: WSStats{ByChannel: map[string]uint64{}}}
}

// Set replaces the instrument set; connections are added, removed or
// re-subscribed to match.
func (s *Stream) Set(ctx context.Context, ids []string) {
	ids = append([]string(nil), ids...)
	sort.Strings(ids)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.want = ids
	if s.started.IsZero() {
		s.started = time.Now()
	}
	need := (len(ids) + s.perConn - 1) / s.perConn
	for len(s.conns) < need {
		c := newWSConn(s, len(s.conns))
		s.conns = append(s.conns, c)
		go c.run(ctx)
	}
	for len(s.conns) > need {
		c := s.conns[len(s.conns)-1]
		s.conns = s.conns[:len(s.conns)-1]
		c.close()
	}
	for i, c := range s.conns {
		lo := i * s.perConn
		hi := lo + s.perConn
		if hi > len(ids) {
			hi = len(ids)
		}
		c.setInstruments(ids[lo:hi])
	}
}

// Stats returns a snapshot.
func (s *Stream) Stats() WSStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.stats
	st.ByChannel = make(map[string]uint64, len(s.stats.ByChannel))
	for k, v := range s.stats.ByChannel {
		st.ByChannel[k] = v
	}
	st.Connections = len(s.conns)
	st.Instruments = len(s.want)
	st.StartedAt = s.started.UnixMilli()
	for _, c := range s.conns {
		if c.isLive() {
			st.Live++
		}
	}
	return st
}

// StartedAt is when Set was first called (zero before).
func (s *Stream) StartedAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started
}

type wsConn struct {
	s   *Stream
	idx int

	mu     sync.Mutex
	want   []string
	subbed map[string]bool
	ws     *websocket.Conn
	live   bool
	closed bool
	wake   chan struct{}
}

func newWSConn(s *Stream, idx int) *wsConn {
	return &wsConn{s: s, idx: idx, subbed: map[string]bool{}, wake: make(chan struct{}, 1)}
}

func (c *wsConn) setInstruments(ids []string) {
	c.mu.Lock()
	c.want = append([]string(nil), ids...)
	c.mu.Unlock()
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *wsConn) isLive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.live
}

func (c *wsConn) close() {
	c.mu.Lock()
	c.closed = true
	ws := c.ws
	c.mu.Unlock()
	if ws != nil {
		_ = ws.Close()
	}
}

type wsMsg struct {
	Event string `json:"event"`
	Code  string `json:"code"`
	Msg   string `json:"msg"`
	Arg   struct {
		Channel string `json:"channel"`
		InstID  string `json:"instId"`
	} `json:"arg"`
	Data []wsRow `json:"data"`
}

// wsRow is the union of the fields of every channel we subscribe to.
type wsRow struct {
	InstID string `json:"instId"`
	TS     string `json:"ts"`
	// trades
	Px    string `json:"px"`
	Sz    string `json:"sz"`
	Side  string `json:"side"`
	Count string `json:"count"`
	// tickers
	Last      string `json:"last"`
	BidPx     string `json:"bidPx"`
	AskPx     string `json:"askPx"`
	Open24h   string `json:"open24h"`
	High24h   string `json:"high24h"`
	Low24h    string `json:"low24h"`
	Vol24h    string `json:"vol24h"`
	VolCcy24h string `json:"volCcy24h"`
	// mark-price, index-tickers
	MarkPx string `json:"markPx"`
	IdxPx  string `json:"idxPx"`
	// open-interest
	OI    string `json:"oi"`
	OICcy string `json:"oiCcy"`
	OIUsd string `json:"oiUsd"`
	// funding-rate
	FundingRate     string `json:"fundingRate"`
	NextFundingRate string `json:"nextFundingRate"`
	FundingTime     string `json:"fundingTime"`
	NextFundingTime string `json:"nextFundingTime"`
	MinFundingRate  string `json:"minFundingRate"`
	MaxFundingRate  string `json:"maxFundingRate"`
}

// run connects with backoff until closed or ctx ends.
func (c *wsConn) run(ctx context.Context) {
	backoff := time.Second
	for {
		c.mu.Lock()
		closed := c.closed
		c.mu.Unlock()
		if closed || ctx.Err() != nil {
			return
		}
		err := c.session(ctx)
		c.mu.Lock()
		c.live = false
		c.ws = nil
		c.subbed = map[string]bool{}
		closed = c.closed
		c.mu.Unlock()
		if closed || ctx.Err() != nil {
			return
		}
		c.s.mu.Lock()
		c.s.stats.Reconnects++
		if err != nil {
			c.s.stats.LastErr = err.Error()
		}
		c.s.mu.Unlock()
		c.s.log.Warn("ws session ended", "conn", c.idx, "err", err, "retryIn", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (c *wsConn) session(ctx context.Context) error {
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	ws, _, err := dialer.DialContext(ctx, c.s.url, nil)
	if err != nil {
		return err
	}
	defer ws.Close()
	c.mu.Lock()
	c.ws = ws
	c.live = true
	c.mu.Unlock()

	var wmu sync.Mutex
	write := func(v any) error {
		wmu.Lock()
		defer wmu.Unlock()
		_ = ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if s, ok := v.(string); ok {
			return ws.WriteMessage(websocket.TextMessage, []byte(s))
		}
		return ws.WriteJSON(v)
	}
	resync := func() error {
		c.mu.Lock()
		want := append([]string(nil), c.want...)
		var sub, unsub []map[string]string
		wantSet := map[string]bool{}
		for _, id := range want {
			wantSet[id] = true
			if !c.subbed[id] {
				for _, ch := range c.s.channels {
					sub = append(sub, map[string]string{"channel": ch, "instId": id})
				}
				c.subbed[id] = true
			}
		}
		for id := range c.subbed {
			if !wantSet[id] {
				for _, ch := range c.s.channels {
					unsub = append(unsub, map[string]string{"channel": ch, "instId": id})
				}
				delete(c.subbed, id)
			}
		}
		c.mu.Unlock()
		const chunk = 150
		for _, op := range []struct {
			name string
			args []map[string]string
		}{{"unsubscribe", unsub}, {"subscribe", sub}} {
			for i := 0; i < len(op.args); i += chunk {
				j := i + chunk
				if j > len(op.args) {
					j = len(op.args)
				}
				if err := write(map[string]any{"op": op.name, "args": op.args[i:j]}); err != nil {
					return err
				}
			}
		}
		if len(sub) > 0 {
			c.s.log.Info("ws subscribed", "conn", c.idx, "args", len(sub), "channels", c.s.channels)
		}
		return nil
	}
	if err := resync(); err != nil {
		return err
	}

	errc := make(chan error, 1)
	lastMsg := time.Now()
	var lmu sync.Mutex
	go func() {
		for {
			_ = ws.SetReadDeadline(time.Now().Add(40 * time.Second))
			_, data, err := ws.ReadMessage()
			if err != nil {
				errc <- err
				return
			}
			lmu.Lock()
			lastMsg = time.Now()
			lmu.Unlock()
			if len(data) == 4 && string(data) == "pong" {
				continue
			}
			c.handle(data)
		}
	}()
	ping := time.NewTicker(5 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errc:
			return err
		case <-c.wake:
			c.mu.Lock()
			closed := c.closed
			c.mu.Unlock()
			if closed {
				return errors.New("closed")
			}
			if err := resync(); err != nil {
				return err
			}
		case <-ping.C:
			lmu.Lock()
			idle := time.Since(lastMsg)
			lmu.Unlock()
			if idle > 20*time.Second {
				if err := write("ping"); err != nil {
					return err
				}
			}
		}
	}
}

func (c *wsConn) handle(data []byte) {
	var m wsMsg
	if err := json.Unmarshal(data, &m); err != nil {
		return
	}
	c.s.mu.Lock()
	c.s.stats.Msgs++
	c.s.stats.LastMsgAt = time.Now().UnixMilli()
	c.s.mu.Unlock()
	if m.Event != "" {
		if m.Event == "error" {
			c.s.log.Warn("ws error event", "conn", c.idx, "code", m.Code, "msg", m.Msg)
			c.s.mu.Lock()
			c.s.stats.LastErr = m.Code + " " + m.Msg
			c.s.mu.Unlock()
		}
		return
	}
	if len(m.Data) == 0 {
		return
	}
	ev := WSEvent{Channel: m.Arg.Channel}
	switch m.Arg.Channel {
	case "trades":
		ev.Trades = make([]WSTrade, 0, len(m.Data))
		for _, d := range m.Data {
			cnt := int(f(d.Count))
			if cnt <= 0 {
				cnt = 1
			}
			ev.Trades = append(ev.Trades, WSTrade{InstID: d.InstID, TS: ms(d.TS), Px: f(d.Px), Sz: f(d.Sz), Side: d.Side, Count: cnt})
		}
	case "tickers":
		for _, d := range m.Data {
			ev.Tickers = append(ev.Tickers, Ticker{
				InstID: d.InstID, Last: f(d.Last), BidPx: f(d.BidPx), AskPx: f(d.AskPx),
				Open24h: f(d.Open24h), High24h: f(d.High24h), Low24h: f(d.Low24h),
				Vol24h: f(d.Vol24h), VolCcy24h: f(d.VolCcy24h), TS: ms(d.TS),
			})
		}
	case "mark-price":
		for _, d := range m.Data {
			ev.Marks = append(ev.Marks, MarkPrice{InstID: d.InstID, MarkPx: f(d.MarkPx), TS: ms(d.TS)})
		}
	case "index-tickers":
		for _, d := range m.Data {
			ev.Index = append(ev.Index, IndexTicker{InstID: d.InstID, IdxPx: f(d.IdxPx), TS: ms(d.TS)})
		}
	case "open-interest":
		for _, d := range m.Data {
			ev.OI = append(ev.OI, OpenInterest{InstID: d.InstID, OI: f(d.OI), OICcy: f(d.OICcy), OIUsd: f(d.OIUsd), TS: ms(d.TS)})
		}
	case "funding-rate":
		for _, d := range m.Data {
			next := math.NaN()
			if d.NextFundingRate != "" {
				next = f(d.NextFundingRate)
			}
			ev.Funding = append(ev.Funding, Funding{
				InstID: d.InstID, Rate: f(d.FundingRate), NextRate: next,
				FundingTime: ms(d.FundingTime), NextFundingTime: ms(d.NextFundingTime),
				MinRate: f(d.MinFundingRate), MaxRate: f(d.MaxFundingRate), TS: ms(d.TS),
			})
		}
	default:
		return
	}
	c.s.mu.Lock()
	c.s.stats.ByChannel[m.Arg.Channel]++
	c.s.stats.Trades += uint64(len(ev.Trades))
	c.s.mu.Unlock()
	c.s.handler(ev)
}
