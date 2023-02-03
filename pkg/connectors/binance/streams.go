package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"degen/pkg/connectors"
	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

const Name = "binance"

var requestID uint64

type BinanceReq struct {
	Method string `json:"method"`
	ID     uint64 `json:"id"`
	Params any    `json:"params,omitempty"`
}

func SendWSMsg(
	ctx context.Context,
	ws *connectors.WS,
	method string,
	params interface{},
) error {
	b, err := json.Marshal(BinanceReq{
		Method: method,
		ID:     atomic.AddUint64(&requestID, 1),
		Params: params,
	})
	if err != nil {
		return fmt.Errorf("binance.SendWSMsg: %w", err)
	}

	log.Printf("subscribing: %s\n", b)
	return ws.Write(ctx, b)
}

type Binance struct {
	ws        *connectors.WS
	API       *API
	symbols   []string
	listenKey string

	mux sync.RWMutex
}

func NewBinance(
	ctx context.Context,
	key, secret, apiBaseURL, wsBaseURL string,
) *Binance {
	api := NewAPI(key, secret, apiBaseURL)
	// TODO: proper reconnection handling
	ws := &connectors.WS{}

	// Todo refresh listen key
	lk, err := api.GetListenKey(ctx)
	if err != nil {
		panic(err)
	}

	if err := ws.Connect(ctx, fmt.Sprintf("%s/ws/%s", wsBaseURL, lk)); err != nil {
		panic(err)
	}

	return &Binance{
		ws:        ws,
		API:       api,
		listenKey: lk,
	}
}

func (bts *Binance) SubscribeBookTickers(ctx context.Context, symbols []string) error {
	streams := make([]string, len(symbols))
	for i, s := range symbols {
		streams[i] = strings.ToLower(s) + "@bookTicker"
	}
	err := SendWSMsg(ctx, bts.ws, "SUBSCRIBE", streams)
	if err == nil {
		bts.mux.Lock()
		bts.symbols = symbols
		bts.mux.Unlock()
	}
	return err
}

type dummyEvent struct {
	Event string `json:"e"`
}

type bookTicker struct {
	Event     string          `json:"e"`
	Symbol    string          `json:"s"`
	BidPrice  decimal.Decimal `json:"b"`
	BidSize   decimal.Decimal `json:"B"`
	AskPrice  decimal.Decimal `json:"a"`
	AskSize   decimal.Decimal `json:"A"`
	Timestamp int64           `json:"T"`
}

type orderUpdate struct {
	Event string `json:"e"`
	Order struct {
		Symbol          string `json:"s"`
		ClientOrderID   string `json:"c"`
		ExchangeOrderID int64  `json:"i"`
		Side            string `json:"S"`
		Type            string `json:"o"`
		Status          string `json:"X"`
		FilledSize      string `json:"z"`
		AveragePrice    string `json:"ap"`
		UpdatedAtMS     int64  `json:"T"`
	} `json:"o"`
}

func (bts *Binance) Listen(ctx context.Context, ch chan<- models.ExchangeMessage) {
	rawCh := make(chan []byte, 100)
	go func() {
		for {
			if err := bts.ws.Listen(ctx, rawCh); err != nil {
				log.Printf("binance.Listen returned: %v\n", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				// wait for some time before reconnecting
				log.Println("binance.Listen reconnecting")
			}
		}
	}()

	for {
		select {
		case msg := <-rawCh:
			// log.Printf("got msg: %s\n", string(msg))
			var e dummyEvent
			if err := json.Unmarshal(msg, &e); err != nil {
				log.Printf("failed to unmarshal msg: %v\n%v\n", err, string(msg))
				break
			}

			switch e.Event {
			case "ORDER_TRADE_UPDATE":
				var upd orderUpdate
				if err := json.Unmarshal(msg, &upd); err != nil {
					log.Printf("failed to unmarshal ORDER_TRADE_UPDATE: %v %q", err, string(msg))
					break
				}

				o := upd.Order
				size, err := decimal.NewFromString(o.FilledSize)
				if err != nil {
					log.Printf("failed to parse filled size: %v %q", err, string(msg))
					break
				}
				price, err := decimal.NewFromString(o.AveragePrice)
				if err != nil {
					log.Printf("failed to parse average price: %v %q", err, string(msg))
					break
				}

				ch <- models.ExchangeMessage{
					Exchange:  Name,
					Symbol:    strings.ToLower(o.Symbol),
					Timestamp: time.Now().UTC(),
					MsgType:   models.MsgTypeOrderStatus,
					Payload: models.OrderUpdate{
						ClientOrderID:   o.ClientOrderID,
						ExchangeOrderID: strconv.FormatInt(o.ExchangeOrderID, 10),
						UpdatedAt:       time.Unix(0, o.UpdatedAtMS*1000*1000).UTC(),
						Status:          orderStatusFromExchange(o.Status),
						FilledSize:      size,
						AveragePrice:    price,
					},
				}
			case "bookTicker":
				var ticker bookTicker
				if err := json.Unmarshal(msg, &ticker); err != nil {
					log.Printf("failed to unmarshal bookTicker: %v %q", err, string(msg))
					break
				}

				if ticker.Symbol != "" {
					ts := time.Now().UTC()
					ch <- models.ExchangeMessage{
						Exchange:  Name,
						Symbol:    strings.ToLower(ticker.Symbol),
						Timestamp: ts,
						MsgType:   models.MsgTypeTopAsk,
						Payload: models.PriceLevel{
							Price: ticker.AskPrice,
							Size:  ticker.AskSize,
						},
					}

					ch <- models.ExchangeMessage{
						Exchange:  Name,
						Symbol:    strings.ToLower(ticker.Symbol),
						Timestamp: ts,
						MsgType:   models.MsgTypeTopBid,
						Payload: models.PriceLevel{
							Price: ticker.BidPrice,
							Size:  ticker.BidSize,
						},
					}
				}
			default:
				log.Printf("unknown event type: %q", e.Event)
			}
		case <-ctx.Done():
			return
		}
	}
}
