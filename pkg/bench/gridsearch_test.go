package bench

import (
	"context"
	"testing"
)

func TestGridSearch(t *testing.T) {
	// Synthetic 7-ish-day candle history with mild trend and swings.
	var candles []Candle
	p := 1000.0
	for i := 0; i < 40; i++ {
		o := p
		hi := o * 1.02
		lo := o * 0.985
		c := o * (1.0 + 0.004*float64((i%5)-2)) // gentle oscillation
		candles = append(candles, Candle{Open: o, High: hi, Low: lo, Close: c})
		p = c
	}

	cfg := Config{
		Symbol: "X", Base: "X", Quote: "Y",
		Spread: 0.0005, MakerFee: 0.0012, SellTaxRate: 0.0021,
		PriceTick: 0.01, QuantityTick: 0.0001, MinQuantity: 0.0001,
		TicksPerCandle: 15, Runs: 5, BaseSeed: 1,
	}
	grid := TuneGrid{
		Levels:        []int{3},
		Allocations:   []float64{0.5},
		LevelSpreads:  []float64{0.004, 0.008, 0.016},
		ToleranceMult: []float64{1.5},
	}

	best, all, err := GridSearch(context.Background(), candles, cfg, grid)
	if err != nil {
		t.Fatalf("GridSearch: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("want 3 grid points, got %d", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].MeanPnLPct < all[i].MeanPnLPct {
			t.Errorf("results not sorted best-first: %v < %v", all[i-1].MeanPnLPct, all[i].MeanPnLPct)
		}
	}
	if best != all[0] {
		t.Errorf("best %+v != all[0] %+v", best, all[0])
	}
	if best.Tolerance != best.LevelSpread*1.5 {
		t.Errorf("tolerance %v != 1.5*spread %v", best.Tolerance, best.LevelSpread)
	}
	lc := best.LadderConfig()
	if lc.Validate() != nil {
		t.Errorf("best params produce an invalid ladder config")
	}

	if _, _, err := GridSearch(context.Background(), nil, cfg, grid); err == nil {
		t.Error("expected error for empty candles")
	}
}
