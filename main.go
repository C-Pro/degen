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
	theSymbol     = "BTC-IDR"
	theAsset      = "IDR"
	orderNotional = decimal.NewFromFloat(150000)
	spread        = decimal.NewFromFloat(0.001)
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if os.Getenv("SYMBOL") != "" {
		theSymbol = os.Getenv("SYMBOL")
	}

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		if err := http.ListenAndServe(":8080", nil); err != http.ErrServerClosed {
			log.Printf("HTTP server stopped with error: %v", err)
		}
	}()

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

	acc := account.NewAccount("pintu", ptu)
	monkey := strategies.NewMonkey(
		ctx,
		acc,
		theSymbol,
		orderNotional,
		spread,
	)

	acc.SetStrategy(monkey.See)
	if err := acc.Start(ctx); err != nil {
		log.Printf("failed to start account: %v\n", err)
		return
	}
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

	go func() {
		var lastChange time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				b := acc.GetBalance(theAsset)
				if b.UpdatedAt.After(lastChange) {
					pnl := b.Total.Sub(initialBalance.Total)
					log.Printf("### Current balance is %v; PNL is %v", b.Total, pnl)
					lastChange = b.UpdatedAt
				}
			}
		}
	}()

	<-ctx.Done()
}
