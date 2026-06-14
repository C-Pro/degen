package bench

import (
	"context"
	"fmt"
	"sort"

	"degen/pkg/strategies"
)

// TuneGrid is the parameter grid GridSearch sweeps. Tolerance is expressed as a
// multiple of the level spread (kept >= 1 so resting orders are not re-quoted
// before the price can reach them).
type TuneGrid struct {
	Levels        []int
	Allocations   []float64 // fraction of the deployable budget to quote
	LevelSpreads  []float64
	ToleranceMult []float64
}

// DefaultTuneGrid returns a grid informed by the multi-token study: spreads from
// just above the round-trip fee floor out to "barely trades", a few allocation
// fractions, 3 levels, tolerance 1.5x the spread.
func DefaultTuneGrid() TuneGrid {
	return TuneGrid{
		Levels:        []int{3},
		Allocations:   []float64{0.25, 0.5, 1.0},
		LevelSpreads:  []float64{0.004, 0.006, 0.008, 0.012, 0.016, 0.024},
		ToleranceMult: []float64{1.5},
	}
}

// TuneResult is one grid point: its parameters and measured performance.
type TuneResult struct {
	Levels      int
	Allocation  float64
	LevelSpread float64
	Tolerance   float64
	MeanPnLPct  float64
	StdPnLPct   float64
	MeanFills   float64
}

// LadderConfig builds the ladder configuration for this result.
func (t TuneResult) LadderConfig() strategies.LadderConfig {
	return UniformLadderConfig(t.Levels, t.Allocation, t.LevelSpread, t.Tolerance)
}

// GridSearch replays the candle history over every grid point and returns the
// parameters with the highest mean PnL, plus all evaluated points sorted
// best-first. cfg supplies the fixed scenario (assets, fees, tick sizes,
// inventory, Runs, TicksPerCandle); cfg.Candles is overwritten with candles.
//
// It runs sequentially on purpose: Run mutates a process-global RNG (uuid seed)
// and is not safe to run concurrently.
func GridSearch(ctx context.Context, candles []Candle, cfg Config, grid TuneGrid) (TuneResult, []TuneResult, error) {
	if len(candles) == 0 {
		return TuneResult{}, nil, fmt.Errorf("GridSearch: no candles to tune on")
	}
	cfg.Candles = candles
	cfg.CandleSource = nil

	var all []TuneResult
	for _, levels := range grid.Levels {
		for _, alloc := range grid.Allocations {
			for _, spread := range grid.LevelSpreads {
				for _, mult := range grid.ToleranceMult {
					tol := spread * mult
					res, err := Run(ctx, cfg, LadderFactory(UniformLadderConfig(levels, alloc, spread, tol)))
					if err != nil {
						return TuneResult{}, nil, fmt.Errorf("GridSearch point (levels=%d alloc=%.3g spread=%.4g tol=%.4g): %w",
							levels, alloc, spread, tol, err)
					}
					all = append(all, TuneResult{
						Levels:      levels,
						Allocation:  alloc,
						LevelSpread: spread,
						Tolerance:   tol,
						MeanPnLPct:  res.MeanPnLPct,
						StdPnLPct:   res.StdPnLPct,
						MeanFills:   res.MeanFills,
					})
				}
			}
		}
	}

	sort.SliceStable(all, func(i, j int) bool { return all[i].MeanPnLPct > all[j].MeanPnLPct })
	return all[0], all, nil
}
