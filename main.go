package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"degen/pkg/account"
	"degen/pkg/connectors/pintupro"
	"degen/pkg/strategies"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/shopspring/decimal"
)

var (
	theSymbol     = "WLD-IDR"
	theAsset      = "IDR"
	orderNotional = decimal.NewFromFloat(150000)

	maxOrderNotional = decimal.NewFromFloat(1000000)
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if os.Getenv("SYMBOL") != "" {
		theSymbol = os.Getenv("SYMBOL")
	}

	if os.Getenv("NOTIONAL") != "" {
		var err error
		orderNotional, err = decimal.NewFromString(os.Getenv("NOTIONAL"))
		if err != nil {
			log.Printf("failed to parse NOTIONAL: %v\n", err)
			return
		}

		if orderNotional.LessThanOrEqual(decimal.Zero) || orderNotional.GreaterThan(maxOrderNotional) {
			log.Printf("NOTIONAL must be greater than 0 and less than %v\n", maxOrderNotional)
			return
		}
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
	bal := acc.GetBalance(theAsset)
	alloc := decimal.NewFromFloat(1.0)
	if bal.Total.GreaterThan(decimal.NewFromFloat(500000)) {
		alloc = decimal.NewFromFloat(500000).Div(bal.Total)
	}

	ladder := strategies.NewLadder(
		ctx,
		acc,
		theSymbol,
		strategies.LadderConfig{
			PortfolioAllocation: alloc,
			LevelsCount:         3,
			LevelsSpread: []decimal.Decimal{
				decimal.NewFromFloat(0.018), // 1.8%
				decimal.NewFromFloat(0.018), // 1.8%
				decimal.NewFromFloat(0.018), // 1.8%
			},
			LevelsSize: []decimal.Decimal{
				decimal.NewFromFloat(0.30), // 30% (150k IDR)
				decimal.NewFromFloat(0.30), // 30% (150k IDR)
				decimal.NewFromFloat(0.40), // 40% (200k IDR)
			},
			LevelsPriceTolerance: []decimal.Decimal{
				decimal.NewFromFloat(0.008), // 0.8%
				decimal.NewFromFloat(0.008), // 0.8%
				decimal.NewFromFloat(0.008), // 0.8%
			},
		},
	)
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
	initialBalance := acc.GetBalance(theAsset)
	log.Printf(
		`Initial balalance:
	Total: %s
	Available: %s
`, initialBalance.Total.String(),
		initialBalance.Available.String(),
	)

	if err := acc.SubscribeBookTickers(ctx, []string{theSymbol}); err != nil {
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
				b := acc.GetBalance(theAsset)
				if b.UpdatedAt.After(lastChange) {
					pos := acc.GetPosition(theSymbol)
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
	if err := acc.CancelAllOrders(shutdownCtx, theSymbol); err != nil {
		log.Printf("failed to cancel all orders on shutdown: %v\n", err)
	}
	acc.Stop()
}
