package venue

import (
	"testing"

	"screener/internal/metrics"
)

func TestSplitMult(t *testing.T) {
	cases := []struct {
		in   string
		base string
		mult float64
	}{
		{"1000PEPE", "PEPE", 1000}, {"1000000MOG", "MOG", 1e6}, {"1MBABYDOGE", "BABYDOGE", 1e6}, {"10000SATS", "SATS", 1e4},
		{"SHIB1000", "SHIB", 1000}, {"BTC", "BTC", 1}, {"1INCH", "1INCH", 1}, {"0G", "0G", 1},
	}
	for _, c := range cases {
		if b, m := SplitMult(c.in); b != c.base || m != c.mult {
			t.Errorf("SplitMult(%q) = %q, %v; want %q, %v", c.in, b, m, c.base, c.mult)
		}
	}
}

func TestCanonicalAndKey(t *testing.T) {
	cases := []struct {
		class, in, base, key string
		mult                 float64
	}{
		{ClassStock, "AMDSTOCK", "AMD", "t:AMD", 1},    // Bybit's suffix where a coin holds the ticker
		{ClassCommodity, "NATGAS", "NG", "t:NG", 1},    // Binance's name for OKX's NG
		{ClassCommodity, "COPPER", "XCU", "t:XCU", 1},  //
		{ClassStock, "PAYP", "PAYP", "t:PAYP", 1},      // not PayPal: another paper, another price
		{ClassStock, "CAT", "CAT", "t:CAT", 1},         // Caterpillar
		{ClassCrypto, "1000CAT", "CAT", "c:CAT", 1000}, // Simon's Cat: same ticker, another asset
		{ClassStock, "1000X", "1000X", "t:1000X", 1},   // multipliers are a coin convention only
		{ClassCrypto, "1000000BABYDOGE", "BABYDOGE", "c:BABYDOGE", 1e6},
	}
	for _, c := range cases {
		b, m := Canonical(c.class, c.in)
		if b != c.base || m != c.mult || Key(c.class, b) != c.key {
			t.Errorf("Canonical(%s, %q) = %q, %v, key %q; want %q, %v, %q", c.class, c.in, b, m, Key(c.class, b), c.base, c.mult, c.key)
		}
	}
}

func TestSectorFromTags(t *testing.T) {
	if s := SectorFromTags([]string{"Crypto", "DeFi", "Meme"}); s != "Meme" {
		t.Errorf("priority: got %q, want Meme", s)
	}
	if s := SectorFromTags([]string{"Crypto", "Alpha"}); s != "" {
		t.Errorf("no sector tag: got %q", s)
	}
}

func TestMergeBars(t *testing.T) {
	bar := func(ts int64, c float64) metrics.Bar { return metrics.Bar{TS: ts, C: c} }
	old := []metrics.Bar{bar(30, 3), bar(20, 2), bar(10, 1)}
	// fresh bars overlap the stored ones: the forming bar is replaced, a new one comes on top
	got := mergeBars(old, []metrics.Bar{bar(40, 4), bar(30, 3.5)}, 10, 10)
	if len(got) != 4 || got[0].TS != 40 || got[1].C != 3.5 || got[3].TS != 10 {
		t.Fatalf("overlap: %+v", got)
	}
	// a hole between stored and fresh: history is dropped so the next pass reloads it
	got = mergeBars(old, []metrics.Bar{bar(70, 7), bar(60, 6)}, 10, 10)
	if len(got) != 2 || got[0].TS != 70 {
		t.Fatalf("hole: %+v", got)
	}
	if got = mergeBars(old, []metrics.Bar{bar(40, 4)}, 10, 2); len(got) != 2 {
		t.Fatalf("cap: %+v", got)
	}
}
