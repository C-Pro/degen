// Package bench provides a Monte-Carlo harness for evaluating trading
// strategies against a random-walk "dummy" exchange.
//
// Given a symbol's 24h OHLC ticker, a start price and a bid-ask spread, the
// harness simulates price paths whose expected 24h high-low swing matches the
// symbol's actual amplitude (see PriceModel / Parkinson calibration in
// pricegen.go). It then runs a strategy over many independent seeds, fills the
// strategy's resting orders with a maker-fill engine (matcher.go) and reports
// the distribution of portfolio PnL percentage across runs.
//
// The simulation is driftless: the direction of the real 24h move is not
// reproduced, only its swing magnitude. A high-volatility coin (e.g. a 15% 24h
// range) therefore churns the strategy far harder than a stablecoin (~1%),
// purely from its OHLC.
package bench

import (
	"context"
	"fmt"
	"math"
	"math/rand" // nosemgrep: deterministic Monte-Carlo seeding, not security-sensitive
	"sort"
	"time"

	"degen/pkg/account"
	"degen/pkg/connectors/dummy"
	"degen/pkg/models"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	defaultLevelSize = 10.0
	defaultPriceTick = 0.01
	defaultQtyTick   = 0.0001
	defaultMinQty    = 0.0001
	// defaultTicks is one tick per minute over a 24h window.
	defaultTicks = 1440
	defaultRuns  = 200
	// maxSwingRatio caps Ticker.High/Ticker.Low. Beyond this the calibrated
	// per-step sigma grows large enough that exp(logP) can overflow to +Inf and
	// panic decimal.NewFromFloat; no real 24h market comes close.
	maxSwingRatio = 1e4
	// uuidSeedOffset derives the deterministic order-ID stream's seed from the
	// run seed, keeping it independent of the price-walk stream.
	uuidSeedOffset int64 = 0x5DEECE66D
)

// Strategy is the minimal contract a strategy must satisfy to be benchmarked.
// Both *strategies.Ladder and *strategies.Monkey already satisfy it.
type Strategy interface {
	See(models.ExchangeMessage)
}

// StrategyFactory builds a fresh strategy bound to a freshly-seeded account. It
// is invoked once per run so that runs never share mutable state. Returning a
// nil strategy or an error aborts the run.
type StrategyFactory func(ctx context.Context, acc *account.Account, symbol string) (Strategy, error)

// Ticker is the 24h OHLC snapshot used to calibrate swing amplitude. Only High
// and Low drive the calibration; Open and Close are informational.
type Ticker struct {
	Open  float64
	High  float64
	Low   float64
	Close float64
}

// Config defines a single benchmark scenario.
type Config struct {
	Symbol string // e.g. "BTCUSDT"
	Base   string // base asset, e.g. "BTC"
	Quote  string // quote asset, e.g. "USDT"

	StartPrice float64 // price the random walk starts at
	Spread     float64 // relative bid-ask spread, (ask-bid)/mid
	Ticker     Ticker  // 24h OHLC; High/Low calibrate the swing amplitude

	Ticks    int   // price samples per run (granularity of the 24h window)
	Runs     int   // number of seeds to average over
	BaseSeed int64 // seeds used are BaseSeed, BaseSeed+1, ...

	MakerFee    float64 // maker fee fraction applied to every fill (0 = no fee)
	SellTaxRate float64 // extra withholding levied on sell proceeds only (e.g. PPh); 0 = none
	StartBase   float64 // initial base-asset balance
	StartQuote  float64 // initial quote-asset balance

	PriceTick    float64 // price tick size (must be > 0)
	QuantityTick float64 // quantity tick size (must be > 0)
	MinQuantity  float64 // minimum order quantity
	BBOLevelSize float64 // displayed size on each generated BBO level (cosmetic)

	// Candles, when non-empty, switches the price model from the single-ticker
	// random walk to replaying this OHLC history (CandleWalk). Ticker/Ticks are
	// then derived/ignored; the path has len(Candles)*TicksPerCandle samples.
	Candles        []Candle
	TicksPerCandle int // sub-ticks per candle in candle mode (default 30)
}

// candleMode reports whether the config drives the candle-following price model.
func (c Config) candleMode() bool { return len(c.Candles) > 0 }

