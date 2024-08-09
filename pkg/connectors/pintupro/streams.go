package pintupro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"degen/pkg/models"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func (p *PintuPro) wsReconnectLoop(ctx context.Context, wsBaseURL string) {
	var (
		connectedAt time.Time
		once        sync.Once
	)
	for {
		if err := p.ws.Connect(ctx, wsBaseURL); err != nil {
			log.Printf("pintupro websocket connect error: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				continue
			}
		}
		if p.key != "" {
			if err := p.auth(ctx); err != nil {
				log.Printf("pintupro auth write error: %v", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
					continue
				}
			}
		}
		// Connected. Subscribe to streams.
		connectedAt = time.Now()
		once.Do(func() { close(p.wsReady) })

		var toSubscribe []string
		p.mux.RLock()
		if len(p.subscribedStreams) > 0 {
			toSubscribe = slices.Clone(p.subscribedStreams)
			p.subscribedStreams = p.subscribedStreams[:0]
		}
		p.mux.RUnlock()

		if len(toSubscribe) > 0 {
			log.Printf("pintupro: subscribing: %q", strings.Join(toSubscribe, ","))
			if err := p.subscribeStreams(ctx, toSubscribe); err != nil {
				log.Printf("pintupro websocket subscribe error: %v", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
					continue
				}
			}
		}

	ignore:
		select {
		case <-ctx.Done():
			return
		case reason := <-p.reconnectCh:
			// If last connection was established less than 10 seconds ago, ignore reconnect request.
			// To avoid reconnect loop that can lead to IP ban.
			if time.Since(connectedAt) < time.Second*10 {
				goto ignore
			}
			log.Printf("reconnecting: %s", reason)
			p.ws.Close()
			continue
		}
	}
}

func (p *PintuPro) auth(ctx context.Context) error {
	req := WrapAndSign("public/auth", p.key, p.secret, uuid.NewString(), nil, time.Now())
	b, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}
	return p.ws.Write(ctx, b)
}

func (p *PintuPro) subscribeStreams(ctx context.Context, streams []string) error {
	req := Envelope{
		RequestID: uuid.NewString(),
		Method:    "subscribe",
		Params:    map[string]interface{}{"channels": streams},
		Timestamp: time.Now().UnixMilli(),
	}
	b, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}
	return p.ws.Write(ctx, b)
}

func (p *PintuPro) SubscribeBookTickers(ctx context.Context, symbols []string) error {
	if len(symbols) == 0 {
		return nil
	}

	streams := make([]string, len(symbols))
	for i, s := range symbols {
		streams[i] = "aggrbook.snapshot.1." + s
	}

	if err := p.subscribeStreams(ctx, streams); err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	return nil
}

func (p *PintuPro) SubscribeBookAggTrades(ctx context.Context, symbols []string) error {
	if len(symbols) == 0 {
		return nil
	}

	streams := make([]string, len(symbols))
	for i, s := range symbols {
		streams[i] = "trades." + s
	}

	if err := p.subscribeStreams(ctx, streams); err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	return nil
}

func (p *PintuPro) SubscribeUserBalance(ctx context.Context) error {
	if p.key == "" {
		return errors.New("SubscribeUserBalance: connection is not authenticated")
	}

	if err := p.subscribeStreams(ctx, []string{"user.balance.snapshot"}); err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	return nil
}

// easyjson:json
type orderStatusMsg struct {
	Status        string          `json:"status"`
	Symbol        string          `json:"symbol"`
	Reason        string          `json:"reason"`
	Type          string          `json:"type"`
	TimeInForce   string          `json:"time_in_force"`
	ExecInst      string          `json:"exec_inst"`
	Side          string          `json:"side"`
	Price         decimal.Decimal `json:"price"`
	Size          decimal.Decimal `json:"size"`
	Notional      decimal.Decimal `json:"notional"`
	CumPrice      decimal.Decimal `json:"cum_price"`
	CumSize       decimal.Decimal `json:"cum_size"`
	CumValue      decimal.Decimal `json:"cum_value"`
	OrderID       string          `json:"order_id"`
	ClientOrderID string          `json:"client_order_id"`
	CreatedAt     int64           `json:"created_at"`
	UpdatedAt     int64           `json:"updated_at"`
}

