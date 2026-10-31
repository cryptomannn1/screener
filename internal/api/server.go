// Package api serves the screener over plain HTTP with permissive CORS: the
// terminal widget polls GET /screener and renders the rows as they are.
package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"screener/internal/collector"
	"screener/internal/store"
)

// Server is the HTTP layer.
type Server struct {
	col     *collector.Collector
	st      *store.Store // may be nil
	mux     *http.ServeMux
	start   time.Time
	version string
}

// New builds the handler.
func New(col *collector.Collector, st *store.Store, version string) *Server {
	s := &Server{col: col, st: st, mux: http.NewServeMux(), start: time.Now(), version: version}
	s.mux.HandleFunc("GET /screener", s.handleScreener)
	s.mux.HandleFunc("GET /instruments", s.handleInstruments)
	s.mux.HandleFunc("GET /health", s.handleHealth)
	return s
}

// ServeHTTP implements http.Handler with CORS.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	_ = enc.Encode(v)
}

// ScreenerResponse is the payload of GET /screener.
type ScreenerResponse struct {
	TS        int64           `json:"ts"`
	Periods   []string        `json:"periods"`
	Benchmark string          `json:"benchmark"`
	Count     int             `json:"count"`
	Total     int             `json:"total"`
	Rows      []collector.Row `json:"rows"`
	Coverage  map[string]int  `json:"coverage"`
}

// handleScreener: GET /screener?symbols=BTC,ETH&limit=N&sort=vol24&dir=desc
func (s *Server) handleScreener(w http.ResponseWriter, r *http.Request) {
	rows, at := s.col.Rows()
	if at.IsZero() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "warming up", "health": s.col.Health()})
		return
	}
	q := r.URL.Query()
	out := rows
	if syms := q.Get("symbols"); syms != "" {
		want := map[string]bool{}
		for _, p := range strings.Split(strings.ToUpper(syms), ",") {
			want[strings.TrimSpace(p)] = true
		}
		out = out[:0:0]
		for _, row := range rows {
			if want[row.Symbol] || want[row.InstID] {
				out = append(out, row)
			}
		}
	}
	if key := q.Get("sort"); key != "" {
		out = append([]collector.Row(nil), out...)
		desc := q.Get("dir") != "asc"
		period := q.Get("period")
		if period == "" {
			period = "24h"
		}
		sortRows(out, key, period, desc)
	}
	total := len(out)
	if lim, _ := strconv.Atoi(q.Get("limit")); lim > 0 && lim < len(out) {
		out = out[:lim]
	}
	h := s.col.Health()
	writeJSON(w, http.StatusOK, ScreenerResponse{
		TS: at.UnixMilli(), Periods: collector.Periods, Benchmark: h.Benchmark,
		Count: len(out), Total: total, Rows: out, Coverage: h.Coverage,
	})
}

func sortRows(rows []collector.Row, key, period string, desc bool) {
	val := func(r collector.Row) float64 {
		switch key {
		case "vol24":
			return float64(r.Vol24)
		case "oi":
			return float64(r.OI)
		case "chg":
			return float64(r.Chg[period])
		case "oiD":
			return float64(r.OID[period])
		case "vsBtc":
			return float64(r.VsBtc[period])
		case "volx":
			return float64(r.VolX[period])
		case "funding":
			return float64(r.Funding)
		case "fundingAnn":
			return float64(r.FundingAnn)
		case "atr":
			return float64(r.ATR)
		case "rv24":
			return float64(r.RV24)
		case "regime":
			return float64(r.Regime)
		case "beta":
			return float64(r.Beta)
		case "presence":
			return float64(r.Presence)
		case "depth05":
			return float64(r.Depth05)
		case "spread":
			return float64(r.Spread)
		case "listed":
			return float64(r.Listed)
		default:
			return float64(r.Vol24)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := val(rows[i]), val(rows[j])
		if a != a { // NaN last
			return false
		}
		if b != b {
			return true
		}
		if desc {
			return a > b
		}
		return a < b
	})
}

func (s *Server) handleInstruments(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"count": len(s.col.Instruments()), "instruments": s.col.Instruments()})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	h := s.col.Health()
	resp := map[string]any{
		"status":    "ok",
		"version":   s.version,
		"collector": h,
	}
	code := http.StatusOK
	if h.Rows == 0 {
		resp["status"] = "warming"
		code = http.StatusServiceUnavailable
	}
	if s.st != nil {
		resp["store"] = s.st.Stats()
	} else {
		resp["store"] = nil
	}
	writeJSON(w, code, resp)
}
