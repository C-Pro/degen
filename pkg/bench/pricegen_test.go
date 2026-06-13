package bench

import (
	"math"
	"math/rand"
	"testing"
)

// meanLogRange returns the average high-low log-range of the simulated path over
// `runs` seeds for the given model.
func meanLogRange(m PriceModel, runs int) float64 {
	sum := 0.0
	for s := 0; s < runs; s++ {
		bbos := m.Generate(rand.New(rand.NewSource(int64(s) + 1)))
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, b := range bbos {
			mid := b.Midprice().InexactFloat64()
			if mid < lo {
				lo = mid
			}
			if mid > hi {
				hi = mid
			}
		}
		sum += math.Log(hi / lo)
	}
	return sum / float64(runs)
}

func TestStepSigma_Degenerate(t *testing.T) {
	if got := stepSigma(0, 1000); got != 0 {
		t.Errorf("stepSigma with zero range = %v, want 0", got)
	}
	if got := stepSigma(0.1, 1); got != 0 {
		t.Errorf("stepSigma with n<2 = %v, want 0", got)
	}
	// Larger target range must yield larger per-step sigma.
	if stepSigma(0.2, 1000) <= stepSigma(0.1, 1000) {
		t.Errorf("stepSigma not monotonic in target range")
	}
	// More steps over the same range must yield smaller per-step sigma.
	if stepSigma(0.1, 4000) >= stepSigma(0.1, 1000) {
		t.Errorf("stepSigma not decreasing in step count")
	}
}

// TestPriceModel_ExpectedRangeMatchesTarget is the core calibration check: the
// mean simulated 24h high-low log-range across seeds should match the ticker's
// ln(High/Low) within a few percent.
func TestPriceModel_ExpectedRangeMatchesTarget(t *testing.T) {
	cases := []struct {
		name             string
		high, low, start float64
		ticks            int
	}{
		{"15pct_swing", 1.15, 1.00, 1.05, 2000},
		{"1pct_swing", 1.01, 1.00, 1.005, 2000},
		{"50pct_swing", 1.50, 1.00, 1.20, 2000},
		{"coarse_window", 1.20, 1.00, 1.10, 288}, // 5-min ticks over 24h
	}

	const runs = 800
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{
				StartPrice: tc.start,
				Ticks:      tc.ticks,
				Ticker:     Ticker{High: tc.high, Low: tc.low},
			}
			m := NewPriceModel(cfg)
			target := math.Log(tc.high / tc.low)
			got := meanLogRange(m, runs)
			rel := math.Abs(got-target) / target
			t.Logf("mean log-range %.5f vs target %.5f (%.2f%% off, sigmaStep=%.6f)",
				got, target, rel*100, m.SigmaStep())
			if rel > 0.06 {
				t.Errorf("mean log-range %.5f vs target %.5f: %.2f%% off (>6%%)", got, target, rel*100)
			}
		})
	}
}

// TestPriceModel_RelativeAmplitudeScales encodes the motivating example: a coin
// that swung ~15% over 24h must produce paths that wander roughly an order of
// magnitude more than a stablecoin that swung ~1%, purely from each one's OHLC.
func TestPriceModel_RelativeAmplitudeScales(t *testing.T) {
	const runs = 600
	doge := NewPriceModel(Config{StartPrice: 0.15, Ticks: 1440, Ticker: Ticker{High: 0.165, Low: 0.1435}})
	usdt := NewPriceModel(Config{StartPrice: 1.0, Ticks: 1440, Ticker: Ticker{High: 1.005, Low: 0.995}})

	dogeSwing := meanLogRange(doge, runs)
	usdtSwing := meanLogRange(usdt, runs)
	ratio := dogeSwing / usdtSwing

	// Targets: ln(0.165/0.1435) ~ 0.1396, ln(1.005/0.995) ~ 0.01001; ratio ~ 13.9.
	targetRatio := math.Log(0.165/0.1435) / math.Log(1.005/0.995)
	t.Logf("doge swing %.4f, usdt swing %.4f, ratio %.2f (target ~%.2f)",
		dogeSwing, usdtSwing, ratio, targetRatio)
	if ratio < 0.8*targetRatio || ratio > 1.2*targetRatio {
		t.Errorf("amplitude ratio %.2f not within 20%% of target %.2f", ratio, targetRatio)
	}
}

// TestPriceModel_FlatWhenNoRange verifies a symbol with High==Low produces a
// perfectly flat path (no swings) rather than an error or NaN.
func TestPriceModel_FlatWhenNoRange(t *testing.T) {
	m := NewPriceModel(Config{StartPrice: 1.0, Ticks: 100, Ticker: Ticker{High: 1.0, Low: 1.0}})
	if m.SigmaStep() != 0 {
		t.Fatalf("expected zero sigma for flat ticker, got %v", m.SigmaStep())
	}
	bbos := m.Generate(rand.New(rand.NewSource(1)))
	first := bbos[0].Midprice()
	for i, b := range bbos {
		if !b.Midprice().Equal(first) {
			t.Fatalf("tick %d midprice %s != %s (expected flat)", i, b.Midprice(), first)
		}
	}
}
