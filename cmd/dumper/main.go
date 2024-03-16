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

	"degen/pkg/accum"
	"degen/pkg/connectors/binance"
	"degen/pkg/models"
)

type (
	featureFn   func(*accum.Intervals) float64
	featureSpec struct {
		name string
		fn   featureFn
	}
)

var (
	symbols         = []string{"ethusdt", "btcusdt", "dogeusdt", "solusdt", "bnbusdt"}
	metrics         = []string{"min", "max", "first", "last", "avg", "sum", "count"}
	windowIntervals = map[string]time.Duration{
		"1_sec":  time.Second,
		"15_sec": time.Second * 15,
		"1_min":  time.Minute,
		"15_min": time.Minute * 15,
		"1_hour": time.Hour,
	}
	dataFields = []string{"bid_price", "bid_size", "ask_price", "ask_size", "buy_volume", "sell_volume", "buy_price", "sell_price"}
	allFields  []string
)

func initAcc(symbols []string) map[string]*accum.Intervals {
	cnt := map[string]int{
		"1_sec":  15,
		"15_sec": 4,
		"1_min":  15,
		"15_min": 4,
		"1_hour": 1,
	}
	accs := make(map[string]*accum.Intervals)
	for _, s := range symbols {
		for _, n := range dataFields {
			name := fmt.Sprintf("%s-%s", s, n)
			accs[name] = accum.NewIntervals()
			for k, v := range windowIntervals {
				accs[name].AddInterval(k, v, cnt[k])
				for _, m := range metrics {
					allFields = append(allFields, fmt.Sprintf("%s-%s-%s-%s", s, n, m, k))
				}
			}
		}
	}
	return accs
}

func getVector(accs map[string]*accum.Intervals) map[string]float64 {
	vec := make(map[string]float64)
	for k, acc := range accs {
		values := acc.GetValues()
		for i, f := range values {
			for m, v := range f {
				vec[fmt.Sprintf("%s-%s-%s", k, m, i)] = v
			}
		}
	}

	return vec
}

func key(symbol, field, metric, interval string) string {
	return fmt.Sprintf("%s-%s-%s-%s", symbol, field, metric, interval)
}

func getHeader(keys []string) []string {
	return append([]string{"timestamp"}, allFields...)
}

func getFeatures(windows map[string]*accum.Accumulator, keys []string) []string {
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

	// Initialize accumulators for the feature vector.
	keys := []string{}
	windows := make(map[string]accum.Accumulator)
	for _, s := range symbols {
		for n := range windowIntervals {
			for _, f := range dataFields {
				k := key(s, n, f)
				windows[k] = accum.New()
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
