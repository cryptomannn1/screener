// Package okx is the REST client for the OKX v5 public market endpoints the
// screener needs. No keys: everything here is public data. Every call goes
// through a per-endpoint-group Limiter sized to OKX's documented IP limits,
// so a full pass over ~520 instruments never trips a 429.
package okx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// BaseURL is the OKX REST host.
const BaseURL = "https://www.okx.com"

// Client talks to the OKX public REST API.
type Client struct {
	http *http.Client
	base string
	log  *slog.Logger

	// Limits per endpoint family (requests per 2 s, per IP, from the docs).
	limTickers *Limiter // market/tickers, market/index-tickers: 20/2s
	limPublic  *Limiter // public/instruments, open-interest, funding-rate, mark-price: 20/2s
	limCandles *Limiter // market/candles: 40/2s
	limBooks   *Limiter // market/books: 40/2s
	limFull    *Limiter // market/books-full: 10/2s
}

// New builds a client. A nil logger discards.
func New(log *slog.Logger) *Client {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Client{
		http: &http.Client{Timeout: 15 * time.Second},
		base: BaseURL,
		log:  log,
		// About 60 % of the documented limits: OKX also meters the whole IP,
		// and the sweeps run concurrently.
		limTickers: NewLimiter(10, 2*time.Second),
		limPublic:  NewLimiter(10, 2*time.Second),
		limCandles: NewLimiter(24, 2*time.Second),
		limBooks:   NewLimiter(24, 2*time.Second),
		limFull:    NewLimiter(6, 2*time.Second),
	}
}

// SetBaseURL points the client at another host (tests).
func (c *Client) SetBaseURL(u string) { c.base = u }

