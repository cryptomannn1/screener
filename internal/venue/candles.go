package venue

import (
	"context"
	"time"

	"screener/internal/metrics"
)

const (
	hourlyKeep = 220 // 168 h of returns and volume norm, with margin
	dailyKeep  = 240 // MA200 and the 30-day change, with margin
)

type candleTarget struct {
	key  string
	inst Instrument
}

// candleTargets picks, for every asset OKX does not list, the instrument whose
// candles stand for it: the busiest contract on Bybit (gentler limits), else on
// Binance. A choice once made is kept while the instrument stays listed. The
// benchmark for those assets is BTC on the same exchange.
func (s *Service) candleTargets() []candleTarget {
	rows, _ := s.rows.Rows()
	onOKX := make(map[string]bool, len(rows))
	for i := range rows {
		class, _ := okxClass(string(rows[i].Class), rows[i].Symbol)
		onOKX[Key(class, rows[i].Symbol)] = true
	}
	var out []candleTarget
	taken := map[string]bool{}
	var bench *Instrument
	for _, v := range []string{Bybit, Binance} {
		for _, in := range s.symbolsByVolume(v) {
			in := in
			k := in.Key()
			if bench == nil && k == "c:BTC" && in.Quote == "USDT" {
				bench = &in
			}
			if onOKX[k] || taken[k] {
				continue
			}
			taken[k] = true
			s.mu.RLock()
			if se := s.series[k]; se != nil {
				if cur, ok := s.inst[se.inst.Venue][se.inst.Symbol]; ok && cur.Key() == k {
					in = se.inst
				}
			}
			s.mu.RUnlock()
			out = append(out, candleTarget{key: k, inst: in})
		}
	}
	if bench != nil {
		out = append([]candleTarget{{key: benchKey, inst: *bench}}, out...)
	}
	return out
}

// candleLoop keeps the candles of assets outside OKX: a full load once, then a
// few fresh bars every period and right after each hour and UTC day turns.
func (s *Service) candleLoop(ctx context.Context) {
	// Wait for OKX rows, so that assets OKX covers are not loaded here as well.
	for i := 0; i < 45; i++ {
		if rows, _ := s.rows.Rows(); len(rows) > 0 {
			break
		}
		if !sleep(ctx, 2*time.Second) {
			return
		}
	}
	for ctx.Err() == nil {
		var last error
		okN := 0
		targets := s.candleTargets()
		for _, t := range targets {
			if ctx.Err() != nil {
				return
			}
			if err := s.refreshSeries(ctx, t); err != nil {
				last = err
			} else {
				okN++
			}
		}
		if len(targets) > 0 {
			if okN > 0 {
				last = nil
			}
			s.mark("candles", last)
		}
		s.dropStale(targets)
		if !sleep(ctx, 20*time.Second) {
			return
		}
	}
}

// dropStale forgets the candles of assets that left the target list (delisted,
// or OKX listed them).
func (s *Service) dropStale(targets []candleTarget) {
	keep := make(map[string]bool, len(targets))
	for _, t := range targets {
		keep[t.key] = true
	}
	s.mu.Lock()
	for k := range s.series {
		if !keep[k] {
			delete(s.series, k)
		}
	}
	s.mu.Unlock()
}

func (s *Service) refreshSeries(ctx context.Context, t candleTarget) error {
	s.mu.Lock()
	se := s.series[t.key]
	if se == nil || se.inst.Venue != t.inst.Venue || se.inst.Symbol != t.inst.Symbol {
		se = &series{inst: t.inst}
		s.series[t.key] = se
	}
	nh, nd, hAt, dAt := len(se.hourly), len(se.daily), se.hourlyAt, se.dailyAt
	s.mu.Unlock()

	now := time.Now()
	if nh == 0 || now.Sub(hAt) >= s.cfg.HourlyEvery || now.Truncate(time.Hour).After(hAt) {
		limit := 200
		if nh >= 170 {
			limit = 4
		}
		bars, err := s.klines(ctx, t.inst, true, limit)
		if err != nil {
			return err
		}
		s.mu.Lock()
		se.hourly = mergeBars(se.hourly, bars, 3_600_000, hourlyKeep)
		se.hourlyAt = now
		se.ret = metrics.LogReturns(se.hourly, 168)
		s.mu.Unlock()
	}
	if nd == 0 || now.Sub(dAt) >= s.cfg.DailyEvery || now.UTC().Truncate(24*time.Hour).After(dAt) {
		limit := dailyKeep
		if nd >= 60 {
			limit = 3 // the history an asset has is loaded once; later only the fresh days
		}
		bars, err := s.klines(ctx, t.inst, false, limit)
		if err != nil {
			return err
		}
		s.mu.Lock()
		se.daily = mergeBars(se.daily, bars, 86_400_000, dailyKeep)
		se.dailyAt = now
		s.mu.Unlock()
	}
	return nil
}

// klines fetches candles of an instrument and brings prices to one coin.
func (s *Service) klines(ctx context.Context, in Instrument, hourly bool, limit int) ([]metrics.Bar, error) {
	var bars []metrics.Bar
	var err error
	switch {
	case in.Venue == Bybit && hourly:
		bars, err = s.bb.Klines(ctx, in.Symbol, "60", limit)
	case in.Venue == Bybit:
		bars, err = s.bb.Klines(ctx, in.Symbol, "D", limit)
	case hourly:
		bars, err = s.bn.Klines(ctx, in.Symbol, "1h", limit)
	default:
		bars, err = s.bn.Klines(ctx, in.Symbol, "1d", limit)
	}
	if err != nil {
		return nil, err
	}
	if in.Mult != 1 && in.Mult > 0 {
		for i := range bars {
			bars[i].O /= in.Mult
			bars[i].H /= in.Mult
			bars[i].L /= in.Mult
			bars[i].C /= in.Mult
		}
	}
	return bars, nil
}

// mergeBars puts fresh bars (newest first) on top of the stored ones. When the
// fresh bars do not reach the stored ones there is a hole: the stored history
// is dropped and the next pass loads it whole again.
func mergeBars(old, fresh []metrics.Bar, step int64, keep int) []metrics.Bar {
	if len(fresh) == 0 {
		return old
	}
	oldest := fresh[len(fresh)-1].TS
	out := append([]metrics.Bar{}, fresh...)
	if len(old) > 0 && old[0].TS >= oldest-step {
		for _, b := range old {
			if b.TS < oldest {
				out = append(out, b)
			}
		}
	}
	if len(out) > keep {
		out = out[:keep]
	}
	return out
}
