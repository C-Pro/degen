package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"degen/pkg/connectors/binance"
	"degen/pkg/models"
	"degen/pkg/strategies"

	"github.com/shopspring/decimal"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	ch := make(chan models.ExchangeMessage, 100)
	go func() {
		<-ctx.Done()
		close(ch)
	}()

	bnc := binance.NewBinance(
		ctx,
		os.Getenv("BINANCE_KEY"),
		os.Getenv("BINANCE_SECRET"),
		"https://testnet.binancefuture.com",
		"wss://stream.binancefuture.com",
	)

	go bnc.Listen(ctx, ch)

	if err := bnc.SubscribeBookTickers(ctx, []string{"ethusdt"}); err != nil {
		log.Printf("failed to subscribe: %v\n", err)
		return
	}

	monkey := strategies.NewMonkey(ctx)
	go func() {
		for {
			select {
			case side := <-monkey.Say():
				log.Printf("MONKEY WANNA %s!\n", strings.ToUpper(string(side)))
				order := models.Order{
					CreatedAt: time.Now().UTC(),
					Symbol:    "ethusdt",
					Size:      decimal.NewFromFloat(0.01),
					Side:      side,
					Type:      models.OrderTypeMarket,
				}
				res, err := bnc.API.PlaceOrder(ctx, order)
				if err != nil {
					log.Printf("monkey has failed to place an order: %v", err)
					continue
				}

				log.Printf("monkey has placed an order: %#v", res)
			case <-ctx.Done():
				return
			}
		}
	}()

	format := "Best %s price: %v, size: %v\n"
	side := ""
	for msg := range ch {

		switch msg.MsgType {
		case models.MsgTypeTopAsk:
			side = "ask"
		case models.MsgTypeTopBid:
			side = "bid"
		default:
			continue
		}

		tick := msg.Payload.(models.PriceLevel)
		log.Printf(format, side, tick.Price, tick.Size)
		monkey.See(msg)
	}
}