// withDefaults returns a copy of the config with unset structural fields filled
// in. MakerFee is intentionally not defaulted: a zero value means "no fee".
func (c Config) withDefaults() Config {
	if c.Symbol == "" {
		c.Symbol = "SIM"
	}
	if c.Base == "" {
		c.Base = "BASE"
	}
	if c.Quote == "" {
		c.Quote = "QUOTE"
	}
	if c.Ticks == 0 {
		c.Ticks = defaultTicks
	}
	if c.Runs == 0 {
		c.Runs = defaultRuns
	}
	// BaseSeed is intentionally NOT defaulted: 0 is a legal RNG seed and must be
	// distinguishable from any other. The CLI applies its own default.
	if c.PriceTick == 0 {
		c.PriceTick = defaultPriceTick
	}
	if c.QuantityTick == 0 {
		c.QuantityTick = defaultQtyTick
	}
	if c.MinQuantity == 0 {
		c.MinQuantity = defaultMinQty
	}
	if c.BBOLevelSize == 0 {
		c.BBOLevelSize = defaultLevelSize
	}
	if c.candleMode() {
		// Derive the missing scenario fields from the candle history.
		if c.TicksPerCandle == 0 {
			c.TicksPerCandle = defaultTicksPerCandle
		}
		if c.StartPrice == 0 {
			c.StartPrice = c.Candles[0].Open
		}
		if c.Ticker == (Ticker{}) {
			c.Ticker = aggregateTicker(c.Candles)
		}
	}
	if c.StartBase == 0 {
		c.StartBase = 1.0
	}
	if c.StartQuote == 0 {
		// Default to a balanced 50/50 inventory around the start price so a
		// market-making strategy has something to quote on both sides.
		c.StartQuote = c.StartPrice
	}
	return c
}

func (c Config) validate() error {
	// Reject non-finite floats first: the order/sign comparisons below are all
	// false for NaN, so a NaN/Inf would otherwise slip through and panic
	// decimal.NewFromFloat deep inside the simulation.
	floats := []struct {
		name string
		v    float64
	}{
		{"StartPrice", c.StartPrice}, {"Spread", c.Spread}, {"MakerFee", c.MakerFee},
		{"SellTaxRate", c.SellTaxRate},
		{"StartBase", c.StartBase}, {"StartQuote", c.StartQuote},
		{"PriceTick", c.PriceTick}, {"QuantityTick", c.QuantityTick}, {"MinQuantity", c.MinQuantity},
		{"Ticker.Open", c.Ticker.Open}, {"Ticker.High", c.Ticker.High},
		{"Ticker.Low", c.Ticker.Low}, {"Ticker.Close", c.Ticker.Close},
	}
	for _, f := range floats {
		if math.IsNaN(f.v) || math.IsInf(f.v, 0) {
			return fmt.Errorf("%s must be finite, got %v", f.name, f.v)
		}
	}

	switch {
	case c.StartPrice <= 0:
		return fmt.Errorf("StartPrice must be > 0, got %v", c.StartPrice)
	case c.Ticks < 2:
		return fmt.Errorf("Ticks must be >= 2, got %d", c.Ticks)
	case c.Runs < 1:
		return fmt.Errorf("Runs must be >= 1, got %d", c.Runs)
	case c.Spread < 0:
		return fmt.Errorf("Spread must be >= 0, got %v", c.Spread)
	case c.Spread >= 2:
		// bid = mid*(1 - Spread/2); Spread >= 2 yields a non-positive bid.
		return fmt.Errorf("Spread must be < 2, got %v", c.Spread)
	case c.MakerFee < 0 || c.MakerFee >= 1:
		return fmt.Errorf("MakerFee must be in [0, 1), got %v", c.MakerFee)
	case c.SellTaxRate < 0 || c.SellTaxRate >= 1:
		return fmt.Errorf("SellTaxRate must be in [0, 1), got %v", c.SellTaxRate)
	case c.Ticker.Low <= 0:
		return fmt.Errorf("Ticker.Low must be > 0, got %v", c.Ticker.Low)
	case c.Ticker.High < c.Ticker.Low:
		return fmt.Errorf("Ticker.High (%v) must be >= Ticker.Low (%v)", c.Ticker.High, c.Ticker.Low)
	case c.Ticker.High/c.Ticker.Low > maxSwingRatio:
		return fmt.Errorf("Ticker.High/Low ratio %v exceeds sane cap %v", c.Ticker.High/c.Ticker.Low, maxSwingRatio)
	case c.PriceTick <= 0:
		return fmt.Errorf("PriceTick must be > 0, got %v", c.PriceTick)
	case c.QuantityTick <= 0:
		return fmt.Errorf("QuantityTick must be > 0, got %v", c.QuantityTick)
	case c.StartBase < 0 || c.StartQuote < 0:
		return fmt.Errorf("StartBase/StartQuote must be >= 0")
	}

	for i, cd := range c.Candles {
		for _, v := range []float64{cd.Open, cd.High, cd.Low, cd.Close} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("candle %d has a non-finite OHLC value", i)
			}
		}
		switch {
		case cd.Low <= 0:
			return fmt.Errorf("candle %d Low must be > 0, got %v", i, cd.Low)
		case cd.High < cd.Low:
			return fmt.Errorf("candle %d High %v < Low %v", i, cd.High, cd.Low)
		case cd.High < cd.Open || cd.High < cd.Close:
			return fmt.Errorf("candle %d High %v below Open/Close", i, cd.High)
		case cd.Low > cd.Open || cd.Low > cd.Close:
			return fmt.Errorf("candle %d Low %v above Open/Close", i, cd.Low)
		}
	}

	return nil
}

