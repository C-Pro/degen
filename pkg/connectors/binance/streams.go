package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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

type BookTickerStream struct {
	ws      *connectors.WS
	symbols []string
	mux     sync.RWMutex
}

func NewBookTickerStream(ws *connectors.WS) *BookTickerStream {
	return &BookTickerStream{
		ws: ws,
	}
}

func (bts *BookTickerStream) Subscribe(ctx context.Context, symbols []string) error {
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

type streamMessage struct {
	Stream string          `json:"stream"`
	Data   json.RawMessage `json:"data"`
}

/*{
  "u":400900217,     // order book updateId
  "s":"BNBUSDT",     // symbol
  "b":"25.35190000", // best bid price
  "B":"31.21000000", // best bid qty
  "a":"25.36520000", // best ask price
  "A":"40.66000000"  // best ask qty
}*/

type bookTicker struct {
	Symbol   string          `json:"s"`
	BidPrice decimal.Decimal `json:"b"`
	BidSize  decimal.Decimal `json:"B"`
	AskPrice decimal.Decimal `json:"a"`
	AskSize  decimal.Decimal `json:"A"`
}

func (bts *BookTickerStream) Listen(ctx context.Context, ch chan<- models.ExchangeMessage) {
	rawCh := make(chan []byte, 100)
	go func() {
		for {
			if err := bts.ws.Listen(ctx, rawCh); err != nil {
				log.Printf("ws.Listen returned: %v\n", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				// wait for some time before reconnecting
			}
		}
	}()

	for {
		select {
		case msg := <-rawCh:
			// log.Printf("got msg: %s\n", string(msg))
			var envelope streamMessage
			if err := json.Unmarshal(msg, &envelope); err != nil {
				break
			}

			parts := strings.Split(envelope.Stream, "@")
			if len(parts) < 2 {
				break
			}
			// symbol := strings.ToLower(parts[0])
			streamType := parts[1]

			switch streamType {
			case "bookTicker":
				var ticker bookTicker
				if err := json.Unmarshal(envelope.Data, &ticker); err == nil && ticker.Symbol != "" {
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
				log.Printf("unknown stream type: %q", streamType)
			}
		case <-ctx.Done():
			return
		}
	}
}
