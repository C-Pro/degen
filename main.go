package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"degen/pkg/account"
	"degen/pkg/connectors/pintupro"
	"degen/pkg/models"
	"degen/pkg/strategies"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/shopspring/decimal"
)

var (
	theSymbol     = "BTC-IDR"
	theAsset      = "IDR"
	orderNotional = decimal.NewFromFloat(150000)
	spread        = decimal.NewFromFloat(0.0005)
)

func main() {
	initialBalance := decimal.Zero
	once := sync.Once{}
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

	ch := make(chan models.ExchangeMessage, 100)
	go func() {
		<-ctx.Done()
		close(ch)
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

	go ptu.Listen(ctx, ch)

	if err := ptu.SubscribeBookTickers(ctx, []string{theSymbol}); err != nil {
		log.Printf("failed to subscribe tiker: %v\n", err)
		return
	}

	if err := ptu.SubscribeUserBalance(ctx); err != nil {
		log.Printf("failed to subscribe balance: %v\n", err)
		return
	}

	if err := ptu.SubscribeUserOrders(ctx); err != nil {
		log.Printf("failed to subscribe orders: %v\n", err)
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

	go func() {
		var lastChange time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				b := acc.GetBalance(theAsset)
				if b.UpdatedAt.After(lastChange) {
					pnl := b.Total.Sub(initialBalance)
					log.Printf("### Current balance is %v; PNL is %v", b.Total, pnl)
					lastChange = b.UpdatedAt
				}
			}
		}
	}()

	for msg := range ch {
		acc.Update(msg)
		switch msg.MsgType {
		case models.MsgTypeBBO:
			bbo := msg.Payload.(models.BBO)
			log.Printf("BBO %s:%s", bbo.Bid.Price.String(), bbo.Ask.Price.String())
			monkey.See(msg)
		case models.MsgTypeOrderStatus:
			upd := msg.Payload.(models.Order)
			log.Printf("%s: %s (%v at %v)\n", upd.ExchangeOrderID, upd.Status, upd.FilledSize, upd.AveragePrice)
			continue
		case models.MsgTypeBalanceUpdate:
			upd := msg.Payload.(models.BalanceUpdate)
			log.Printf("Balance %s = %v\n", upd.Asset, upd.Balance)
			once.Do(func() {
				initialBalance = upd.Balance
			})
			continue
		default:
			continue
		}
	}
}
