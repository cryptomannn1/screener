package okx

// Instrument is one row of public/instruments.
type Instrument struct {
	ID        string  `json:"instId"`
	Type      string  `json:"instType"`
	Uly       string  `json:"uly"`
	Base      string  `json:"base"`
	SettleCcy string  `json:"settleCcy"`
	CtVal     float64 `json:"ctVal"`    // base coin per contract (SWAP)
	CtValCcy  string  `json:"ctValCcy"` // base coin for linear swaps, USD for inverse
	CtType    string  `json:"ctType"`   // linear | inverse
	TickSz    float64 `json:"tickSz"`
	LotSz     float64 `json:"lotSz"`
	Lever     float64 `json:"lever"`
	ListTime  int64   `json:"listTime"`     // ms
	State     string  `json:"state"`        // live | suspend | preopen | test
	Category  string  `json:"instCategory"` // venue asset class: 1 crypto, 3 tokenised stock / ETF / index, 4 commodity
	RuleType  string  `json:"ruleType"`     // normal | pre_market
}

// Class is the asset class of an instrument as the venue reports it.
type Class string

const (
	ClassCrypto    Class = "crypto"
	ClassStock     Class = "stock"     // tokenised equities, ETFs and indices
	ClassPreMarket Class = "premarket" // pre-listing contracts (OpenAI, Anthropic, …)
	ClassCommodity Class = "commodity" // metals and energy
	ClassOther     Class = "other"
)

// AssetClass maps the venue's instCategory / ruleType to a Class.
func (i Instrument) AssetClass() Class {
	if i.RuleType == "pre_market" {
		return ClassPreMarket
	}
	switch i.Category {
	case "1":
		return ClassCrypto
	case "3":
		return ClassStock
	case "4":
		return ClassCommodity
	}
	return ClassOther
}

// Ticker is one row of market/tickers. For SWAP, Vol24h is in contracts and
// VolCcy24h in base coin; the quote volume is VolCcy24h × Last.
type Ticker struct {
	InstID    string
	Last      float64
	BidPx     float64
	AskPx     float64
	Open24h   float64
	High24h   float64
	Low24h    float64
	Vol24h    float64
	VolCcy24h float64
	TS        int64
}

// OpenInterest is one row of public/open-interest.
type OpenInterest struct {
	InstID string
	OI     float64 // contracts
	OICcy  float64 // base coin
	OIUsd  float64 // USD
	TS     int64
}

// Funding is one row of public/funding-rate.
type Funding struct {
	InstID          string
	Rate            float64
	NextRate        float64
	FundingTime     int64
	NextFundingTime int64
	MinRate         float64
	MaxRate         float64
	TS              int64
}

// MarkPrice is one row of public/mark-price.
type MarkPrice struct {
	InstID string
	MarkPx float64
	TS     int64
}

// IndexTicker is one row of market/index-tickers (instId like "BTC-USDT").
type IndexTicker struct {
	InstID string
	IdxPx  float64
	TS     int64
}

// Candle is one OKX bar. Newest first in API responses.
type Candle struct {
	TS          int64   `json:"ts"`
	Open        float64 `json:"o"`
	High        float64 `json:"h"`
	Low         float64 `json:"l"`
	Close       float64 `json:"c"`
	Vol         float64 `json:"vol"`      // contracts (SWAP)
	VolCcy      float64 `json:"volCcy"`   // base coin
	VolCcyQuote float64 `json:"volQuote"` // quote currency
	Confirm     bool    `json:"confirm"`
}

// Level is one price level, size in contracts.
type Level struct {
	Px float64
	Sz float64
}

// Book is a REST order-book snapshot. Bids descend, asks ascend. A side with
// exactly Requested levels may be truncated: the venue had more.
type Book struct {
	TS        int64
	Requested int
	Bids      []Level
	Asks      []Level
}

// Truncated reports whether a side hit the requested size.
func (b Book) Truncated() (bids, asks bool) {
	return b.Requested > 0 && len(b.Bids) >= b.Requested, b.Requested > 0 && len(b.Asks) >= b.Requested
}
