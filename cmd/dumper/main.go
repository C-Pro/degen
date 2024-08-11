package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"degen/pkg/accum"
	"degen/pkg/connectors/binance"
	"degen/pkg/csvwriter"
	"degen/pkg/models"
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
)

func initAccs(symbols []string) (map[string]*accum.Intervals, []string) {
	allFields := make([]string, 0)
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
			name := key(s, n)
			accs[name] = accum.NewIntervals()
			for k, v := range windowIntervals {
				accs[name].AddInterval(k, v, cnt[k])
				for _, m := range metrics {
					allFields = append(allFields, key(name, m, k))
				}
			}
		}
	}
	sort.Strings(allFields)

	return accs, allFields
}

// getVector returns the feature vector from the accumulators.
func getVector(accs map[string]*accum.Intervals, allFields []string) []float64 {
	values := make(map[string]float64, len(allFields))
	vec := make([]float64, 0, len(allFields)+1)
	// First field of the feature vector is the current timestamp.
	vec = append(vec, float64(time.Now().UnixMilli()))

	for _, s := range symbols {
		for _, n := range dataFields {
			name := key(s, n)
			vals := accs[name].GetValues()
			for i, f := range vals {
				for m, v := range f {
					values[key(name, m, i)] = v
				}
			}
		}
	}

	for _, f := range allFields {
		vec = append(vec, values[f])
	}

	return vec
}

// key builds the key for the the accumulator or value.
// the order of fields is: symbol, field, metric, interval.
func key(v ...string) string {
	return strings.Join(v, "-")
}

// vecToString converts the feature vector to a string slice.
func vecToString(vec []float64) []string {
	row := make([]string, 0, len(vec))
	for _, f := range vec {
		row = append(row, strconv.FormatFloat(f, 'f', -1, 64))
	}

	return row
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Initialize accumulators for the feature vector.
	accs, allFields := initAccs(symbols)

	ch := make(chan models.ExchangeMessage, 100)
	go func() {
		<-ctx.Done()
		close(ch)
	}()

	bnc := binance.NewBinance(
		ctx,
		os.Getenv("BINANCE_KEY"),
		os.Getenv("BINANCE_SECRET"),
		"https://api.binance.com",
		"wss://stream.binance.com",
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

	w, err := csvwriter.NewCSVWriter(ctx, ".", "binance", allFields, csvwriter.IntervalDaily)
	if err != nil {
		log.Fatalf("failed to create csv writer: %v", err)
	}

	mux := &sync.RWMutex{}

	btcBidIdx := slices.Index(allFields, "btcusdt-bid_price-avg-1_sec")
	btcAskIdx := slices.Index(allFields, "btcusdt-ask_price-avg-1_sec")

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
			row := getVector(accs, allFields)
			log.Printf("%s: BTC: %06.02f",
				time.Now().Format(time.RFC3339),
				(row[btcBidIdx]+row[btcAskIdx])/2,
			)
			mux.RUnlock()

			w.WriteRow(vecToString(row))
			i++
		}
	}()

	for msg := range ch {
		msg.Symbol = strings.ToLower(msg.Symbol)
		switch msg.MsgType {
		case models.MsgTypeBBO:
			bbo := msg.Payload.(models.BBO)
			mux.Lock()
			accs[key(msg.Symbol, "bid_price")].Observe(time.Now(), bbo.Bid.Price.InexactFloat64())
			accs[key(msg.Symbol, "ask_price")].Observe(time.Now(), bbo.Ask.Price.InexactFloat64())
			accs[key(msg.Symbol, "bid_size")].Observe(time.Now(), bbo.Bid.Size.InexactFloat64())
			accs[key(msg.Symbol, "ask_size")].Observe(time.Now(), bbo.Ask.Size.InexactFloat64())
			mux.Unlock()
		case models.MsgTypePublicTrade:
			trade := msg.Payload.(models.Trade)
			mux.Lock()
			if trade.Side == models.OrderSideBuy {
				accs[key(msg.Symbol, "buy_volume")].Observe(time.Now(), trade.Size.InexactFloat64())
				accs[key(msg.Symbol, "buy_price")].Observe(time.Now(), trade.Price.InexactFloat64())
			} else {
				accs[key(msg.Symbol, "sell_volume")].Observe(time.Now(), trade.Size.InexactFloat64())
				accs[key(msg.Symbol, "sell_price")].Observe(time.Now(), trade.Price.InexactFloat64())
			}
			mux.Unlock()
		default:
			continue
		}
	}
}
