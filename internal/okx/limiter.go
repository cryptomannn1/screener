package okx

import (
	"context"
	"sync"
	"time"
)

// Limiter is a token bucket sized to one OKX public rate limit: n requests
// per window per IP. OKX documents its public limits as "N requests / 2 s";
// the bucket refills continuously at n/window so a burst never exceeds n in
// any window, and Wait blocks until a token is free or ctx ends.
type Limiter struct {
	mu     sync.Mutex
	every  time.Duration
	burst  int
	tokens float64
	last   time.Time
}

// NewLimiter allows n requests per window with a small burst: OKX counts
// requests in short windows, so an initial burst of n would itself trip the
// limit even though the long-run rate is right.
func NewLimiter(n int, window time.Duration) *Limiter {
	if n < 1 {
		n = 1
	}
	burst := n / 4
	if burst < 2 {
		burst = 2
	}
	return &Limiter{every: window / time.Duration(n), burst: burst, tokens: float64(burst), last: time.Now()}
}

// Wait blocks until one token is available.
func (l *Limiter) Wait(ctx context.Context) error {
	for {
		l.mu.Lock()
		now := time.Now()
		l.tokens += float64(now.Sub(l.last)) / float64(l.every)
		if l.tokens > float64(l.burst) {
			l.tokens = float64(l.burst)
		}
		l.last = now
		if l.tokens >= 1 {
			l.tokens--
			l.mu.Unlock()
			return nil
		}
		wait := time.Duration((1 - l.tokens) * float64(l.every))
		l.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}
