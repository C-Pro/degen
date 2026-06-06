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
	spread        = decimal.NewFromFloat(0.001)

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

	if os.Getenv("SPREAD") != "" {
		var err error
		spread, err = decimal.NewFromString(os.Getenv("SPREAD"))
		if err != nil {
			log.Printf("failed to parse SPREAD: %v\n", err)
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
				decimal.NewFromFloat(0.30),  // 30% (150k IDR)
				decimal.NewFromFloat(0.30),  // 30% (150k IDR)
				decimal.NewFromFloat(0.40),  // 40% (200k IDR)
			},
			LevelsPriceTolerance: []decimal.Decimal{
				decimal.NewFromFloat(0.008), // 0.8%
				decimal.NewFromFloat(0.008), // 0.8%
				decimal.NewFromFloat(0.008), // 0.8%
			},
		},
	)

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		if err := http.ListenAndServe(":8080", nil); err != http.ErrServerClosed {
			log.Printf("HTTP server stopped with error: %v", err)
		}
	}()

	if err := acc.Start(ctx); err != nil {
		log.Printf("failed to start account: %v\n", err)
		return
	}

	go func() {
		for msg := range acc.Updates() {
			ladder.See(msg)
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
}
