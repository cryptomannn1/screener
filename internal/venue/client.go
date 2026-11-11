package venue

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"screener/internal/okx"
)

// getter is a rate-limited JSON GET against one public REST API.
type getter struct {
	base string
	hc   *http.Client
	lim  *okx.Limiter
}

func newGetter(base string, perSec int) getter {
	return getter{base: strings.TrimRight(base, "/"), hc: &http.Client{Timeout: 15 * time.Second}, lim: okx.NewLimiter(perSec*2, 2*time.Second)}
}

func (g getter) get(ctx context.Context, path string, q url.Values, dst any) error {
	if err := g.lim.Wait(ctx); err != nil {
		return err
	}
	u := g.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "screener/0.1")
	res, err := g.hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return fmt.Errorf("%s: http %d: %s", path, res.StatusCode, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(res.Body).Decode(dst)
}

// num parses an exchange's decimal string; "" and garbage are NaN.
func num(s string) float64 {
	if s == "" {
		return math.NaN()
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return math.NaN()
	}
	return v
}

// anyNum reads a JSON value that an exchange sends either as a number or as a
// decimal string (Binance candles mix both in one array).
func anyNum(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		return num(x)
	}
	return math.NaN()
}
