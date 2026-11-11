// Package venue puts Binance and Bybit next to the OKX collector and folds the
// three exchanges into one row per asset: a block of figures per exchange
// (price, 24h change, volume, open interest, funding) plus the asset-level
// price metrics. OKX figures and metrics come from the collector's rows; for
// assets OKX does not list the price metrics are computed here from the
// candles of Bybit or Binance with the same formulas.
//
// Only linear USDT / USDC perpetuals are read, the same universe the OKX
// collector covers. Tickers differ between exchanges and are brought to one
// form first: see Canonical.
package venue

import (
	"regexp"
	"strconv"
	"strings"
)

// Exchange ids as they appear in the API (keys of Asset.V).
const (
	OKX     = "okx"
	Binance = "bn"
	Bybit   = "bb"
)

// Venues lists the exchanges in display order.
var Venues = []string{OKX, Binance, Bybit}

// Asset classes shared by the three exchanges. All of them list stocks and
// commodities as perpetuals; FX only Binance and Bybit.
const (
	ClassCrypto    = "crypto"
	ClassStock     = "stock" // equities, ETFs, equity indices, pre-IPO
	ClassCommodity = "commodity"
	ClassFX        = "fx"
)

// Instrument is one linear perpetual of an exchange, normalised.
type Instrument struct {
	Venue    string
	Symbol   string   // exchange symbol, e.g. 1000PEPEUSDT
	Base     string   // asset ticker without the contract multiplier, e.g. PEPE
	Mult     float64  // how many coins one unit of the exchange price stands for
	Quote    string   // USDT | USDC
	Class    string   // one of the Class* constants
	Kind     string   // Stock | ETF | Index | Pre-IPO | Commodity | FX; "" for a coin
	Tags     []string // the exchange's own sector tags, when it publishes them
	FundingH float64  // funding interval in hours; 0 when the listing does not say
}

// Key identifies the asset across exchanges.
func (i Instrument) Key() string { return Key(i.Class, i.Base) }

// Ticker is the fast-moving part of one instrument in the exchange's own
// units: prices are per exchange contract unit (not yet divided by Mult).
// A figure the feed does not carry is NaN.
type Ticker struct {
	Symbol   string
	Last     float64
	Mark     float64
	Chg24    float64 // fraction, 0.05 = +5%
	Vol24    float64 // quote currency
	OIUsd    float64
	Funding  float64 // rate per funding interval
	FundingH float64 // funding interval in hours; 0 when the feed has none
	Prev1h   float64 // price one hour ago
}

var (
	multPrefix = regexp.MustCompile(`^(1000000|100000|10000|1000|1M)([A-Z0-9]+)$`)
	multSuffix = regexp.MustCompile(`^([A-Z]+)1000$`)
)

// SplitMult strips the contract multiplier some exchanges put into the ticker
// of cheap coins: 1000PEPE, 1000000MOG, 1MBABYDOGE (Binance, Bybit) and
// SHIB1000 (Bybit). It returns the plain ticker and the multiplier, so that
// price / multiplier is the price of one coin on every exchange.
func SplitMult(base string) (string, float64) {
	if m := multPrefix.FindStringSubmatch(base); m != nil {
		if m[1] == "1M" {
			return m[2], 1e6
		}
		f, _ := strconv.ParseFloat(m[1], 64)
		return m[2], f
	}
	if m := multSuffix.FindStringSubmatch(base); m != nil {
		return m[1], 1000
	}
	return base, 1
}

// tradfiAlias maps the names one exchange uses for a stock or a commodity to
// the ticker the others use.
var tradfiAlias = map[string]string{"COPPER": "XCU", "NATGAS": "NG", "PAYP": "PYPL"}

// Canonical brings an exchange's base ticker to the common one and returns the
// contract multiplier. Coins lose the multiplier prefix; stocks and
// commodities lose Bybit's STOCK suffix (AMDSTOCK, added where a coin already
// holds the ticker) and exchange-specific names.
func Canonical(class, base string) (string, float64) {
	if class == ClassCrypto {
		return SplitMult(base)
	}
	if a, ok := tradfiAlias[base]; ok {
		return a, 1
	}
	if strings.HasSuffix(base, "STOCK") && len(base) > 5 {
		return strings.TrimSuffix(base, "STOCK"), 1
	}
	return base, 1
}

// Key is the asset id: the ticker with a class prefix, because the same
// ticker can be a coin on one exchange and a stock on another (BB, CAT, ON,
// QNT).
func Key(class, base string) string {
	if class == ClassCrypto {
		return "c:" + base
	}
	return "t:" + base
}

// indexTickers are equity and crypto indices listed without an index flag.
var indexTickers = map[string]bool{"US100": true, "US500": true, "JP225": true, "KR200": true, "H100": true, "BTCDOM": true, "ALL": true, "PREDICT": true}

// kindOrder is the priority when exchanges describe one asset differently.
var kindOrder = []string{"Index", "ETF", "Pre-IPO", "Commodity", "FX", "Stock"}

// tagSector maps Binance's sector tags onto the screener's sectors, in the
// priority order of the resolver; used only for coins the metadata resolver
// has not placed yet.
var tagSector = []struct{ tag, sector string }{
	{"Meme", "Meme"}, {"Layer-2", "L2"}, {"Layer-1", "L1"}, {"AI", "AI"}, {"RWA", "RWA"}, {"Storage", "DePIN"},
	{"Gaming", "Gaming"}, {"Metaverse", "Gaming"}, {"NFT", "Gaming"}, {"DeFi", "DeFi"}, {"DEX", "DeFi"}, {"Infrastructure", "Infra"},
}

// SectorFromTags picks a sector from an exchange's tags, "" when none fits.
func SectorFromTags(tags []string) string {
	for _, ts := range tagSector {
		for _, t := range tags {
			if t == ts.tag {
				return ts.sector
			}
		}
	}
	return ""
}