// easyjson:json
type userOrdersMsg struct {
	Orders []orderStatusMsg `json:"orders"`
}

//easyjson:json
type wsMessage struct {
	RequestID string          `json:"request_id"`
	Timestamp int64           `json:"timestamp"`
	Method    string          `json:"method"`
	Channel   string          `json:"channel"`
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	Reason    string          `json:"reason"`
	Data      json.RawMessage `json:"data"`
}

func (p *PintuPro) handleUserOrders(msg wsMessage, ch chan<- models.ExchangeMessage) error {
	var orders userOrdersMsg
	if err := json.Unmarshal(msg.Data, &orders); err != nil {
		return fmt.Errorf("failed to unmarshal user orders: %w", err)
	}

	for _, o := range orders.Orders {
		order, err := toOrder(o)
		if err != nil {
			return fmt.Errorf("failed to convert order: %w", err)
		}
		ch <- models.ExchangeMessage{
			Exchange:  Name,
			Symbol:    o.Symbol,
			Timestamp: tsToTime(o.UpdatedAt),
			MsgType:   models.MsgTypeOrderStatus,
			Payload:   order,
		}
	}

	return nil
}

func toOrder(o orderStatusMsg) (models.Order, error) {
	side := models.OrderSideBuy
	if o.Side == "SELL" {
		side = models.OrderSideSell
	}

	var otype models.OrderType
	switch o.Type {
	case "LIMIT":
		otype = models.OrderTypeLimit
	case "MARKET":
		otype = models.OrderTypeMarket
	default:
		return models.Order{}, fmt.Errorf("unknown order type: %s", o.Type)
	}

	var status models.OrderStatus
	switch o.Status {
	case "PLACED":
		status = models.OrderStatusPlaced
	case "PARTIALLY_FILLED":
		status = models.OrderStatusPartiallyFilled
	case "FILLED":
		status = models.OrderStatusFilled
	case "CANCELED":
		status = models.OrderStatusCanceled
	case "REJECTED":
		status = models.OrderStatusRejected
	default:
		return models.Order{}, fmt.Errorf("unknown order status: %s", o.Status)
	}

	var timeInForce models.TimeInForce
	switch o.TimeInForce {
	case "GTC":
		timeInForce = models.TimeInForceGTC
	case "IOC":
		timeInForce = models.TimeInForceIOC
	case "FOK":
		timeInForce = models.TimeInForceFOK
	default:
		return models.Order{}, fmt.Errorf("unknown time in force: %s", o.TimeInForce)
	}

	parts := strings.Split(o.Symbol, "-")
	if len(parts) != 2 {
		return models.Order{}, fmt.Errorf("invalid symbol: %s", o.Symbol)
	}

	final := false
	if otype == models.OrderTypeMarket && timeInForce == models.TimeInForceIOC {
		final = true
	}

	if otype == models.OrderTypeLimit &&
		(status == models.OrderStatusFilled ||
			status == models.OrderStatusCanceled ||
			status == models.OrderStatusRejected) {
		final = true
	}

	return models.Order{
		ExchangeOrderID: o.OrderID,
		ClientOrderID:   o.ClientOrderID,
		Symbol:          o.Symbol,
		Base:            parts[0],
		Quote:           parts[1],
		Side:            side,
		Type:            otype,
		Status:          status,
		Price:           o.Price,
		Size:            o.Size,
		NotionalSize:    o.Notional,
		FilledSize:      o.CumSize,
		AveragePrice:    o.CumPrice,
		TimeInForce:     timeInForce,
		PostOnly:        o.ExecInst == "POST_ONLY",
		Final:           final,

		CreatedAt: tsToTime(o.CreatedAt),
		UpdatedAt: tsToTime(o.UpdatedAt),
	}, nil
}

func (p *PintuPro) SubscribeUserOrders(ctx context.Context) error {
	if p.key == "" {
		return errors.New("SubscribeUserOrders: connection is not authenticated")
	}

	if err := p.subscribeStreams(ctx, []string{"user.orders"}); err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	return nil
}