// RunResult is the outcome of a single seed.
type RunResult struct {
	Seed        int64
	PnLPct      float64 // portfolio value change percent, base valued at final mid
	RealizedPnL float64 // realised PnL reported by the position structure
	SwingPct    float64 // realised (max-min)/min of the simulated midprice path, percent
	Fills       int
	FinalBase   float64
	FinalQuote  float64

	InitialValue float64 // base*startPrice + quote
	FinalValue   float64 // base*finalMid + quote
}

// Result aggregates RunResults across all seeds.
type Result struct {
	Config Config
	Runs   []RunResult

	MeanPnLPct   float64
	StdPnLPct    float64 // sample standard deviation
	MinPnLPct    float64
	MaxPnLPct    float64
	MedianPnLPct float64

	MeanSwingPct   float64 // mean realised swing across seeds (should track TargetSwingPct)
	TargetSwingPct float64 // (High/Low - 1) * 100, the symbol's actual 24h swing
	MeanFills      float64
	// ProfitableFraction is the share of seeds with PnLPct > 0.
	ProfitableFraction float64
}

// RunOne executes a single seeded simulation and returns its result.
func RunOne(ctx context.Context, cfg Config, seed int64, factory StrategyFactory) (RunResult, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return RunResult{}, err
	}
	if factory == nil {
		return RunResult{}, fmt.Errorf("nil strategy factory")
	}

	// Make order-ID generation deterministic for this run. The ladder/monkey
	// mint ClientOrderIDs via uuid.NewString(), which by default draws from
	// crypto/rand. Because account.GetOpenOrders returns orders ordered by that
	// ID, non-deterministic IDs make the fill order — and therefore the strategy's
	// re-quote decisions and the resulting PnL — irreproducible for the same
	// seed. The harness is single-threaded, so seeding the process-global uuid
	// source here is safe; we restore the crypto/rand default afterward.
	uuid.SetRand(rand.New(rand.NewSource(seed + uuidSeedOffset)))
	defer uuid.SetRand(nil)

	d := dummy.NewDummy(ctx, "", "", "", "")
	// Block the dummy's built-in random generator; we drive ticks synchronously
	// so the whole simulation is deterministic and race-free.
	d.Generator = func(ctx context.Context, _ *dummy.Dummy, _ chan<- models.ExchangeMessage) {
		<-ctx.Done()
	}

	d.SetSymbol(models.SymbolInfo{
		Symbol:           cfg.Symbol,
		Base:             cfg.Base,
		Quote:            cfg.Quote,
		PriceTickSize:    decimal.NewFromFloat(cfg.PriceTick),
		QuantityTickSize: decimal.NewFromFloat(cfg.QuantityTick),
		MinQuantity:      decimal.NewFromFloat(cfg.MinQuantity),
	})

	now := time.Now().UTC()
	d.SetBalance(models.Balance{
		Total:     decimal.NewFromFloat(cfg.StartBase),
		Available: decimal.NewFromFloat(cfg.StartBase),
		UpdatedAt: now,
	}, cfg.Base)
	d.SetBalance(models.Balance{
		Total:     decimal.NewFromFloat(cfg.StartQuote),
		Available: decimal.NewFromFloat(cfg.StartQuote),
		UpdatedAt: now,
	}, cfg.Quote)

	acc, err := account.NewAccount("bench", d)
	if err != nil {
		return RunResult{}, fmt.Errorf("create account: %w", err)
	}

	strat, err := factory(ctx, acc, cfg.Symbol)
	if err != nil {
		return RunResult{}, fmt.Errorf("create strategy: %w", err)
	}
	if strat == nil {
		return RunResult{}, fmt.Errorf("strategy factory returned nil")
	}

	// Pick the price model: replay real candle history when provided, otherwise
	// the single-ticker calibrated random walk.
	var gen interface {
		Generate(rng *rand.Rand) []models.BBO
	} = NewPriceModel(cfg)
	if cfg.candleMode() {
		gen = NewCandleWalk(cfg)
	}
	bbos := gen.Generate(rand.New(rand.NewSource(seed)))

	m := &matcher{
		d:        d,
		acc:      acc,
		symbol:   cfg.Symbol,
		base:     cfg.Base,
		quote:    cfg.Quote,
		makerFee: decimal.NewFromFloat(cfg.MakerFee),
		sellTax:  decimal.NewFromFloat(cfg.SellTaxRate),
	}

	minMid, maxMid := math.Inf(1), math.Inf(-1)
	for _, bbo := range bbos {
		select {
		case <-ctx.Done():
			return RunResult{}, ctx.Err()
		default:
		}

		mid := bbo.Midprice().InexactFloat64()
		if mid < minMid {
			minMid = mid
		}
		if mid > maxMid {
			maxMid = mid
		}

		// Fill any orders the new price crossed before letting the strategy
		// react to the same BBO (price moves, then the strategy re-quotes).
		m.match(bbo)
		strat.See(models.ExchangeMessage{
			Exchange:  dummy.Name,
			Symbol:    cfg.Symbol,
			MsgType:   models.MsgTypeBBO,
			Payload:   bbo,
			Timestamp: bbo.Timestamp,
		})
	}

	lastMid := bbos[len(bbos)-1].Midprice()
	finalBase := acc.GetBalance(cfg.Base).Total
	finalQuote := acc.GetBalance(cfg.Quote).Total
	finalValue := finalBase.Mul(lastMid).Add(finalQuote)
	initialValue := decimal.NewFromFloat(cfg.StartBase).
		Mul(decimal.NewFromFloat(cfg.StartPrice)).
		Add(decimal.NewFromFloat(cfg.StartQuote))

	pnlPct := 0.0
	if initialValue.IsPositive() {
		pnlPct = finalValue.Sub(initialValue).Div(initialValue).InexactFloat64() * 100
	}

	swingPct := 0.0
	if minMid > 0 && !math.IsInf(minMid, 0) && !math.IsInf(maxMid, 0) {
		swingPct = (maxMid/minMid - 1) * 100
	}

	return RunResult{
		Seed:         seed,
		PnLPct:       pnlPct,
		RealizedPnL:  acc.GetPosition(cfg.Symbol).RealizedPnL.InexactFloat64(),
		SwingPct:     swingPct,
		Fills:        m.fills,
		FinalBase:    finalBase.InexactFloat64(),
		FinalQuote:   finalQuote.InexactFloat64(),
		InitialValue: initialValue.InexactFloat64(),
		FinalValue:   finalValue.InexactFloat64(),
	}, nil
}

