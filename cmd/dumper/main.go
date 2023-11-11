package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/c-pro/rolling"

	"degen/pkg/connectors/binance"
	"degen/pkg/models"
)

type (
	featureFn   func(*rolling.Window) float64
	featureSpec struct {
		name string
		fn   featureFn
	}
)

var (
	symbols         = []string{"ethusdt", "btcusdt", "dogeusdt", "solusdt", "bnbusdt"}
	windowIntervals = map[string]time.Duration{
		"1_sec":  time.Second,
		"5_sec":  5 * time.Second,
		"30_sec": 30 * time.Second,
		"1_min":  time.Minute,
	}
	dataFields = []string{"bid_price", "bid_size", "ask_price", "ask_size", "buy_volume", "sell_volume", "buy_price", "sell_price"}
	features   = []featureSpec{
		{"min", func(w *rolling.Window) float64 { return w.Min() }},
		{"max", func(w *rolling.Window) float64 { return w.Max() }},
		{"first", func(w *rolling.Window) float64 { return w.First() }},
		{"last", func(w *rolling.Window) float64 { return w.Last() }},
		{"mid", func(w *rolling.Window) float64 { return w.Mid() }},
		{"avg", func(w *rolling.Window) float64 { return w.Avg() }},
		{"sum", func(w *rolling.Window) float64 { return w.Sum() }},
		{"count", func(w *rolling.Window) float64 { return float64(w.Count()) }},
	}
)

func key(symbol, interval, field string) string {
	return fmt.Sprintf("%s-%s-%s", symbol, interval, field)
}

func getHeader(keys []string) []string {
	row := []string{"timestamp"}
	for _, k := range keys {
		for _, spec := range features {
			row = append(row, fmt.Sprintf("%s-%s", k, spec.name))
		}
	}

	return row
}

func getFeatures(windows map[string]*rolling.Window, keys []string) []string {
	row := []string{
		strconv.FormatInt(time.Now().UnixMilli(), 10),
	}
	for _, k := range keys {
		window := windows[k]

		for _, spec := range features {
			row = append(row, strconv.FormatFloat(spec.fn(window), 'f', -1, 64))
		}
	}

	return row
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Initialize rolling windows for the feature vector.
	keys := []string{}
	windows := make(map[string]*rolling.Window)
	for _, s := range symbols {
		for n, d := range windowIntervals {
			for _, f := range dataFields {
				k := key(s, n, f)
				windows[k] = rolling.NewWindow(10000, d)
				keys = append(keys, k)
			}
		}
	}
	sort.Strings(keys)

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

	f, err := os.OpenFile("binance.csv", os.O_RDWR|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		log.Fatalf("failed to open file: %v", err)
	}

	w := csv.NewWriter(f)
	mux := &sync.RWMutex{}

	// If file is empty, write header.
	if stat, err := f.Stat(); err == nil && stat.Size() == 0 {
		if err := w.Write(getHeader(keys)); err != nil {
			log.Fatalf("failed to write header to csv: %v", err)
		}
	}

	defer func() {
		w.Flush()
		f.Close()
	}()

	go func() {
		i := uint64(0)
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}

			mux.RLock()
			row := getFeatures(windows, keys)
			log.Printf("%s: BTC: %06.02f",
				time.Now().Format(time.RFC3339),
				(windows[key("btcusdt", "1_sec", "bid_price")].Avg()+
					windows[key("btcusdt", "1_sec", "ask_price")].Avg())/2,
			)
			mux.RUnlock()

			err := w.Write(row)
			if err != nil {
				log.Printf("failed to write row to csv: %v", err)
			}
			w.Flush()
			i++
		}
	}()

	for msg := range ch {
		msg.Symbol = strings.ToLower(msg.Symbol)
		switch msg.MsgType {
		case models.MsgTypeBBO:
			bbo := msg.Payload.(models.BBO)
			for n := range windowIntervals {
				mux.Lock()
				windows[key(msg.Symbol, n, "ask_price")].Add(bbo.Ask.Price.InexactFloat64())
				windows[key(msg.Symbol, n, "bid_price")].Add(bbo.Bid.Price.InexactFloat64())
				windows[key(msg.Symbol, n, "ask_size")].Add(bbo.Ask.Size.InexactFloat64())
				windows[key(msg.Symbol, n, "bid_size")].Add(bbo.Bid.Size.InexactFloat64())
				mux.Unlock()
			}
		case models.MsgTypeTrade:
			trade := msg.Payload.(models.Trade)
			for n := range windowIntervals {
				mux.Lock()
				if trade.Side == models.OrderSideBuy {
					windows[key(msg.Symbol, n, "buy_volume")].Add(trade.Size.InexactFloat64())
					windows[key(msg.Symbol, n, "buy_price")].Add(trade.Price.InexactFloat64())
				} else {
					windows[key(msg.Symbol, n, "sell_volume")].Add(trade.Size.InexactFloat64())
					windows[key(msg.Symbol, n, "sell_price")].Add(trade.Price.InexactFloat64())
				}
				mux.Unlock()
			}
		default:
			continue
		}
	}
}