func (p *PintuPro) registerWSHandlers() {
	p.wsHandlers = map[string]wsHandlerFunc{
		"heartbeat-request":     p.handleHeartbeat,
		"subscribe":             p.handleSubscription,
		"trades.":               p.handlePublicTrades,
		"aggrbook.snapshot.":    p.handleOrderBook,
		"public/auth":           p.handleAuth,
		"user.balance.snapshot": p.handleUserBalance,
		"user.orders":           p.handleUserOrders,
		// "user.orders.snapshot": p.handleUserOrdersSnapshot,
		// "user.trades":        p.handleUserTrades,
		// "user.trades.snapshot": p.handleUserTradesSnapshot,
	}
}

func (p *PintuPro) getWsHandler(method, channel string) (wsHandlerFunc, error) {
	handler, ok := p.wsHandlers[method]
	if ok {
		return handler, nil
	}

	handler, ok = p.wsHandlers[channel]
	if ok {
		return handler, nil
	}

	for k, h := range p.wsHandlers {
		if strings.HasPrefix(channel, k) {
			return h, nil
		}
	}

	err := fmt.Errorf("unsupported method: %s", method)
	if channel != "" {
		err = fmt.Errorf("unsupported channel: %s", channel)
	}

	return nil, err
}

func (p *PintuPro) handleHeartbeat(msg wsMessage, _ chan<- models.ExchangeMessage) error {
	req := wsMessage{
		RequestID: msg.RequestID,
		Timestamp: time.Now().UnixMilli(),
		Method:    "heartbeat-response",
	}
	b, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal heartbeat response: %w", err)
	}
	return p.ws.Write(context.Background(), b)
}

func (p *PintuPro) handleSubscription(msg wsMessage, _ chan<- models.ExchangeMessage) error {
	if msg.Code != 0 {
		return fmt.Errorf("subscription error: %s %s", msg.Message, msg.Reason)
	}
	var sub struct {
		Channel string `json:"channel"`
	}
	if err := json.Unmarshal(msg.Data, &sub); err != nil {
		return fmt.Errorf("failed to unmarshal subscription: %w %s", err, msg.Data)
	}
	log.Printf("subscribed to %s", sub.Channel)
	p.mux.Lock()
	p.subscribedStreams = append(p.subscribedStreams, sub.Channel)
	p.mux.Unlock()

	return nil
}

// easyjson:json
type walletSnapshotMsg struct {
	Balance   decimal.Decimal `json:"balance"`
	Available decimal.Decimal `json:"available"`
	Order     decimal.Decimal `json:"order"`
}

// easyjson:json
type walletsMsg map[string]walletSnapshotMsg

// easyjson:json
type balanceSnapshotMsg struct {
	Assets walletsMsg `json:"assets"`
}

func (p *PintuPro) handleUserBalance(msg wsMessage, ch chan<- models.ExchangeMessage) error {
	var balances balanceSnapshotMsg
	if err := json.Unmarshal(msg.Data, &balances); err != nil {
		return fmt.Errorf("failed to unmarshal balance snapshot: %w", err)
	}

	for asset, wallet := range balances.Assets {
		ch <- models.ExchangeMessage{
			Exchange:  Name,
			Symbol:    asset,
			Timestamp: tsToTime(msg.Timestamp),
			MsgType:   models.MsgTypeBalanceUpdate,
			Payload: models.Balance{
				Total:     wallet.Balance,
				Available: wallet.Available,
				UpdatedAt: time.Now(),
			},
		}
	}

	return nil
}

// easyjson:json
type tradesMsg struct {
	Trades []struct {
		Side      string `json:"side"`
		Price     string `json:"price"`
		Size      string `json:"size"`
		Timestamp int64  `json:"timestamp"`
		Symbol    string `json:"symbol"`
	} `json:"trades"`
}

func tsToTime(ts int64) time.Time {
	return time.Unix(0, ts*int64(time.Millisecond))
}

