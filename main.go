package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"degen/pkg/account"
	"degen/pkg/bench"
	"degen/pkg/connectors/pintupro"
	"degen/pkg/models"
	"degen/pkg/strategies"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Operator-configured (env) strategy parameters. The market-fit parameters
// (level spread, allocation, tolerance) are auto-detected by backtesting the
// last 7 days of history before trading starts; see tuneLadder.
type config struct {
	symbol           string
	makerFee         float64 // taker/maker fee fraction (pintu: 0.0012)
	sellTax          float64 // withholding on sells (pintu PPh: 0.0021)
	orderNotional    float64 // nominal size of a single order, in quote currency
	maxOrderNotional float64 // hard cap on any single order's notional
	maxAllocation    float64 // max total capital to deploy to this asset, in quote
}

func getenvFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err == nil {
			return f
		}
		log.Printf("invalid %s=%q, using default %v", key, v, def)
	}
	return def
}

func loadConfig() config {
	c := config{
		symbol:           os.Getenv("SYMBOL"),
		makerFee:         getenvFloat("MAKER_FEE", 0.0012),
		sellTax:          getenvFloat("SELL_TAX", 0.0021),
		orderNotional:    getenvFloat("ORDER_NOTIONAL", 150000),
		maxOrderNotional: getenvFloat("MAX_ORDER_NOTIONAL", 1000000),
		maxAllocation:    getenvFloat("MAX_NOTIONAL_ALLOCATION", 5000000),
	}
	if v := os.Getenv("SYMBOL"); v != "" {
		c.symbol = v
	}
	// NOTIONAL kept as an alias for ORDER_NOTIONAL for backward compatibility.
	if os.Getenv("ORDER_NOTIONAL") == "" {
		c.orderNotional = getenvFloat("NOTIONAL", c.orderNotional)
	}
	return c
}

