package bench_test

import (
	"context"
	"io"
	"log"
	"math"
	"os"
	"testing"
	"time"

	"degen/pkg/bench"
)

// The strategies and account packages log verbosely on every order/fill; silence
// them for the whole test binary.
func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

func ladderCfg() bench.Config {
	return bench.Config{
		Symbol:       "BTCUSDT",
		Base:         "BTC",
		Quote:        "USDT",
		StartPrice:   50000,
		Spread:       0.0005,
		Ticker:       bench.Ticker{Open: 50000, High: 52000, Low: 49000, Close: 51000},
		Ticks:        1440,
		Runs:         24,
		BaseSeed:     1,
		MakerFee:     0.001,
		StartBase:    1.0,
		StartQuote:   50000,
		PriceTick:    0.01,
		QuantityTick: 0.0001,
		MinQuantity:  0.0001,
	}
}

func TestRun_Ladder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	factory := bench.LadderFactory(bench.UniformLadderConfig(3, 0.5, 0.002, 0.005))
	res, err := bench.Run(ctx, ladderCfg(), factory)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(res.Runs) != 24 {
		t.Fatalf("expected 24 runs, got %d", len(res.Runs))
	}

	t.Logf("PnL mean=%.3f%% std=%.3f%% median=%.3f%% min=%.3f%% max=%.3f%% profitable=%.0f%%",
		res.MeanPnLPct, res.StdPnLPct, res.MedianPnLPct, res.MinPnLPct, res.MaxPnLPct,
		res.ProfitableFraction*100)
	t.Logf("swing mean=%.2f%% target=%.2f%% mean fills=%.0f",
		res.MeanSwingPct, res.TargetSwingPct, res.MeanFills)

	// The mean simulated swing should track the symbol's actual 24h swing
	// (High/Low - 1 = ~6.1%) reasonably closely even over a modest seed count.
	if res.MeanSwingPct <= 0 {
		t.Errorf("expected positive mean swing, got %v", res.MeanSwingPct)
	}
	rel := (res.MeanSwingPct - res.TargetSwingPct) / res.TargetSwingPct
	if rel < -0.25 || rel > 0.25 {
		t.Errorf("mean swing %.2f%% too far from target %.2f%% (%.1f%%)",
			res.MeanSwingPct, res.TargetSwingPct, rel*100)
	}

	// The strategy must actually trade against a 6% swinging market.
	if res.MeanFills <= 0 {
		t.Errorf("expected some fills, got mean %.2f", res.MeanFills)
	}
}

// TestRun_Determinism uses a HIGH-volatility config (many fills) so that any
// non-determinism in fill ordering or strategy decisions would surface, and
// asserts byte-for-byte equality of every per-seed result field across two
// identical invocations. This guards the order-ID seeding fix.
func TestRun_Determinism(t *testing.T) {
	ctx := context.Background()
	factory := bench.LadderFactory(bench.UniformLadderConfig(3, 0.5, 0.002, 0.005))

	cfg := ladderCfg()
	cfg.Ticker = bench.Ticker{Open: 50000, High: 60000, Low: 42000, Close: 50000} // ~43% swing
	cfg.Runs = 8

	a, err := bench.Run(ctx, cfg, factory)
	if err != nil {
		t.Fatal(err)
	}
	b, err := bench.Run(ctx, cfg, factory)
	if err != nil {
		t.Fatal(err)
	}

	// The config must actually exercise heavy trading or the test proves nothing.
	if a.MeanFills < 5 {
		t.Fatalf("determinism test config too quiet: mean fills %.1f", a.MeanFills)
	}

	if a.MeanPnLPct != b.MeanPnLPct || a.MeanFills != b.MeanFills {
		t.Errorf("non-deterministic aggregate: mean PnL %v vs %v, fills %v vs %v",
			a.MeanPnLPct, b.MeanPnLPct, a.MeanFills, b.MeanFills)
	}
	for i := range a.Runs {
		x, y := a.Runs[i], b.Runs[i]
		if x.PnLPct != y.PnLPct || x.RealizedPnL != y.RealizedPnL ||
			x.FinalBase != y.FinalBase || x.FinalQuote != y.FinalQuote ||
			x.Fills != y.Fills || x.SwingPct != y.SwingPct {
			t.Errorf("seed %d non-deterministic:\n  %+v\n  %+v", x.Seed, x, y)
		}
	}
}

// TestRunOne_SeedZeroHonored ensures seed 0 is a real, distinct seed and not
// silently rewritten to 1.
func TestRunOne_SeedZeroHonored(t *testing.T) {
	ctx := context.Background()
	factory := bench.LadderFactory(bench.UniformLadderConfig(3, 0.5, 0.002, 0.005))
	cfg := ladderCfg()

	r0, err := bench.RunOne(ctx, cfg, 0, factory)
	if err != nil {
		t.Fatal(err)
	}
	r1, err := bench.RunOne(ctx, cfg, 1, factory)
	if err != nil {
		t.Fatal(err)
	}

	if r0.Seed != 0 {
		t.Errorf("seed 0 not honored, RunResult.Seed = %d", r0.Seed)
	}
	if r0.SwingPct == r1.SwingPct && r0.PnLPct == r1.PnLPct {
		t.Error("seed 0 produced identical path to seed 1; seed 0 is likely aliased")
	}
}

