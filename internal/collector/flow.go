package collector

import (
	"math"
	"sync"
	"time"
)

// flowMinutes is the window of per-minute taker buckets kept per instrument.
const flowMinutes = 24 * 60

// FlowMinute is one closed minute of taker activity for one instrument.
type FlowMinute struct {
	InstID    string
	Minute    time.Time
	BuyQuote  float64
	SellQuote float64
	Trades    int
	BuyTrades int
}

type flowBucket struct {
	minute    int64 // unix minute
	buyQuote  float64
	sellQuote float64
	trades    int
	buyTrades int
}

// flowBook is a ring of minute buckets for one instrument.
type flowBook struct {
	ring [flowMinutes]flowBucket
	last int64 // newest minute seen
}

// Flow aggregates the trade stream into per-minute taker buckets and answers
// per-period taker share, delta and trade counts.
type Flow struct {
	mu    sync.Mutex
	books map[string]*flowBook
	since time.Time // when the stream first delivered
}

// NewFlow builds an empty aggregator.
func NewFlow() *Flow { return &Flow{books: map[string]*flowBook{}} }

// Add records one trade. quote is px × size in quote currency.
func (f *Flow) Add(instID string, tsMs int64, buy bool, quote float64, count int) {
	minute := tsMs / 60_000
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.since.IsZero() {
		f.since = time.Now()
	}
	b := f.books[instID]
	if b == nil {
		b = &flowBook{}
		f.books[instID] = b
	}
	slot := &b.ring[minute%flowMinutes]
	if slot.minute != minute {
		*slot = flowBucket{minute: minute}
	}
	if minute > b.last {
		b.last = minute
	}
	if buy {
		slot.buyQuote += quote
		slot.buyTrades += count
	} else {
		slot.sellQuote += quote
	}
	slot.trades += count
}

// Seed restores one stored minute (service restart). It never moves `since`
// forward; call SetSince once the seeded range is known.
func (f *Flow) Seed(instID string, minute int64, buyQuote, sellQuote float64, trades, buyTrades int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b := f.books[instID]
	if b == nil {
		b = &flowBook{}
		f.books[instID] = b
	}
	slot := &b.ring[((minute%flowMinutes)+flowMinutes)%flowMinutes]
	if slot.minute != minute {
		*slot = flowBucket{minute: minute}
	}
	slot.buyQuote += buyQuote
	slot.sellQuote += sellQuote
	slot.trades += trades
	slot.buyTrades += buyTrades
	if minute > b.last {
		b.last = minute
	}
}

// SetSince declares from when the window is continuous (restored history
// without gaps up to now). Only moves the mark earlier.
func (f *Flow) SetSince(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.since.IsZero() || t.Before(f.since) {
		f.since = t
	}
}

// Since is when the first trade arrived (zero if none yet).
func (f *Flow) Since() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.since
}

// FlowStats summarises one instrument's window.
type FlowStats struct {
	BuyQuote, SellQuote float64
	Trades              int
}

// Window sums the last minutes ending now (inclusive of the current minute).
func (f *Flow) Window(instID string, nowMs int64, minutes int) FlowStats {
	f.mu.Lock()
	defer f.mu.Unlock()
	b := f.books[instID]
	var s FlowStats
	if b == nil {
		return s
	}
	cur := nowMs / 60_000
	for m := cur - int64(minutes) + 1; m <= cur; m++ {
		slot := &b.ring[((m%flowMinutes)+flowMinutes)%flowMinutes]
		if slot.minute != m {
			continue
		}
		s.BuyQuote += slot.buyQuote
		s.SellQuote += slot.sellQuote
		s.Trades += slot.trades
	}
	return s
}

// TakerShare is buy / (buy + sell), NaN without volume.
func (s FlowStats) TakerShare() float64 {
	if t := s.BuyQuote + s.SellQuote; t > 0 {
		return s.BuyQuote / t
	}
	return math.NaN()
}

// Delta is (buy − sell) / (buy + sell), NaN without volume.
func (s FlowStats) Delta() float64 {
	if t := s.BuyQuote + s.SellQuote; t > 0 {
		return (s.BuyQuote - s.SellQuote) / t
	}
	return math.NaN()
}

// Closed returns every bucket of the given minute (one per instrument that
// traded) so the store can persist closed minutes.
func (f *Flow) Closed(minute int64) []FlowMinute {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []FlowMinute
	for id, b := range f.books {
		slot := &b.ring[((minute%flowMinutes)+flowMinutes)%flowMinutes]
		if slot.minute != minute || slot.trades == 0 {
			continue
		}
		out = append(out, FlowMinute{
			InstID: id, Minute: time.Unix(minute*60, 0).UTC(),
			BuyQuote: slot.buyQuote, SellQuote: slot.sellQuote, Trades: slot.trades, BuyTrades: slot.buyTrades,
		})
	}
	return out
}
