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

var requestID uint64

type BinanceReq struct {
	Method string `json:"method"`
	ID     uint64 `json:"id"`
	Params any    `json:"params,omitempty"`
}

func (bts *Binance) refreshListenKey(ctx context.Context) error {
	lk, err := bts.API.GetListenKey(ctx)
	if err != nil {
		return fmt.Errorf("binance.refreshListenKey: %v", err)
	}

	bts.mux.Lock()
	bts.listenKey = lk
	bts.mux.Unlock()

	return nil
}

// refreshListenKeyLoop refreshes listen key every 15 minutes
func (bts *Binance) refreshListenKeyLoop(ctx context.Context, once *sync.Once, ready chan any) {
	ticker := time.NewTicker(time.Minute * 15)
	defer ticker.Stop()
	for {
		if err := bts.refreshListenKey(ctx); err != nil {
			log.Println(err)
			select {
			case <-time.After(time.Second):
				continue
			case <-ctx.Done():
				return
			}
		}

		once.Do(func() { close(ready) })

		select {
		case <-ticker.C:
			continue
		case <-ctx.Done():
			return
		}
	}
}

func (bts *Binance) getListenKey() string {
	bts.mux.RLock()
	defer bts.mux.RUnlock()

	return bts.listenKey
}

// wsReconnectLoop connects to websocket and reconnects on error or
// when something is received via reconnectCh channel
func (bts *Binance) wsReconnectLoop(
	ctx context.Context,
	wsBaseURL string,
	once *sync.Once,
	ready chan any,
) {
	for {
		endpoint := fmt.Sprintf("%s/ws", wsBaseURL)
		lk := bts.getListenKey()
		if lk != "" {
			endpoint = fmt.Sprintf("%s/%s", endpoint, lk)
		}
		if err := bts.ws.Connect(
			ctx,
			endpoint,
		); err != nil {
			log.Printf("binance websocket connect error: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				continue
			}
		}

		var toSubscribe []string
		bts.mux.RLock()
		if len(bts.subscribedStreams) > 0 {
			toSubscribe = bts.subscribedStreams
		}
		bts.mux.RUnlock()

		if len(toSubscribe) > 0 {
			log.Printf("subscribing: %q", strings.Join(toSubscribe, ","))
			if err := bts.subscribeStreams(ctx, toSubscribe); err != nil {
				log.Printf("binance websocket subscribe error: %v", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
					continue
				}
			}
		}

		once.Do(func() { close(ready) })

		select {
		case <-ctx.Done():
			return
		case <-bts.reconnectCh:
			continue
		}
	}
}

// SendWSMsg sends a websocket command with unique ID
func SendWSMsg(
	ctx context.Context,
	ws *connectors.WS,
	method string,
	params interface{},
) (uint64, error) {
	id := atomic.AddUint64(&requestID, 1)
	b, err := json.Marshal(BinanceReq{
		Method: method,
		ID:     id,
		Params: params,
	})
	if err != nil {
		return 0, fmt.Errorf("binance.SendWSMsg: %w", err)
	}

	return id, ws.Write(ctx, b)
}

func (bts *Binance) SubscribeBookTickers(ctx context.Context, symbols []string) error {
	streams := make([]string, len(symbols))
	for i, s := range symbols {
		streams[i] = strings.ToLower(s) + "@bookTicker"
	}

	return bts.subscribeStreams(ctx, streams)
}

func (bts *Binance) SubscribeBookAggTrades(ctx context.Context, symbols []string) error {
	streams := make([]string, len(symbols))
	for i, s := range symbols {
		streams[i] = strings.ToLower(s) + "@aggTrade"
	}

	return bts.subscribeStreams(ctx, streams)
}

func (bts *Binance) subscribeStreams(ctx context.Context, streams []string) error {
	id, err := SendWSMsg(ctx, bts.ws, "SUBSCRIBE", streams)
	if err == nil {
		bts.mux.Lock()
		bts.subscriptionRequests[id] = streams
		bts.mux.Unlock()
	}

	return err
}

//easyjson:json
type dummyEvent struct {
	Event string `json:"e"`
}

//easyjson:json
type subscribeResponse struct {
	ID     uint64  `json:"id"`
	Result *string `json:"result"`
}

//easyjson:json
type bookTicker struct {
	Event     string          `json:"e"`
	Symbol    string          `json:"s"`
	BidPrice  decimal.Decimal `json:"b"`
	BidSize   decimal.Decimal `json:"B"`
	AskPrice  decimal.Decimal `json:"a"`
	AskSize   decimal.Decimal `json:"A"`
	Timestamp int64           `json:"T"`
}

//easyjson:json
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

//easyjson:json
type accountUpdate struct {
	Event     string `json:"e"`
	Timestamp int64  `json:"E"`
	Update    struct {
		Reason   string `json:"m"`
		Balances []struct {
			Asset   string          `json:"a"`
			Balance decimal.Decimal `json:"wb"`
		} `json:"B"`
		Positions []struct {
			Symbol     string          `json:"s"`
			Amount     decimal.Decimal `json:"pa"`
			EntryPrice decimal.Decimal `json:"ep"`
		} `json:"P"`
	} `json:"a"`
}

