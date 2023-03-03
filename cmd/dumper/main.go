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

	writers := make(map[string]mw, len(symbols))
	for _, s := range symbols {
		f, err := os.OpenFile(fmt.Sprintf("binance-%s-bbo.csv", s), os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o644)
		if err != nil {
			log.Fatalf("failed to open file: %v", err)
		}

		writers[s] = mw{w: csv.NewWriter(f), m: &sync.Mutex{}}
		defer func(f *os.File, w mw) {
			w.m.Lock()
			w.w.Flush()
			w.m.Unlock()
			f.Close()
		}(f, writers[s])
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

			for _, w := range writers {
				w.m.Lock()
				w.w.Flush()
				w.m.Unlock()
			}
		}
	}()

	for msg := range ch {
		switch msg.MsgType {
		case models.MsgTypeTopAsk:
			ask := msg.Payload.(models.PriceLevel)
			w := writers[msg.Symbol]
			w.m.Lock()
			err := w.w.Write([]string{
				strconv.FormatInt(msg.Timestamp.UnixMilli(), 10),
				ask.Price.String(),
				ask.Size.String(),
				"0",
				"0",
			})
			w.m.Unlock()
			if err != nil {
				log.Printf("failed to write %s ask to csv: %v", msg.Symbol, err)
			}
		case models.MsgTypeTopBid:
			bid := msg.Payload.(models.PriceLevel)
			w := writers[msg.Symbol]
			w.m.Lock()
			err := w.w.Write([]string{
				strconv.FormatInt(msg.Timestamp.UnixMilli(), 10),
				"0",
				"0",
				bid.Price.String(),
				bid.Size.String(),
			})
			w.m.Unlock()
			if err != nil {
				log.Printf("failed to write %s bid to csv: %v", msg.Symbol, err)
			}
		default:
			continue
		}
	}
}
