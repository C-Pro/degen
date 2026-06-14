package bench

import (
	"math"
	"math/rand"
	"testing"
)

// TestCandleMids_RespectsOHLC checks the core invariant: within each candle the
// generated path opens at Open, closes at Close, and its min/max equal the
// candle's Low/High exactly, for every seed.
func TestCandleMids_RespectsOHLC(t *testing.T) {
	cases := []Candle{
		{Open: 100, High: 110, Low: 95, Close: 108},      // up bar
		{Open: 108, High: 109, Low: 90, Close: 92},       // down bar
		{Open: 50, High: 50, Low: 50, Close: 50},         // flat (degenerate)
		{Open: 9022, High: 9075, Low: 9022, Close: 9075}, // real WLD-IDR bar (low==open)
		{Open: 8575, High: 8600, Low: 8559, Close: 8587},
	}
	const k = 30
	for ci, c := range cases {
		for seed := int64(1); seed <= 25; seed++ {
			mids := candleMids(c, k, rand.New(rand.NewSource(seed)))
			if len(mids) != k {
				t.Fatalf("candle %d seed %d: got %d mids, want %d", ci, seed, len(mids), k)
			}
			if mids[0] != c.Open {
				t.Errorf("candle %d seed %d: open %v != %v", ci, seed, mids[0], c.Open)
			}
			if mids[k-1] != c.Close {
				t.Errorf("candle %d seed %d: close %v != %v", ci, seed, mids[k-1], c.Close)
			}
			lo, hi := math.Inf(1), math.Inf(-1)
			for _, m := range mids {
				lo = math.Min(lo, m)
				hi = math.Max(hi, m)
			}
			if lo != c.Low {
				t.Errorf("candle %d seed %d: min %v != Low %v", ci, seed, lo, c.Low)
			}
			if hi != c.High {
				t.Errorf("candle %d seed %d: max %v != High %v", ci, seed, hi, c.High)
			}
		}
	}
}

// TestCandleWalk_AggregateSwing verifies the full path reproduces the candle
// history's overall high/low (so every seed's realized swing equals the target)
// and yields the expected number of ticks.
func TestCandleWalk_AggregateSwing(t *testing.T) {
	candles := []Candle{
		{Open: 100, High: 105, Low: 98, Close: 103},
		{Open: 103, High: 112, Low: 101, Close: 109},
		{Open: 109, High: 110, Low: 96, Close: 99},
	}
	cfg := Config{Candles: candles, TicksPerCandle: 20, Spread: 0.001}
	agg := aggregateTicker(candles)
	if agg.High != 112 || agg.Low != 96 {
		t.Fatalf("aggregate H/L = %v/%v, want 112/96", agg.High, agg.Low)
	}

	m := NewCandleWalk(cfg)
	for seed := int64(1); seed <= 10; seed++ {
		bbos := m.Generate(rand.New(rand.NewSource(seed)))
		if len(bbos) != len(candles)*20 {
			t.Fatalf("got %d ticks, want %d", len(bbos), len(candles)*20)
		}
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, b := range bbos {
			mid := b.Midprice().InexactFloat64()
			lo = math.Min(lo, mid)
			hi = math.Max(hi, mid)
		}
		// Path min/max must equal the aggregate Low/High (extremes are pinned).
		if math.Abs(hi-agg.High) > 1e-6 || math.Abs(lo-agg.Low) > 1e-6 {
			t.Errorf("seed %d: path range [%v,%v] != aggregate [%v,%v]", seed, lo, hi, agg.Low, agg.High)
		}
	}
}
