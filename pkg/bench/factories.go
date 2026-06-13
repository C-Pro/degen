package bench

import (
	"context"
	"fmt"

	"degen/pkg/account"
	"degen/pkg/strategies"

	"github.com/shopspring/decimal"
)

// LadderFactory returns a StrategyFactory that builds a *strategies.Ladder with
// the given config for each run. The config is validated on first use so that
// an out-of-range allocation (e.g. > 1) surfaces as a clear error instead of
// silently producing over-leveraged, meaningless PnL.
func LadderFactory(cfg strategies.LadderConfig) StrategyFactory {
	return func(ctx context.Context, acc *account.Account, symbol string) (Strategy, error) {
		if err := cfg.Validate(); err != nil {
			return nil, fmt.Errorf("invalid ladder config: %w", err)
		}
		l := strategies.NewLadder(ctx, acc, symbol, cfg)
		if l == nil {
			return nil, fmt.Errorf("ladder init failed for %s", symbol)
		}
		return l, nil
	}
}

// MonkeyFactory returns a StrategyFactory that builds a *strategies.Monkey for
// each run.
//
// Note: Monkey keeps at most one bid and one ask resting, and its cancels
// resolve synchronously against the dummy, so in this harness it stays well
// below the >4-open-orders threshold that would trigger Account.SyncWithExchange
// (a one-second sleep). That hazard exists in the live strategy but is not
// normally reached here.
func MonkeyFactory(orderNotional, spread decimal.Decimal) StrategyFactory {
	return func(ctx context.Context, acc *account.Account, symbol string) (Strategy, error) {
		mk := strategies.NewMonkey(ctx, acc, symbol, orderNotional, spread)
		if mk == nil {
			return nil, fmt.Errorf("monkey init failed for %s", symbol)
		}
		return mk, nil
	}
}

// UniformLadderConfig builds a LadderConfig with `levels` price levels per side,
// each separated by `levelSpread` (relative), equal size weighting, and a shared
// re-quote `tolerance`. It is a convenience for the CLI and tests.
func UniformLadderConfig(levels int, allocation, levelSpread, tolerance float64) strategies.LadderConfig {
	if levels < 1 {
		levels = 1
	}
	spreads := make([]decimal.Decimal, levels)
	sizes := make([]decimal.Decimal, levels)
	tols := make([]decimal.Decimal, levels)
	size := decimal.NewFromInt(1).Div(decimal.NewFromInt(int64(levels)))
	for i := 0; i < levels; i++ {
		spreads[i] = decimal.NewFromFloat(levelSpread)
		sizes[i] = size
		tols[i] = decimal.NewFromFloat(tolerance)
	}
	return strategies.LadderConfig{
		PortfolioAllocation:  decimal.NewFromFloat(allocation),
		LevelsCount:          levels,
		LevelsSpread:         spreads,
		LevelsSize:           sizes,
		LevelsPriceTolerance: tols,
	}
}