func (p *PintuPro) handlePublicTrades(msg wsMessage, ch chan<- models.ExchangeMessage) error {
	var trades tradesMsg
	if err := json.Unmarshal(msg.Data, &trades); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	for _, trade := range trades.Trades {
		side := models.OrderSideBuy
		if trade.Side == "SELL" {
			side = models.OrderSideSell
		}

		price, err := decimal.NewFromString(trade.Price)
		if err != nil {
			return fmt.Errorf("failed to parse price: %w", err)
		}

		size, err := decimal.NewFromString(trade.Size)
		if err != nil {
			return fmt.Errorf("failed to parse size: %w", err)
		}

		ch <- models.ExchangeMessage{
			Exchange:  Name,
			Symbol:    trade.Symbol,
			Timestamp: tsToTime(trade.Timestamp),
			MsgType:   models.MsgTypeTrade,
			Payload: models.Trade{
				Side:      side,
				Timestamp: tsToTime(trade.Timestamp),
				Price:     price,
				Size:      size,
			},
		}
	}

	return nil
}

// easyjson:json
type orderBookMsg struct {
	Symbol string     `json:"symbol"`
	Bids   [][]string `json:"bids"`
	Asks   [][]string `json:"asks"`
}

func (p *PintuPro) handleAuth(msg wsMessage, ch chan<- models.ExchangeMessage) error {
	if msg.Code != 0 {
		return fmt.Errorf("auth error: %s %s", msg.Message, msg.Reason)
	}

	log.Println("authenticated")
	return nil
}

func (p *PintuPro) handleOrderBook(msg wsMessage, ch chan<- models.ExchangeMessage) error {
	var ob orderBookMsg
	if err := json.Unmarshal(msg.Data, &ob); err != nil {
		return fmt.Errorf("failed to unmarshal orderbook: %w", err)
	}

	// For now I don't care much about depth, so I will just take the first level.
	bbo := models.BBO{
		Timestamp: tsToTime(msg.Timestamp),
		Bid:       models.PriceLevel{},
		Ask:       models.PriceLevel{},
	}

	if len(ob.Bids) > 0 {
		bbo.Bid.Price, _ = decimal.NewFromString(ob.Bids[0][0])
		bbo.Bid.Size, _ = decimal.NewFromString(ob.Bids[0][1])
	}

	if len(ob.Asks) > 0 {
		bbo.Ask.Price, _ = decimal.NewFromString(ob.Asks[0][0])
		bbo.Ask.Size, _ = decimal.NewFromString(ob.Asks[0][1])
	}

	ch <- models.ExchangeMessage{
		Exchange:  Name,
		Symbol:    ob.Symbol,
		Timestamp: tsToTime(msg.Timestamp),
		MsgType:   models.MsgTypeBBO,
		Payload:   bbo,
	}

	return nil
}

func (p *PintuPro) Listen(ctx context.Context, ch chan<- models.ExchangeMessage) {
	defer close(ch)
	errCnt := 0
	rawCh := make(chan []byte, 100)
	go func() {
		for {
			err := p.ws.Listen(rawCh)
			if err != nil {
				log.Printf("PintuPro.Listen returned: %v\n", err)
			}

			select {
			case <-ctx.Done():
				return
			default:
				msg := "normal close"
				if err != nil {
					msg = err.Error()
				}
				p.reconnectCh <- msg
				// Wait for some time before listening again.
				time.Sleep(time.Second)
			}
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
	loop:
		if errCnt > 10 {
			p.reconnectCh <- "too many errors"
			errCnt = 0
			goto loop
		}

		select {
		case <-ticker.C:
			ts := atomic.LoadInt64(&p.lastReceived)
			if time.Since(time.Unix(0, ts)) > p.idleTimeout {
				p.reconnectCh <- fmt.Sprintf("no messages for %s", p.idleTimeout)
				time.Sleep(time.Second)
				goto loop
			}
		case msg, ok := <-rawCh:
			if !ok {
				return
			}

			atomic.StoreInt64(&p.lastReceived, time.Now().UnixNano())

			var r wsMessage
			if err := json.Unmarshal(msg, &r); err != nil {
				log.Printf("failed to unmarshal msg: %v\n%v\n", err, string(msg))
				errCnt++
				goto loop
			}

			if r.Code != 0 {
				log.Printf("%s:%s %s", r.Method, r.Message, r.Reason)
				goto loop
			}

			handler, err := p.getWsHandler(r.Method, r.Channel)
			if err != nil {
				log.Println(err)
				goto loop
			}

			if err := handler(r, ch); err != nil {
				log.Printf("handler error: %v", err)
				errCnt++
				goto loop
			}
		}
	}
}
