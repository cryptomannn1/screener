package collector

import (
	"encoding/json"
	"math"
	"strconv"

	"screener/internal/metrics"
	"screener/internal/okx"
)

// F is a float that serialises NaN as null, so a metric that cannot be
// computed yet reads as "no value" in the API instead of breaking the JSON.
type F float64

// MarshalJSON implements json.Marshaler.
func (v F) MarshalJSON() ([]byte, error) {
	x := float64(v)
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return []byte("null"), nil
	}
	return []byte(strconv.FormatFloat(x, 'g', -1, 64)), nil
}

// Periods are the switchable horizons of the widget; every per-period metric
// is a map keyed by them.
var Periods = []string{"1h", "4h", "24h"}

// PeriodHours maps a period id to hourly bars.
var PeriodHours = map[string]int{"1h": 1, "4h": 4, "24h": 24}

// PerPeriod holds one metric per period.
type PerPeriod map[string]F

// Row is one screener line: everything the widget shows for one instrument.
// Field names match the prototype's column keys so the UI adapter is 1:1.
type Row struct {
	InstID string `json:"instId"`
	Symbol string `json:"symbol"` // base coin, e.g. BTC
	Settle string `json:"settle"` // USDT | USDC
	Lever  F      `json:"lev"`
	Listed F      `json:"listed"` // days since listing

	// Asset: class from the venue; the rest from the metadata resolver,
	// crypto only (null / "" for stocks, indices and commodities).
	Class    okx.Class `json:"class"`          // crypto | stock | premarket | commodity | other
	Name     string    `json:"name,omitempty"` // project name
	Sector   string    `json:"sector"`         // one of meta.Sectors, Other, or "" when unknown
	Mcap     F         `json:"mcap"`           // market capitalisation, USD
	FDV      F         `json:"fdv"`            // fully diluted valuation, USD
	McapRank F         `json:"mcapRank"`       // market-cap rank
	VolMcap  F         `json:"volMcap"`        // vol24 / mcap (daily turnover of the cap)
	OIMcap   F         `json:"oiMcap"`         // oi / mcap

	// Price
	Last  F `json:"last"`
	Mark  F `json:"mark"`
	Index F `json:"index"`
	Basis F `json:"basis"` // mark / index − 1
	Bid   F `json:"bid"`
	Ask   F `json:"ask"`

	Chg    PerPeriod `json:"chg"`   // last / close N hours ago − 1
	Chg1d  F         `json:"chg1d"` // vs yesterday's UTC close
	Chg7d  F         `json:"chg7d"`
	Chg30d F         `json:"chg30d"`

	// Volume (quote currency)
	Vol24 F         `json:"vol24"`
	VolX7 F         `json:"volx7"` // vol24 / avg of last 7 full days
	VolX  PerPeriod `json:"volx"`  // period volume / its 7-day average

	// Open interest
	OI    F         `json:"oi"`    // USD
	OID   PerPeriod `json:"oiD"`   // OI now / OI N hours ago − 1
	OIVol F         `json:"oiVol"` // OI / vol24

	// Funding
	Funding         F `json:"funding"`         // current rate
	FundingAnn      F `json:"fundingAnn"`      // annualised
	FundingNext     F `json:"fundingNext"`     // predicted next rate
	FundingInterval F `json:"fundingInterval"` // hours

	// Volatility
	ATR     F `json:"atr"`     // ATR(14, 1H) / last
	RV24    F `json:"rv24"`    // annualised std of 24 hourly log returns
	Regime  F `json:"regime"`  // today's TR / median TR of 20 days
	Range1d F `json:"range1d"` // today's (H − L) / L
	ToHigh  F `json:"toHigh"`  // last / 30d high − 1
	ToLow   F `json:"toLow"`   // last / 30d low − 1

	// Strength vs BTC (7 days of hourly returns)
	VsBtc    PerPeriod `json:"vsBtc"` // chg − chg(BTC)
	Beta     F         `json:"beta"`
	Alpha    F         `json:"alpha"` // mean hourly excess over β·BTC
	Presence F         `json:"presence"`
	Stay     F         `json:"stay"`
	Corr     F         `json:"corr"`

	// Order book (quote currency, both sides)
	Depth01 F `json:"depth01"`
	Depth05 F `json:"depth05"`
	Depth1  F `json:"depth1"`
	Imb     F `json:"imb"`    // (bid05 − ask05) / (bid05 + ask05)
	Spread  F `json:"spread"` // bps

	// Flow (trade stream)
	TakerBuy PerPeriod `json:"takerBuy"` // taker buy share of volume
	Delta    PerPeriod `json:"delta"`    // (buy − sell) / total
	Trades   PerPeriod `json:"trades"`   // taker orders in the period
	Tape     F         `json:"tape"`     // taker orders per minute, last 5 minutes

	// Levels (UTC daily bars): distance of last from the level, and the level
	// price itself for tooltips.
	DO    Level `json:"do"`   // today's UTC open
	WO    Level `json:"wo"`   // open of the current ISO week (Monday UTC)
	MO    Level `json:"mo"`   // open of the current month (UTC)
	MA50  Level `json:"ma50"` // simple MA of 50 daily closes incl. the forming day
	MA100 Level `json:"ma100"`
	MA200 Level `json:"ma200"`

	Spark []F `json:"spark"` // last 24 hourly closes, oldest first (forming bar last)

	Updated int64    `json:"updated"` // ms of the last ticker
	Cov     Coverage `json:"cov"`
}

// Level is a reference price and the distance of last from it.
type Level struct {
	Px   F `json:"px"`
	Dist F `json:"dist"` // last / px − 1
}

func level(last, px float64) Level {
	return Level{Px: F(px), Dist: F(metrics.Distance(last, px))}
}

// Coverage says which data families have arrived for the row.
type Coverage struct {
	Hourly bool `json:"hourly"`
	Daily  bool `json:"daily"`
	Book   bool `json:"book"`
	OIHist bool `json:"oiHist"` // at least 1 h of OI history
	Flow   bool `json:"flow"`
}

// MarshalJSON keeps nil maps as {} rather than null.
func (p PerPeriod) MarshalJSON() ([]byte, error) {
	if p == nil {
		return []byte("{}"), nil
	}
	m := make(map[string]F, len(p))
	for k, v := range p {
		m[k] = v
	}
	return json.Marshal(m)
}
