package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"degen/pkg/connectors"
	"degen/pkg/connectors/binance"
	"degen/pkg/models"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	ws := &connectors.WS{}
	tctx, cancel := context.WithTimeout(ctx, time.Second*5)
	err := ws.Connect(tctx, "wss://stream.binancefuture.com/stream")
	cancel()
	if err != nil {
		log.Printf("failed to connect to steram: %v\n", err)
		return
	}

	ch := make(chan models.ExchangeMessage)
	go func() {
		<-ctx.Done()
		close(ch)
	}()

	bnc := binance.NewBinance(
		ctx,
		os.Getenv("BINANCE_KEY"),
		os.Getenv("BINANCE_SECRET"),
		"https://testnet.binancefuture.com",
		"wss://stream.binancefuture.com/stream",
	)

	go bnc.Listen(ctx, ch)

	if err := bnc.SubscribeBookTickers(ctx, []string{"btcusdt"}); err != nil {
		log.Printf("failed to subscribe: %v\n", err)
		return
	}

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
	}
}