// Run executes cfg.Runs seeded simulations (seeds BaseSeed..BaseSeed+Runs-1) and
// returns the aggregated distribution of PnL percentage.
func Run(ctx context.Context, cfg Config, factory StrategyFactory) (Result, error) {
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return Result{}, err
	}

	res := Result{Config: cfg, Runs: make([]RunResult, 0, cfg.Runs)}
	for i := 0; i < cfg.Runs; i++ {
		seed := cfg.BaseSeed + int64(i)
		rr, err := RunOne(ctx, cfg, seed, factory)
		if err != nil {
			return Result{}, fmt.Errorf("run %d (seed %d): %w", i, seed, err)
		}
		res.Runs = append(res.Runs, rr)
	}

	res.summarize()
	return res, nil
}

// summarize fills in the aggregate statistics from res.Runs.
func (res *Result) summarize() {
	n := len(res.Runs)
	res.TargetSwingPct = 0
	if res.Config.Ticker.Low > 0 {
		res.TargetSwingPct = (res.Config.Ticker.High/res.Config.Ticker.Low - 1) * 100
	}
	if n == 0 {
		return
	}

	pnls := make([]float64, n)
	var sumPnL, sumSwing, sumFills float64
	profitable := 0
	res.MinPnLPct = math.Inf(1)
	res.MaxPnLPct = math.Inf(-1)

	for i, r := range res.Runs {
		pnls[i] = r.PnLPct
		sumPnL += r.PnLPct
		sumSwing += r.SwingPct
		sumFills += float64(r.Fills)
		if r.PnLPct > 0 {
			profitable++
		}
		if r.PnLPct < res.MinPnLPct {
			res.MinPnLPct = r.PnLPct
		}
		if r.PnLPct > res.MaxPnLPct {
			res.MaxPnLPct = r.PnLPct
		}
	}

	res.MeanPnLPct = sumPnL / float64(n)
	res.MeanSwingPct = sumSwing / float64(n)
	res.MeanFills = sumFills / float64(n)
	res.ProfitableFraction = float64(profitable) / float64(n)

	if n > 1 {
		var ss float64
		for _, p := range pnls {
			d := p - res.MeanPnLPct
			ss += d * d
		}
		res.StdPnLPct = math.Sqrt(ss / float64(n-1))
	}

	sort.Float64s(pnls)
	if n%2 == 1 {
		res.MedianPnLPct = pnls[n/2]
	} else {
		res.MedianPnLPct = (pnls[n/2-1] + pnls[n/2]) / 2
	}
}
