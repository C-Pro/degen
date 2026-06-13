// Command strategybench evaluates a trading strategy against a random-walk dummy
// exchange calibrated to a symbol's 24h OHLC swing amplitude, across many seeds,
// and reports the distribution of portfolio PnL percentage.
//
// Examples:
//
//	# Explicit OHLC:
//	strategybench -symbol DOGEUSDT -base DOGE -quote USDT \
//	  -price 0.15 -spread 0.0008 \
//	  -open 0.150 -high 0.165 -low 0.1435 -close 0.158 \
//	  -runs 500 -ticks 1440 -strategy ladder
//
//	# Symbol only: fetches symbol details + 24h OHLC from pintupro:
//	strategybench -symbol BTC-IDR -runs 500 -strategy ladder
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"degen/pkg/bench"
	"degen/pkg/connectors/pintupro"

	"github.com/shopspring/decimal"
)

func main() {
	cfg := bench.Config{}

	flag.StringVar(&cfg.Symbol, "symbol", "SIM", "trading symbol")
	flag.StringVar(&cfg.Base, "base", "BASE", "base asset")
	flag.StringVar(&cfg.Quote, "quote", "QUOTE", "quote asset")

	flag.Float64Var(&cfg.StartPrice, "price", 0, "start price of the random walk (required)")
	flag.Float64Var(&cfg.Spread, "spread", 0.0005, "relative bid-ask spread (ask-bid)/mid")

	flag.Float64Var(&cfg.Ticker.Open, "open", 0, "24h open (informational)")
	flag.Float64Var(&cfg.Ticker.High, "high", 0, "24h high (required; calibrates swing)")
	flag.Float64Var(&cfg.Ticker.Low, "low", 0, "24h low (required; calibrates swing)")
	flag.Float64Var(&cfg.Ticker.Close, "close", 0, "24h close (informational)")

	flag.IntVar(&cfg.Ticks, "ticks", 1440, "price samples per run (1440 = 1/min over 24h)")
	flag.IntVar(&cfg.Runs, "runs", 200, "number of seeds to average over")
	flag.Int64Var(&cfg.BaseSeed, "seed", 1, "base seed (runs use seed, seed+1, ...)")

	flag.Float64Var(&cfg.MakerFee, "fee", 0.001, "maker fee fraction per fill")
	flag.Float64Var(&cfg.StartBase, "base-bal", 0, "initial base balance (default 1.0)")
	flag.Float64Var(&cfg.StartQuote, "quote-bal", 0, "initial quote balance (default = price)")

	flag.Float64Var(&cfg.PriceTick, "price-tick", 0, "price tick size (default 0.01)")
	flag.Float64Var(&cfg.QuantityTick, "qty-tick", 0, "quantity tick size (default 0.0001)")
	flag.Float64Var(&cfg.MinQuantity, "min-qty", 0, "minimum order quantity (default 0.0001)")

	strategy := flag.String("strategy", "ladder", "strategy to benchmark: ladder|monkey")

	// Ladder parameters.
	levels := flag.Int("levels", 3, "ladder: price levels per side")
	alloc := flag.Float64("alloc", 0.5, "ladder: portfolio allocation [0,1]")
	levelSpread := flag.Float64("level-spread", 0.010, "ladder: relative spread between levels")
	tolerance := flag.Float64("tolerance", 0.005, "ladder: re-quote price tolerance")

	// Monkey parameters.
	notional := flag.Float64("notional", 0, "monkey: per-order notional (default = quote-bal*0.1)")
	monkeySpread := flag.Float64("monkey-spread", 0.004, "monkey: quote spread")

	apiURL := flag.String("api-url", "", "pintupro REST base URL for auto-fetching OHLC (default $PINTUPRO_API_BASE_URL or https://api.pintu.pro)")

	verbose := flag.Bool("v", false, "show strategy/account debug logs")
	perSeed := flag.Bool("per-seed", false, "print per-seed results")

	flag.Parse()

	if !*verbose {
		// Strategies and the account log on every order/fill; mute by default.
		log.SetOutput(io.Discard)
	}

	// Track which flags the user set explicitly so a fetch never clobbers them.
	set := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// When a real symbol is given but the OHLC isn't, fetch symbol details and
	// the 24h OHLC from pintupro. Explicitly-set flags still win.
	if set["symbol"] && (!set["high"] || !set["low"]) {
		if err := fetchFromPintu(ctx, &cfg, set, *apiURL); err != nil {
			fmt.Fprintf(os.Stderr, "error: failed to fetch %s from pintupro: %v\n", cfg.Symbol, err)
			fmt.Fprintln(os.Stderr, "hint: pass -high/-low (and -price) explicitly, or check the symbol (e.g. BTC-IDR)")
			os.Exit(1)
		}
	}

	if cfg.StartPrice <= 0 {
		fmt.Fprintln(os.Stderr, "error: -price is required and must be > 0 (or pass -symbol to auto-fetch)")
		flag.Usage()
		os.Exit(2)
	}
	if cfg.Ticker.High <= 0 || cfg.Ticker.Low <= 0 {
		fmt.Fprintln(os.Stderr, "error: -high and -low are required (or pass -symbol to auto-fetch them)")
		flag.Usage()
		os.Exit(2)
	}

	factory, params, err := buildFactory(*strategy, cfg, *levels, *alloc, *levelSpread, *tolerance, *notional, *monkeySpread)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	res, err := bench.Run(ctx, cfg, factory)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	report(res, *strategy, params, *perSeed)
}

