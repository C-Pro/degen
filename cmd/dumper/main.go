package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"degen/pkg/connectors/binance"
	"degen/pkg/models"
)

var symbols = []string{"ethusdt", "btcusdt", "dogeusdt", "solusdt"}

type mw struct {
	w *csv.Writer
	m *sync.Mutex
}

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
		"https://fapi.binance.com",
		"wss://fstream.binance.com",
	)

	if bnc == nil {
		return
	}

	go bnc.Listen(ctx, ch)

	if err := bnc.SubscribeBookTickers(ctx, symbols); err != nil {
		log.Printf("failed to subscribe: %v\n", err)
		return
	}
	if err := bnc.SubscribeBookAggTrades(ctx, symbols); err != nil {
		log.Printf("failed to subscribe: %v\n", err)
		return
	}

	bboWriters := make(map[string]mw, len(symbols))
	tradeWriters := make(map[string]mw, len(symbols))
	for _, s := range symbols {
		f, err := os.OpenFile(fmt.Sprintf("binance-%s-bbo.csv", s), os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o644)
		if err != nil {
			log.Fatalf("failed to open file: %v", err)
		}

		bboWriters[s] = mw{w: csv.NewWriter(f), m: &sync.Mutex{}}
		defer func(f *os.File, w mw) {
			w.m.Lock()
			w.w.Flush()
			w.m.Unlock()
			f.Close()
		}(f, bboWriters[s])

		f, err = os.OpenFile(fmt.Sprintf("binance-%s-trades.csv", s), os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o644)
		if err != nil {
			log.Fatalf("failed to open file: %v", err)
		}

		tradeWriters[s] = mw{w: csv.NewWriter(f), m: &sync.Mutex{}}
		defer func(f *os.File, w mw) {
			w.m.Lock()
			w.w.Flush()
			w.m.Unlock()
			f.Close()
		}(f, tradeWriters[s])
	}

	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}

			for _, w := range bboWriters {
				w.m.Lock()
				w.w.Flush()
				w.m.Unlock()
			}
			for _, w := range tradeWriters {
				w.m.Lock()
				w.w.Flush()
				w.m.Unlock()
			}
		}
	}()

	for msg := range ch {
		switch msg.MsgType {
		case models.MsgTypeBBO:
			bbo := msg.Payload.(models.BBO)
			w := bboWriters[msg.Symbol]
			w.m.Lock()
			err := w.w.Write([]string{
				strconv.FormatInt(msg.Timestamp.UnixMilli(), 10),
				bbo.Bid.Price.String(),
				bbo.Bid.Size.String(),
				bbo.Ask.Price.String(),
				bbo.Ask.Size.String(),
			})
			w.m.Unlock()
			if err != nil {
				log.Printf("failed to write %s ask to csv: %v", msg.Symbol, err)
			}
		case models.MsgTypeTrade:
			trade := msg.Payload.(models.Trade)
			w := tradeWriters[msg.Symbol]
			w.m.Lock()
			err := w.w.Write([]string{
				strconv.FormatInt(msg.Timestamp.UnixMilli(), 10),
				string(trade.Side),
				trade.Price.String(),
				trade.Size.String(),
			})
			w.m.Unlock()
			if err != nil {
				log.Printf("failed to write %s trade to csv: %v", msg.Symbol, err)
			}
		default:
			continue
		}
	}
}
