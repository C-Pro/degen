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
	theSymbol = "ETH-IDR"
	theAsset  = "IDR"
	orderSize = decimal.NewFromFloat(0.01)
	spread    = decimal.NewFromFloat(0.01)
)

func main() {
	initialBalance := decimal.Zero
	once := sync.Once{}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		http.ListenAndServe(":8080", nil)
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
		log.Printf("failed to subscribe: %v\n", err)
		return
	}

	acc := account.NewAccount("pintu", ptu)

	monkey := strategies.NewMonkey(
		ctx,
		acc,
		theSymbol,
		orderSize,
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
			acc.UpdateBalance(upd.Asset, upd.Balance, decimal.Zero, msg.Timestamp)
			once.Do(func() {
				initialBalance = upd.Balance
			})
			continue
		case models.MsgTypePositionUpdate:
			upd := msg.Payload.(models.PositionUpdate)
			// log.Printf("Position %s = %v\n", upd.Symbol, upd.Amount)
			acc.UpdatePosition(upd.Symbol, upd.Amount, upd.EntryPrice, msg.Timestamp)
			continue
		default:
			continue
		}
	}
}