//easyjson:json
type aggTrade struct {
	Event     string          `json:"e"`
	Timestamp int64           `json:"E"`
	Symbol    string          `json:"s"`
	TradeID   int64           `json:"a"`
	Price     decimal.Decimal `json:"p"`
	Quantity  decimal.Decimal `json:"q"`
	FirstID   int64           `json:"f"`
	LastID    int64           `json:"l"`
	TradeTime int64           `json:"T"`
	IsBuyer   bool            `json:"m"`
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
				bts.reconnectCh <- "reconnect, please"
			}
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ts := atomic.LoadInt64(&bts.lastReceived)
			if time.Since(time.Unix(0, ts)) > bts.idleTimeout {
				log.Printf("no messages for %s, reconnecting", bts.idleTimeout)
				bts.reconnectCh <- "reconnect, please"
				return
			}
		case msg := <-rawCh:
			var r subscribeResponse
			if err := json.Unmarshal(msg, &r); err != nil {
				log.Printf("failed to unmarshal msg: %v\n%v\n", err, string(msg))
				break
			}

			if r.ID > 0 {
				if r.Result != nil {
					log.Printf("message id=%d returned result %q", r.ID, *r.Result)
					break
				}

				bts.mux.RLock()
				streams, ok := bts.subscriptionRequests[r.ID]
				if !ok {
					log.Printf("unsolicted message id=%d returned result %q", r.ID, *r.Result)
					bts.mux.RUnlock()
					break
				}
				bts.mux.RUnlock()

				bts.mux.Lock()
				bts.subscribedStreams = append(bts.subscribedStreams, streams...)
				bts.mux.Unlock()
				break
			}

			var e dummyEvent
			if err := json.Unmarshal(msg, &e); err != nil {
				log.Printf("failed to unmarshal msg: %v\n%v\n", err, string(msg))
				break
			}

			atomic.StoreInt64(&bts.lastReceived, time.Now().UnixNano())

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
					Symbol:    symbolFromExchange(o.Symbol),
					Timestamp: time.Now().UTC(),
					MsgType:   models.MsgTypeOrderStatus,
					Payload: models.OrderUpdate{
						ClientOrderID:   o.ClientOrderID,
						ExchangeOrderID: strconv.FormatInt(o.ExchangeOrderID, 10),
						UpdatedAt:       timestampToTime(o.UpdatedAtMS),
						Status:          orderStatusFromExchange(o.Status),
						Side:            models.OrderSide(strings.ToLower(o.Side)),
						Symbol:          symbolFromExchange(o.Symbol),
						FilledSize:      size,
						AveragePrice:    price,
					},
				}
			case "ACCOUNT_UPDATE":
				var upd accountUpdate
				if err := json.Unmarshal(msg, &upd); err != nil {
					log.Printf("failed to unmarshal account update: %v %q", err, string(msg))
					break
				}

				for _, b := range upd.Update.Balances {
					ch <- models.ExchangeMessage{
						Exchange:  Name,
						Timestamp: timestampToTime(upd.Timestamp),
						MsgType:   models.MsgTypeBalanceUpdate,
						Payload: models.BalanceUpdate{
							Asset:   strings.ToLower(b.Asset),
							Balance: b.Balance,
						},
					}
				}

				for _, p := range upd.Update.Positions {
					ch <- models.ExchangeMessage{
						Exchange:  Name,
						Timestamp: timestampToTime(upd.Timestamp),
						MsgType:   models.MsgTypePositionUpdate,
						Payload: models.PositionUpdate{
							Symbol:     strings.ToLower(p.Symbol),
							Amount:     p.Amount,
							EntryPrice: p.EntryPrice,
						},
					}
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
						Symbol:    symbolFromExchange(ticker.Symbol),
						Timestamp: ts,
						MsgType:   models.MsgTypeBBO,
						Payload: models.BBO{
							Bid: models.PriceLevel{
								Price: ticker.BidPrice,
								Size:  ticker.BidSize,
							},
							Ask: models.PriceLevel{
								Price: ticker.AskPrice,
								Size:  ticker.AskSize,
							},
							Timestamp: timestampToTime(ticker.Timestamp),
						},
					}
				}
			case "aggTrade":
				var trade aggTrade
				if err := json.Unmarshal(msg, &trade); err != nil {
					log.Printf("failed to unmarshal aggTrade: %v %q", err, string(msg))
					break
				}

				if trade.Symbol != "" {
					side := models.OrderSideSell
					if trade.IsBuyer {
						side = models.OrderSideBuy
					}
					ch <- models.ExchangeMessage{
						Exchange:  Name,
						Symbol:    symbolFromExchange(trade.Symbol),
						Timestamp: time.Now().UTC(),
						MsgType:   models.MsgTypeTrade,
						Payload: models.Trade{
							Price:     trade.Price,
							Size:      trade.Quantity,
							Timestamp: timestampToTime(trade.Timestamp),
							Side:      side,
						},
					}
				}
			default:
				log.Printf("unknown event type: %q\n%q", e.Event, string(msg))
			}
		case <-ctx.Done():
			return
		}
	}
}
