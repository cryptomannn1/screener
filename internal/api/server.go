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
	"screener/internal/meta"
	"screener/internal/store"
)

// Server is the HTTP layer.
type Server struct {
	col     *collector.Collector
	st      *store.Store // may be nil
	mux     *http.ServeMux
	start   time.Time
	version string
	meta    *meta.Service // may be nil
}

// WithMeta exposes the metadata resolver's state in /health.
func (s *Server) WithMeta(m *meta.Service) *Server {
	s.meta = m
	return s
}

// New builds the handler. staticDir, when set, is served under /ui/ (the
// prototype page or a built widget sandbox).
func New(col *collector.Collector, st *store.Store, version string, staticDir string) *Server {
	s := &Server{col: col, st: st, mux: http.NewServeMux(), start: time.Now(), version: version}
	if staticDir != "" {
		s.mux.Handle("GET /ui/", http.StripPrefix("/ui/", http.FileServer(http.Dir(staticDir))))
	}
	s.mux.HandleFunc("GET /screener", s.handleScreener)
	s.mux.HandleFunc("GET /stream", s.handleStream)
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
	writeJSONPretty(w, code, v, false)
}

func writeJSONPretty(w http.ResponseWriter, code int, v any, pretty bool) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	if pretty {
		enc.SetIndent("", "  ")
	}
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

// snapshot applies symbols / sort / period / dir / limit from q to the latest
// rows. ok is false while the collector is warming up.
func (s *Server) snapshot(q map[string][]string) (ScreenerResponse, bool) {
	get := func(k string) string {
		if v := q[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	rows, at := s.col.Rows()
	if at.IsZero() {
		return ScreenerResponse{}, false
	}
	out := rows
	if syms := get("symbols"); syms != "" {
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
	// class=crypto,stock,… and sector=Meme,AI,… narrow by asset class and sector (v0.4).
	if cls, sec := get("class"), get("sector"); cls != "" || sec != "" {
		wantC, wantS := listSet(cls), listSet(sec)
		filtered := make([]collector.Row, 0, len(out))
		for _, row := range out {
			if len(wantC) > 0 && !wantC[strings.ToLower(string(row.Class))] {
				continue
			}
			if len(wantS) > 0 && !wantS[strings.ToLower(row.Sector)] {
				continue
			}
			filtered = append(filtered, row)
		}
		out = filtered
	}
	if key := get("sort"); key != "" {
		out = append([]collector.Row(nil), out...)
		desc := get("dir") != "asc"
		period := get("period")
		if period == "" {
			period = "24h"
		}
		sortRows(out, key, period, desc)
	}
	total := len(out)
	if lim, _ := strconv.Atoi(get("limit")); lim > 0 && lim < len(out) {
		out = out[:lim]
	}
	h := s.col.Health()
	return ScreenerResponse{
		TS: at.UnixMilli(), Periods: collector.Periods, Benchmark: h.Benchmark,
		Count: len(out), Total: total, Rows: out, Coverage: h.Coverage,
	}, true
}

// handleScreener: GET /screener?symbols=BTC,ETH&limit=N&sort=vol24&dir=desc&pretty=1
func (s *Server) handleScreener(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	resp, ok := s.snapshot(q)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "warming up", "health": s.col.Health()})
		return
	}
	writeJSONPretty(w, http.StatusOK, resp, q.Get("pretty") != "")
}

// handleStream: GET /stream — Server-Sent Events. One `rows` event with the
// same payload as /screener every time the collector recomputes (about every
// 5 s), a comment line every 15 s as keep-alive. Same query parameters.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	q := r.URL.Query()
	var lastTS int64
	send := func() bool {
		resp, ok := s.snapshot(q)
		if !ok || resp.TS == lastTS {
			return false
		}
		lastTS = resp.TS
		b, err := json.Marshal(resp)
		if err != nil {
			return false
		}
		_, _ = w.Write([]byte("event: rows\ndata: "))
		_, _ = w.Write(b)
		_, _ = w.Write([]byte("\n\n"))
		fl.Flush()
		return true
	}
	send()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	keep := time.NewTicker(15 * time.Second)
	defer keep.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			send()
		case <-keep.C:
			_, _ = w.Write([]byte(": ping\n\n"))
			fl.Flush()
		}
	}
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
	if s.meta != nil {
		resp["meta"] = s.meta.Health()
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

// listSet turns "a,B, c" into a lower-case set; empty input gives an empty set.
func listSet(csv string) map[string]bool {
	out := map[string]bool{}
	for _, p := range strings.Split(csv, ",") {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			out[p] = true
		}
	}
	return out
}