type envelope struct {
	Code string          `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// APIError is a non-zero OKX code.
type APIError struct {
	Code string
	Msg  string
	Path string
}

func (e *APIError) Error() string { return fmt.Sprintf("okx %s: code %s: %s", e.Path, e.Code, e.Msg) }

// ErrRateLimited is returned on HTTP 429.
var ErrRateLimited = errors.New("okx: rate limited")

// get performs one GET under the limiter; a rate-limit answer is retried once
// after a pause so a sweep survives a short burst.
func (c *Client) get(ctx context.Context, lim *Limiter, path string, q url.Values, out any) error {
	err := c.getOnce(ctx, lim, path, q, out)
	if errors.Is(err, ErrRateLimited) {
		c.log.Debug("rate limited, pausing", "path", path)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2500 * time.Millisecond):
		}
		err = c.getOnce(ctx, lim, path, q, out)
	}
	return err
}

func (c *Client) getOnce(ctx context.Context, lim *Limiter, path string, q url.Values, out any) error {
	if err := lim.Wait(ctx); err != nil {
		return err
	}
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return err
	}
	if res.StatusCode == http.StatusTooManyRequests {
		return ErrRateLimited
	}
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("okx %s: http %d: %s", path, res.StatusCode, truncate(body, 200))
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("okx %s: decode: %w", path, err)
	}
	if env.Code == "50011" || env.Code == "50061" {
		return ErrRateLimited
	}
	if env.Code != "0" {
		return &APIError{Code: env.Code, Msg: env.Msg, Path: path}
	}
	return json.Unmarshal(env.Data, out)
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

// f parses an OKX decimal string; "" → 0.
func f(s string) float64 {
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

// ms parses an OKX millisecond timestamp string.
func ms(s string) int64 {
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

// Instruments lists SWAP instruments (public/instruments).
func (c *Client) Instruments(ctx context.Context, instType string) ([]Instrument, error) {
	var raw []struct {
		InstID    string `json:"instId"`
		InstType  string `json:"instType"`
		Uly       string `json:"uly"`
		BaseCcy   string `json:"baseCcy"`
		QuoteCcy  string `json:"quoteCcy"`
		SettleCcy string `json:"settleCcy"`
		CtVal     string `json:"ctVal"`
		CtValCcy  string `json:"ctValCcy"`
		CtType    string `json:"ctType"`
		TickSz    string `json:"tickSz"`
		LotSz     string `json:"lotSz"`
		Lever     string `json:"lever"`
		ListTime  string `json:"listTime"`
		State     string `json:"state"`
	}
	q := url.Values{"instType": {instType}}
	if err := c.get(ctx, c.limPublic, "/api/v5/public/instruments", q, &raw); err != nil {
		return nil, err
	}
	out := make([]Instrument, 0, len(raw))
	for _, r := range raw {
		base := r.BaseCcy
		if base == "" && r.Uly != "" {
			// SWAP: uly is "BTC-USDT"; the base coin is the part before the dash.
			for i := 0; i < len(r.Uly); i++ {
				if r.Uly[i] == '-' {
					base = r.Uly[:i]
					break
				}
			}
		}
		out = append(out, Instrument{
			ID: r.InstID, Type: r.InstType, Uly: r.Uly, Base: base, SettleCcy: r.SettleCcy,
			CtVal: f(r.CtVal), CtValCcy: r.CtValCcy, CtType: r.CtType, TickSz: f(r.TickSz),
			LotSz: f(r.LotSz), Lever: f(r.Lever), ListTime: ms(r.ListTime), State: r.State,
		})
	}
	return out, nil
}

// Tickers returns every ticker of an instrument type (market/tickers).
func (c *Client) Tickers(ctx context.Context, instType string) ([]Ticker, error) {
	var raw []struct {
		InstID    string `json:"instId"`
		Last      string `json:"last"`
		BidPx     string `json:"bidPx"`
		AskPx     string `json:"askPx"`
		Open24h   string `json:"open24h"`
		High24h   string `json:"high24h"`
		Low24h    string `json:"low24h"`
		Vol24h    string `json:"vol24h"`
		VolCcy24h string `json:"volCcy24h"`
		TS        string `json:"ts"`
	}
	q := url.Values{"instType": {instType}}
	if err := c.get(ctx, c.limTickers, "/api/v5/market/tickers", q, &raw); err != nil {
		return nil, err
	}
	out := make([]Ticker, 0, len(raw))
	for _, r := range raw {
		out = append(out, Ticker{
			InstID: r.InstID, Last: f(r.Last), BidPx: f(r.BidPx), AskPx: f(r.AskPx),
			Open24h: f(r.Open24h), High24h: f(r.High24h), Low24h: f(r.Low24h),
			Vol24h: f(r.Vol24h), VolCcy24h: f(r.VolCcy24h), TS: ms(r.TS),
		})
	}
	return out, nil
}

// OpenInterest returns OI of every instrument of a type (public/open-interest).
func (c *Client) OpenInterest(ctx context.Context, instType string) ([]OpenInterest, error) {
	var raw []struct {
		InstID string `json:"instId"`
		OI     string `json:"oi"`
		OICcy  string `json:"oiCcy"`
		OIUsd  string `json:"oiUsd"`
		TS     string `json:"ts"`
	}
	q := url.Values{"instType": {instType}}
	if err := c.get(ctx, c.limPublic, "/api/v5/public/open-interest", q, &raw); err != nil {
		return nil, err
	}
	out := make([]OpenInterest, 0, len(raw))
	for _, r := range raw {
		out = append(out, OpenInterest{InstID: r.InstID, OI: f(r.OI), OICcy: f(r.OICcy), OIUsd: f(r.OIUsd), TS: ms(r.TS)})
	}
	return out, nil
}

// FundingRates returns the current and next funding of every swap
// (public/funding-rate?instId=ANY).
func (c *Client) FundingRates(ctx context.Context) ([]Funding, error) {
	var raw []struct {
		InstID          string `json:"instId"`
		FundingRate     string `json:"fundingRate"`
		NextFundingRate string `json:"nextFundingRate"`
		FundingTime     string `json:"fundingTime"`
		NextFundingTime string `json:"nextFundingTime"`
		MinFundingRate  string `json:"minFundingRate"`
		MaxFundingRate  string `json:"maxFundingRate"`
		TS              string `json:"ts"`
	}
	q := url.Values{"instId": {"ANY"}}
	if err := c.get(ctx, c.limPublic, "/api/v5/public/funding-rate", q, &raw); err != nil {
		return nil, err
	}
	out := make([]Funding, 0, len(raw))
	for _, r := range raw {
		next := math.NaN() // OKX sends "" when no prediction is published
		if r.NextFundingRate != "" {
			next = f(r.NextFundingRate)
		}
		out = append(out, Funding{
			InstID: r.InstID, Rate: f(r.FundingRate), NextRate: next,
			FundingTime: ms(r.FundingTime), NextFundingTime: ms(r.NextFundingTime),
			MinRate: f(r.MinFundingRate), MaxRate: f(r.MaxFundingRate), TS: ms(r.TS),
		})
	}
	return out, nil
}

// MarkPrices returns mark prices of every instrument of a type (public/mark-price).
func (c *Client) MarkPrices(ctx context.Context, instType string) ([]MarkPrice, error) {
	var raw []struct {
		InstID string `json:"instId"`
		MarkPx string `json:"markPx"`
		TS     string `json:"ts"`
	}
	q := url.Values{"instType": {instType}}
	if err := c.get(ctx, c.limPublic, "/api/v5/public/mark-price", q, &raw); err != nil {
		return nil, err
	}
	out := make([]MarkPrice, 0, len(raw))
	for _, r := range raw {
		out = append(out, MarkPrice{InstID: r.InstID, MarkPx: f(r.MarkPx), TS: ms(r.TS)})
	}
	return out, nil
}

// IndexTickers returns index prices for one quote currency (market/index-tickers).
func (c *Client) IndexTickers(ctx context.Context, quoteCcy string) ([]IndexTicker, error) {
	var raw []struct {
		InstID string `json:"instId"`
		IdxPx  string `json:"idxPx"`
		TS     string `json:"ts"`
	}
	q := url.Values{"quoteCcy": {quoteCcy}}
	if err := c.get(ctx, c.limTickers, "/api/v5/market/index-tickers", q, &raw); err != nil {
		return nil, err
	}
	out := make([]IndexTicker, 0, len(raw))
	for _, r := range raw {
		out = append(out, IndexTicker{InstID: r.InstID, IdxPx: f(r.IdxPx), TS: ms(r.TS)})
	}
	return out, nil
}

// Candles returns up to limit bars (max 300) of one instrument, newest first
// (market/candles). bar is an OKX bar id: "1H", "1Dutc", "5m"…
// For SWAP, Vol is in contracts, VolCcy in base coin, VolCcyQuote in quote.
func (c *Client) Candles(ctx context.Context, instID, bar string, limit int) ([]Candle, error) {
	if limit <= 0 || limit > 300 {
		limit = 300
	}
	var raw [][]string
	q := url.Values{"instId": {instID}, "bar": {bar}, "limit": {strconv.Itoa(limit)}}
	if err := c.get(ctx, c.limCandles, "/api/v5/market/candles", q, &raw); err != nil {
		return nil, err
	}
	return parseCandles(raw), nil
}

func parseCandles(raw [][]string) []Candle {
	out := make([]Candle, 0, len(raw))
	for _, r := range raw {
		if len(r) < 9 {
			continue
		}
		out = append(out, Candle{
			TS: ms(r[0]), Open: f(r[1]), High: f(r[2]), Low: f(r[3]), Close: f(r[4]),
			Vol: f(r[5]), VolCcy: f(r[6]), VolCcyQuote: f(r[7]), Confirm: r[8] == "1",
		})
	}
	return out
}

// Books returns up to sz levels per side of one instrument (market/books,
// sz ≤ 400). Sizes are in contracts.
func (c *Client) Books(ctx context.Context, instID string, sz int) (Book, error) {
	if sz <= 0 || sz > 400 {
		sz = 400
	}
	return c.books(ctx, c.limBooks, "/api/v5/market/books", instID, sz)
}

// BooksFull returns up to sz levels per side (market/books-full, sz ≤ 5000,
// 10 requests / 2 s). Used when 400 levels do not reach ±1 % of the mid.
func (c *Client) BooksFull(ctx context.Context, instID string, sz int) (Book, error) {
	if sz <= 0 || sz > 5000 {
		sz = 5000
	}
	return c.books(ctx, c.limFull, "/api/v5/market/books-full", instID, sz)
}

func (c *Client) books(ctx context.Context, lim *Limiter, path, instID string, sz int) (Book, error) {
	var raw []struct {
		Asks [][]string `json:"asks"`
		Bids [][]string `json:"bids"`
		TS   string     `json:"ts"`
	}
	q := url.Values{"instId": {instID}, "sz": {strconv.Itoa(sz)}}
	if err := c.get(ctx, lim, path, q, &raw); err != nil {
		return Book{}, err
	}
	if len(raw) == 0 {
		return Book{}, fmt.Errorf("okx %s %s: empty", path, instID)
	}
	b := Book{TS: ms(raw[0].TS), Requested: sz}
	for _, l := range raw[0].Bids {
		if len(l) >= 2 {
			b.Bids = append(b.Bids, Level{Px: f(l[0]), Sz: f(l[1])})
		}
	}
	for _, l := range raw[0].Asks {
		if len(l) >= 2 {
			b.Asks = append(b.Asks, Level{Px: f(l[0]), Sz: f(l[1])})
		}
	}
	return b, nil
}
