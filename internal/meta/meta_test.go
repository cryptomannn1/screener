package meta

import "testing"

func ip(v int) *int { return &v }

func TestResolvePrefersOverrideThenRank(t *testing.T) {
	coins := []coin{
		{ID: "bitcoin", Symbol: "btc", Rank: ip(1)},
		{ID: "big-titty-coin", Symbol: "btc", Rank: ip(1044)},
		{ID: "memetoon", Symbol: "meme", Rank: ip(433)},
		{ID: "memecoin-2", Symbol: "meme", Rank: ip(688)},
		{ID: "cross-2", Symbol: "one", Rank: ip(395)},
		{ID: "harmony", Symbol: "one", Rank: ip(931)},
		{ID: "no-rank", Symbol: "zzz"},
		{ID: "ranked", Symbol: "zzz", Rank: ip(1500)},
	}
	got := Resolve([]string{"BTC", "MEME", "ONE", "ZZZ", "NOPE"}, coins)
	want := map[string]string{"BTC": "bitcoin", "MEME": "memecoin-2", "ONE": "harmony", "ZZZ": "ranked"}
	for sym, id := range want {
		if got[sym].ID != id {
			t.Errorf("%s: got %q want %q", sym, got[sym].ID, id)
		}
	}
	if _, ok := got["NOPE"]; ok {
		t.Errorf("NOPE should be unmatched")
	}
}

func TestSectorOfPriorityAndOverrides(t *testing.T) {
	members := map[string]map[string]bool{
		"L1":   {"ethereum": true, "binancecoin": true, "near": true, "bittensor": true},
		"AI":   {"near": true, "bittensor": true},
		"DeFi": {"uniswap": true},
	}
	cases := []struct{ sym, id, want string }{
		{"ETH", "ethereum", "L1"},
		{"BNB", "binancecoin", "Exchange"}, // override beats the category order
		{"NEAR", "near", "L1"},             // L1 comes before AI in the taxonomy
		{"TAO", "bittensor", "AI"},         // override
		{"UNI", "uniswap", "DeFi"},
		{"XYZ", "xyz", SectorOther},
	}
	for _, c := range cases {
		if got := SectorOf(c.sym, c.id, members); got != c.want {
			t.Errorf("%s: got %s want %s", c.sym, got, c.want)
		}
	}
}

func TestTaxonomyCoversSectors(t *testing.T) {
	seen := map[string]bool{}
	for _, tx := range taxonomy {
		seen[tx.sector] = true
	}
	for _, s := range Sectors {
		if !seen[s] {
			t.Errorf("sector %s has no categories", s)
		}
	}
	for _, s := range sectorOverrides {
		if !seen[s] {
			t.Errorf("override sector %s is not in the taxonomy", s)
		}
	}
}