// fetchFromPintu fills in symbol details (base/quote, tick sizes) and the 24h
// OHLC from pintupro's public REST API for cfg.Symbol. Fields the user set
// explicitly (tracked in set) are left untouched. The walk's start price
// defaults to the last close when -price was not given.
func fetchFromPintu(ctx context.Context, cfg *bench.Config, set map[string]bool, apiURL string) error {
	if apiURL == "" {
		apiURL = os.Getenv("PINTUPRO_API_BASE_URL")
	}
	if apiURL == "" {
		apiURL = "https://api.pintu.pro"
	}

	// Public endpoints need no auth; pass any configured keys anyway.
	api := pintupro.NewAPI(os.Getenv("PINTUPRO_KEY"), os.Getenv("PINTUPRO_SECRET"), apiURL)

	symbols, err := api.GetSymbols(ctx)
	if err != nil {
		return fmt.Errorf("get symbols: %w", err)
	}
	si, ok := symbols[cfg.Symbol]
	if !ok {
		return fmt.Errorf("symbol %q not listed by the exchange", cfg.Symbol)
	}
	if !set["base"] {
		cfg.Base = si.Base
	}
	if !set["quote"] {
		cfg.Quote = si.Quote
	}
	if !set["price-tick"] && si.PriceTickSize.IsPositive() {
		cfg.PriceTick = si.PriceTickSize.InexactFloat64()
	}
	if !set["qty-tick"] && si.QuantityTickSize.IsPositive() {
		cfg.QuantityTick = si.QuantityTickSize.InexactFloat64()
	}
	if !set["min-qty"] && si.MinQuantity.IsPositive() {
		cfg.MinQuantity = si.MinQuantity.InexactFloat64()
	}

	t, err := api.Get24hTicker(ctx, cfg.Symbol)
	if err != nil {
		return fmt.Errorf("get 24h ticker: %w", err)
	}
	if !set["open"] {
		cfg.Ticker.Open = t.Open.InexactFloat64()
	}
	if !set["high"] {
		cfg.Ticker.High = t.High.InexactFloat64()
	}
	if !set["low"] {
		cfg.Ticker.Low = t.Low.InexactFloat64()
	}
	if !set["close"] {
		cfg.Ticker.Close = t.Close.InexactFloat64()
	}
	if !set["price"] {
		cfg.StartPrice = t.Close.InexactFloat64()
	}

	return nil
}

// buildFactory returns the strategy factory and a human-readable description of
// the strategy parameters (echoed in the report for provenance).
func buildFactory(
	strategy string,
	cfg bench.Config,
	levels int,
	alloc, levelSpread, tolerance, notional, monkeySpread float64,
) (bench.StrategyFactory, string, error) {
	switch strategy {
	case "ladder":
		lc := bench.UniformLadderConfig(levels, alloc, levelSpread, tolerance)
		if err := lc.Validate(); err != nil {
			return nil, "", fmt.Errorf("invalid ladder config: %w", err)
		}
		params := fmt.Sprintf("levels=%d alloc=%.3g level-spread=%.4g tolerance=%.4g",
			levels, alloc, levelSpread, tolerance)
		return bench.LadderFactory(lc), params, nil
	case "monkey":
		n := notional
		if n <= 0 {
			qb := cfg.StartQuote
			if qb <= 0 {
				qb = cfg.StartPrice
			}
			n = qb * 0.1
		}
		params := fmt.Sprintf("notional=%.6g spread=%.4g", n, monkeySpread)
		return bench.MonkeyFactory(decimal.NewFromFloat(n), decimal.NewFromFloat(monkeySpread)), params, nil
	default:
		return nil, "", fmt.Errorf("unknown strategy %q (want ladder|monkey)", strategy)
	}
}

func report(res bench.Result, strategy, params string, perSeed bool) {
	c := res.Config
	fmt.Printf("Strategy:   %s  [%s]\n", strategy, params)
	fmt.Printf("Symbol:     %s (%s/%s)  start=%.6g spread=%.4f%%\n",
		c.Symbol, c.Base, c.Quote, c.StartPrice, c.Spread*100)
	fmt.Printf("24h OHLC:   O=%.6g H=%.6g L=%.6g C=%.6g  -> target swing %.2f%%\n",
		c.Ticker.Open, c.Ticker.High, c.Ticker.Low, c.Ticker.Close, res.TargetSwingPct)
	fmt.Printf("Sim:        %d runs x %d ticks  seeds %d..%d  fee=%.3f%%  inventory base=%.6g quote=%.6g\n",
		c.Runs, c.Ticks, c.BaseSeed, c.BaseSeed+int64(c.Runs)-1, c.MakerFee*100, c.StartBase, c.StartQuote)
	fmt.Println()
	fmt.Printf("  mean realised swing : %.2f%%  (target %.2f%%)\n", res.MeanSwingPct, res.TargetSwingPct)
	fmt.Printf("  mean fills/run      : %.1f\n", res.MeanFills)
	fmt.Println()
	fmt.Println("  --- PnL % (portfolio value, base valued at final mid) ---")
	fmt.Printf("  mean   : %+.3f%%\n", res.MeanPnLPct)
	fmt.Printf("  std    : %.3f%%\n", res.StdPnLPct)
	fmt.Printf("  median : %+.3f%%\n", res.MedianPnLPct)
	fmt.Printf("  min    : %+.3f%%\n", res.MinPnLPct)
	fmt.Printf("  max    : %+.3f%%\n", res.MaxPnLPct)
	fmt.Printf("  win    : %.1f%% of seeds profitable\n", res.ProfitableFraction*100)

	if perSeed {
		fmt.Println()
		fmt.Println("  seed        PnL%     swing%   fills")
		for _, r := range res.Runs {
			fmt.Printf("  %-10d %+8.3f %8.2f %7d\n", r.Seed, r.PnLPct, r.SwingPct, r.Fills)
		}
	}
}