// tuneLadder downloads the last 7 days of 15m candles for the symbol and grid-
// searches level spread, allocation and tolerance against them (with the
// configured fees), returning the live ladder config. The detected allocation
// fraction is applied to the deployable budget (maxAllocation) and capped so no
// single order exceeds maxOrderNotional.
func tuneLadder(
	ctx context.Context,
	ptu *pintupro.PintuPro,
	si models.SymbolInfo,
	balance float64,
	c config,
) (strategies.LadderConfig, error) {
	now := time.Now().Unix()
	cs, err := ptu.GetCandlesticks(ctx, c.symbol, "15m", now-7*24*3600, now)
	if err != nil {
		return strategies.LadderConfig{}, fmt.Errorf("fetch candles: %w", err)
	}
	if len(cs) == 0 {
		return strategies.LadderConfig{}, fmt.Errorf("no candle history for %s", c.symbol)
	}
	candles := make([]bench.Candle, len(cs))
	for i, k := range cs {
		candles[i] = bench.Candle{
			Open:  k.Open.InexactFloat64(),
			High:  k.High.InexactFloat64(),
			Low:   k.Low.InexactFloat64(),
			Close: k.Close.InexactFloat64(),
		}
	}
	refPrice := candles[0].Open

	bcfg := bench.Config{
		Symbol: c.symbol, Base: si.Base, Quote: si.Quote,
		Spread:         0.0005, // proxy market half-spread
		MakerFee:       c.makerFee,
		SellTaxRate:    c.sellTax,
		PriceTick:      si.PriceTickSize.InexactFloat64(),
		QuantityTick:   si.QuantityTickSize.InexactFloat64(),
		MinQuantity:    c.orderNotional / refPrice,
		TicksPerCandle: 12,
		Runs:           8,
		BaseSeed:       1,
		// Backtest with the deployable budget as the inventory so order sizing
		// is representative.
		StartQuote: c.maxAllocation,
		StartBase:  c.maxAllocation / refPrice,
	}

	// The strategy/account log on every simulated order/fill; mute during the
	// backtest sweep, then restore for live trading.
	log.SetOutput(io.Discard)
	best, all, err := bench.GridSearch(ctx, candles, bcfg, bench.DefaultTuneGrid())
	log.SetOutput(os.Stderr)
	if err != nil {
		return strategies.LadderConfig{}, err
	}

	log.Printf("Tuned %s on %d candles (7d, fee=%.3f%% tax=%.3f%%). Grid (best first):",
		c.symbol, len(candles), c.makerFee*100, c.sellTax*100)
	for _, r := range all {
		log.Printf("  spread=%.4g tol=%.4g alloc=%.2g levels=%d -> PnL %+.3f%% (fills %.1f/run)",
			r.LevelSpread, r.Tolerance, r.Allocation, r.Levels, r.MeanPnLPct, r.MeanFills)
	}

	// Deploy the detected fraction of the budget; never more than the budget nor
	// more than 100% of the live balance.
	liveAlloc := best.Allocation
	if balance > 0 {
		liveAlloc = best.Allocation * c.maxAllocation / balance
	}
	if liveAlloc > 1 {
		liveAlloc = 1
	}
	// Cap so a single (uniform) level's notional stays under maxOrderNotional.
	if best.Levels > 0 && balance > 0 {
		perOrder := liveAlloc * balance / float64(best.Levels)
		if perOrder > c.maxOrderNotional {
			liveAlloc *= c.maxOrderNotional / perOrder
			log.Printf("capped allocation to keep per-order notional <= %.0f", c.maxOrderNotional)
		}
	}

	log.Printf("Chosen for %s: spread=%.4g tol=%.4g detected-alloc=%.2g -> live-alloc=%.3g (7d backtest PnL %+.3f%%)",
		c.symbol, best.LevelSpread, best.Tolerance, best.Allocation, liveAlloc, best.MeanPnLPct)

	return bench.UniformLadderConfig(best.Levels, liveAlloc, best.LevelSpread, best.Tolerance), nil
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg := loadConfig()
	symbol := cfg.symbol

	// Fail fast with an actionable message if required env is missing — the most
	// common cause is a .env that was sourced (`. .env`) but not exported, so the
	// child process sees empty values (which surface as a cryptic "malformed ws
	// or wss URL" from the websocket dialer).
	var missing []string
	for _, k := range []string{"SYMBOL", "PINTUPRO_API_BASE_URL", "PINTUPRO_WS_URL"} {
		if os.Getenv(k) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		log.Printf("missing required env vars: %v", missing)
		log.Printf("if using a .env file it must be EXPORTED to the process; run:")
		log.Printf("  set -a && . ./.env && set +a && ./degen")
		log.Printf("(plain `. .env` only sets shell variables, not the child environment)")
		return
	}

	ptu, err := pintupro.NewPintuPro(
		ctx,
		os.Getenv("PINTUPRO_KEY"),
		os.Getenv("PINTUPRO_SECRET"),
		os.Getenv("PINTUPRO_API_BASE_URL"),
		os.Getenv("PINTUPRO_WS_URL"),
	)
	if err != nil {
		log.Printf("failed to init connector: %v\n", err)
		return
	}

	if ptu == nil {
		return
	}

	acc, err := account.NewAccount("pintu", ptu)
	if err != nil {
		log.Printf("failed to init account: %v\n", err)
		return
	}

	symbols, err := acc.GetSymbols(ctx)
	if err != nil {
		log.Printf("failed to get symbols: %v\n", err)
		return
	}
	si, ok := symbols[symbol]
	if !ok {
		log.Printf("symbol %s not found on exchange\n", symbol)
		return
	}
	quoteAsset := si.Quote

	balance := acc.GetBalance(quoteAsset)

	// Auto-detect market-fit parameters by backtesting the last 7 days.
	ladderCfg, err := tuneLadder(ctx, ptu, si, balance.Total.InexactFloat64(), cfg)
	if err != nil {
		log.Printf("failed to tune ladder: %v\n", err)
		return
	}

	ladder := strategies.NewLadder(ctx, acc, symbol, ladderCfg)
	// NewLadder returns nil on failure (e.g. symbol not found, cancel-all
	// failed). Guard against dereferencing it below. [M3]
	if ladder == nil {
		log.Printf("failed to init ladder strategy\n")
		return
	}

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		// A bind failure means we would trade with no observability; fail loudly
		// by triggering shutdown instead of running blind. [L11]
		if err := http.ListenAndServe(":8080", nil); err != nil && err != http.ErrServerClosed { // nosemgrep
			log.Printf("metrics HTTP server failed: %v", err)
			cancel()
		}
	}()

	if err := acc.Start(ctx); err != nil {
		log.Printf("failed to start account: %v\n", err)
		return
	}

	dispatchDone := make(chan struct{})
	go func() {
		defer close(dispatchDone)
		for {
			// Stop promptly once the context is cancelled so the strategy does
			// not place new orders from buffered updates after the shutdown
			// cancel-all (which would re-orphan live orders). [H9]
			select {
			case <-ctx.Done():
				return
			default:
			}

			select {
			case <-ctx.Done():
				return
			case msg, ok := <-acc.Updates():
				if !ok {
					return
				}
				// Recover per message so a single strategy panic skips that
				// message but the consumer keeps running, instead of dying for
				// the rest of the process lifetime. [C4/M3]
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("recovered from panic in strategy: %v\n", r)
						}
					}()
					ladder.See(msg)
				}()
			}
		}
	}()
	initialBalance := acc.GetBalance(quoteAsset)
	log.Printf(
		`Initial balalance:
	Total: %s
	Available: %s
`, initialBalance.Total.String(),
		initialBalance.Available.String(),
	)

	if err := acc.SubscribeBookTickers(ctx, []string{symbol}); err != nil {
		log.Printf("failed to subscribe tiker: %v\n", err)
		return
	}

	if err := acc.SubscribeUserBalance(ctx); err != nil {
		log.Printf("failed to subscribe balance: %v\n", err)
		return
	}

	if err := acc.SubscribeUserOrders(ctx); err != nil {
		log.Printf("failed to subscribe orders: %v\n", err)
		return
	}

	if err := acc.SubscribeUserTrades(ctx); err != nil {
		log.Printf("failed to subscribe trades: %v\n", err)
		return
	}

	go func() {
		var lastChange time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Minute):
				b := acc.GetBalance(quoteAsset)
				if b.UpdatedAt.After(lastChange) {
					pos := acc.GetPosition(symbol)
					log.Printf("### Current notinal balance is %v; PnL is %v\n",
						b.Total,
						pos.RealizedPnL,
					)
					lastChange = b.UpdatedAt
				}
			}
		}
	}()

	<-ctx.Done()

	// Graceful shutdown. First wait for the strategy dispatch goroutine to stop
	// so no new orders can be placed, THEN cancel resting orders, THEN stop the
	// account. Otherwise buffered updates could re-place orders after the
	// cancel-all and leave them unmanaged on the exchange. [H9]
	<-dispatchDone

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := acc.CancelAllOrders(shutdownCtx, symbol); err != nil {
		log.Printf("failed to cancel all orders on shutdown: %v\n", err)
	}
	acc.Stop()
}