// TestRun_NoNegativeBalances drives an aggressive (10-level, full-allocation)
// config through a high-swing market and asserts the balance-coverage guard
// keeps both legs non-negative (no phantom leverage).
func TestRun_NoNegativeBalances(t *testing.T) {
	ctx := context.Background()
	factory := bench.LadderFactory(bench.UniformLadderConfig(10, 1.0, 0.002, 0.005))

	cfg := ladderCfg()
	cfg.Ticker = bench.Ticker{Open: 50000, High: 60000, Low: 42000, Close: 50000}
	cfg.Runs = 30

	res, err := bench.Run(ctx, cfg, factory)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res.Runs {
		// Allow a tiny float epsilon from decimal->float conversion.
		if r.FinalBase < -1e-9 || r.FinalQuote < -1e-9 {
			t.Errorf("seed %d went negative (phantom leverage): base=%g quote=%g",
				r.Seed, r.FinalBase, r.FinalQuote)
		}
	}
	t.Logf("aggressive config: mean fills %.0f, mean PnL %.3f%%, min PnL %.3f%%",
		res.MeanFills, res.MeanPnLPct, res.MinPnLPct)
}

func TestRun_HigherVolatilityTradesMore(t *testing.T) {
	ctx := context.Background()
	factory := bench.LadderFactory(bench.UniformLadderConfig(3, 0.5, 0.002, 0.005))

	calm := ladderCfg()
	calm.Ticker = bench.Ticker{Open: 50000, High: 50250, Low: 49750, Close: 50000} // ~1% swing
	calm.Runs = 16

	wild := ladderCfg()
	wild.Ticker = bench.Ticker{Open: 50000, High: 57500, Low: 43500, Close: 50000} // ~32% swing
	wild.Runs = 16

	calmRes, err := bench.Run(ctx, calm, factory)
	if err != nil {
		t.Fatal(err)
	}
	wildRes, err := bench.Run(ctx, wild, factory)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("calm: swing=%.2f%% fills=%.0f | wild: swing=%.2f%% fills=%.0f",
		calmRes.MeanSwingPct, calmRes.MeanFills, wildRes.MeanSwingPct, wildRes.MeanFills)

	if wildRes.MeanSwingPct <= calmRes.MeanSwingPct {
		t.Errorf("wild swing %.2f%% should exceed calm swing %.2f%%",
			wildRes.MeanSwingPct, calmRes.MeanSwingPct)
	}
	if wildRes.MeanFills <= calmRes.MeanFills {
		t.Errorf("wild market should produce more fills (%.0f) than calm (%.0f)",
			wildRes.MeanFills, calmRes.MeanFills)
	}
}

func TestRun_ValidationErrors(t *testing.T) {
	ctx := context.Background()
	factory := bench.LadderFactory(bench.UniformLadderConfig(3, 0.5, 0.002, 0.005))

	cases := []struct {
		name string
		mut  func(c *bench.Config)
	}{
		{"zero StartPrice", func(c *bench.Config) { c.StartPrice = 0 }},
		{"High < Low", func(c *bench.Config) { c.Ticker = bench.Ticker{High: 1, Low: 2} }},
		{"NaN StartPrice", func(c *bench.Config) { c.StartPrice = math.NaN() }},
		{"Inf Spread", func(c *bench.Config) { c.Spread = math.Inf(1) }},
		{"Spread >= 2", func(c *bench.Config) { c.Spread = 2.5 }},
		{"NaN MakerFee", func(c *bench.Config) { c.MakerFee = math.NaN() }},
		{"extreme High/Low", func(c *bench.Config) { c.Ticker = bench.Ticker{Open: 1, High: 1e9, Low: 1, Close: 1} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bad := ladderCfg()
			tc.mut(&bad)
			if _, err := bench.Run(ctx, bad, factory); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}

	if _, err := bench.RunOne(ctx, ladderCfg(), 1, nil); err == nil {
		t.Error("expected error for nil factory")
	}
}

// TestRun_RejectsBadLadderConfig verifies an out-of-range ladder allocation is
// rejected rather than silently producing over-leveraged PnL.
func TestRun_RejectsBadLadderConfig(t *testing.T) {
	ctx := context.Background()
	factory := bench.LadderFactory(bench.UniformLadderConfig(3, 2.0, 0.002, 0.005)) // alloc > 1
	if _, err := bench.Run(ctx, ladderCfg(), factory); err == nil {
		t.Error("expected error for over-allocated (alloc=2.0) ladder config")
	}
}
