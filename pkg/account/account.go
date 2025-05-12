package account

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/c-pro/geche"
	"github.com/shopspring/decimal"

	"degen/pkg/metrics"
	"degen/pkg/models"
)

type exchange interface {
	Name() string
	GetSymbols(ctx context.Context) (map[string]models.SymbolInfo, error)
	GetAccountInfo(ctx context.Context) (*models.AccountInfo, error)
	GetOrderDetails(ctx context.Context, order models.Order) (*models.Order, error)
	PlaceOrder(ctx context.Context, order models.Order) (*models.Order, error)
	CancelOrder(ctx context.Context, order models.Order) (*models.Order, error)
	CancelAllOrders(ctx context.Context, symbol string) error
	GetOpenOrders(ctx context.Context, symbol string) ([]models.Order, error)
	Listen(ctx context.Context, ch chan<- models.ExchangeMessage)
	SubscribeBookTickers(ctx context.Context, symbols []string) error
	SubscribeBookAggTrades(ctx context.Context, symbols []string) error
	SubscribeUserOrders(ctx context.Context) error
	SubscribeUserBalance(ctx context.Context) error
	SubscribeUserTrades(ctx context.Context) error
	RequestReconnect(reason string)
}

type strategyCallback (func(upd models.ExchangeMessage))

type Account struct {
	exchange
	id        string
	balances  map[string]models.Balance
	positions map[string]*positionStructure
	orders    *geche.KV[models.Order]
	interest  map[string]*openInterest
	ctx       context.Context
	cancel    context.CancelFunc
	strategy  strategyCallback
	errCh     chan error

	mux sync.RWMutex
}

func NewAccount(id string, api exchange) (*Account, error) {
	a := &Account{
		id:        id,
		exchange:  api,
		balances:  make(map[string]models.Balance),
		positions: make(map[string]*positionStructure),
		orders:    geche.NewKV(geche.NewMapCache[string, models.Order]()),
		interest:  make(map[string]*openInterest),
		errCh:     make(chan error),
	}

	info, err := a.GetAccountInfo(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get initial account info: %w", err)
	}

	a.balances = info.Balances
	for sym, pos := range info.Positions {
		a.positions[sym] = &positionStructure{}
		a.positions[sym].Update(models.PositionUpdate{
			Amount:    pos.Amount,
			Price:     pos.AveragePrice,
			Timestamp: pos.UpdatedAt,
		})
	}

	orders, err := api.GetOpenOrders(context.Background(), "")
	if err != nil {
		return nil, fmt.Errorf("failed to get open orders: %w", err)
	}
	for _, order := range orders {
		if err := a.Update(models.ExchangeMessage{
			Exchange: api.Name(),
			MsgType:  models.MsgTypeOrderStatus,
			Symbol:   order.Symbol,
			Payload:  order,
		},
		); err != nil {
			return nil, fmt.Errorf("failed to update order: %w", err)
		}
	}

	return a, nil
}

func (a *Account) SetStrategy(cb strategyCallback) {
	a.mux.Lock()
	defer a.mux.Unlock()
	a.strategy = cb
}

func (a *Account) Start(ctx context.Context) error {
	a.ctx, a.cancel = context.WithCancel(ctx)
	ch := make(chan models.ExchangeMessage, 100)
	go func() {
		a.Listen(a.ctx, ch)
		close(ch)
	}()

	go func() {
		a.updateLoop(a.ctx, ch)
	}()

	return nil
}

func (a *Account) SubscribeSymbols(symbols []string) error {
	if err := a.SubscribeBookAggTrades(a.ctx, symbols); err != nil {
		return fmt.Errorf("failed to subscribe %v", err)
	}
	if err := a.SubscribeBookTickers(a.ctx, symbols); err != nil {
		return fmt.Errorf("failed to subscribe %v", err)
	}
	return nil
}

func (a *Account) updateLoop(ctx context.Context, ch chan models.ExchangeMessage) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			if (msg.MsgType != models.MsgTypeBBO) && (msg.MsgType != models.MsgTypeBalanceUpdate) {
				var mt string
				switch msg.MsgType {
				case models.MsgTypeOrderStatus:
					mt = "order"
				case models.MsgTypePositionUpdate:
					mt = "position"
				}
				log.Printf("Received %s message: %v\n", mt, msg)
			}
			if err := a.Update(msg); err != nil {
				return
			}
			a.mux.RLock()
			strategy := a.strategy
			a.mux.RUnlock()
			if strategy != nil {
				strategy(msg)
			}
		}
	}
}

func (a *Account) UpdateBalance(
	asset string,
	balance decimal.Decimal,
	locked decimal.Decimal,
	updatedAt time.Time,
) {
	a.mux.Lock()
	defer a.mux.Unlock()

	metrics.RecordAssetBalance(a.exchange.Name(), asset, balance.InexactFloat64())

	a.balances[asset] = models.Balance{
		Total:     balance,
		UpdatedAt: updatedAt,
	}
}

func (a *Account) GetOrder(symbol, clientOrderID string) *models.Order {
	o, err := a.orders.Get(fmt.Sprintf("%s:%s", symbol, clientOrderID))
	if err != nil {
		return nil
	}

	return &o
}

func (a *Account) GetOpenOrders(symbol string) []models.Order {
	prefix := ""
	if symbol != "" {
		prefix = symbol + ":"
	}

	orders, _ := a.orders.ListByPrefix(prefix)

	return orders
}

func (a *Account) GetTotalBidSize(symbol string) decimal.Decimal {
	a.mux.RLock()
	defer a.mux.RUnlock()

	oi, ok := a.interest[symbol]
	if !ok {
		return decimal.Zero
	}

	return oi.totalBidSize
}

func (a *Account) GetTotalAskSize(symbol string) decimal.Decimal {
	a.mux.RLock()
	defer a.mux.RUnlock()

	oi, ok := a.interest[symbol]
	if !ok {
		return decimal.Zero
	}

	return oi.totalAskSize
}

func (a *Account) UpdatePosition(
	symbol string,
	amount decimal.Decimal,
	price decimal.Decimal,
	updatedAt time.Time,
) {
	log.Printf("Updating position %s: %s %s\n", symbol, amount, price)
	a.mux.Lock()
	defer a.mux.Unlock()

	pos, ok := a.positions[symbol]
	if !ok {
		return
	}

	pos.Update(models.PositionUpdate{
		Amount:    amount,
		Price:     price,
		Timestamp: updatedAt,
	})

	metrics.RecordPosition(a.exchange.Name(), symbol, pos.Position())
}

func orderKey(order models.Order) string {
	return fmt.Sprintf("%s:%s", order.Symbol, order.ClientOrderID)
}

func (a *Account) UpdateOrder(order models.Order) {
	key := orderKey(order)
	existing, err := a.orders.Get(key)
	if err == nil && existing.Status == models.OrderStatusNew {
		log.Printf("Order time to book: %s\n", order.CreatedAt.Sub(existing.PlacedAt))
		log.Printf("Order e2e time: %s\n", order.UpdatedAt.Sub(existing.PlacedAt))
		metrics.RecordPlaceOrderDuration(
			a.exchange.Name(),
			existing.PlacedAt,
		)
	}
	if order.Final {
		// nolint:errcheck
		a.orders.Del(key)
		log.Printf("order %s () deleted", key)
		return
	}

	a.orders.Set(key, order)
	if err := a.interest[order.Symbol].observe(order); err != nil {
		log.Printf("failed to observe order: %v\n", err)
		orders, err := a.exchange.GetOpenOrders(context.Background(), order.Symbol)
		if err != nil {
			a.errCh <- fmt.Errorf("failed to get open orders: %w", err)
			return
		}

		a.interest[order.Symbol].setFromOrders(orders)
	}
}

func (a *Account) GetBalance(asset string) models.Balance {
	a.mux.RLock()
	defer a.mux.RUnlock()

	return a.balances[asset]
}

func (a *Account) GetPosition(symbol string) models.Position {
	a.mux.RLock()
	defer a.mux.RUnlock()

	pos, ok := a.positions[symbol]
	if !ok {
		return models.Position{}
	}

	return pos.Position()
}

func (a *Account) Update(upd models.ExchangeMessage) error {
	switch upd.MsgType {
	case models.MsgTypeOrderStatus:
		order, ok := upd.Payload.(models.Order)
		if !ok {
			return fmt.Errorf("invalid payload type %T for MsgType %q", upd.Payload, upd.MsgType)
		}
		a.UpdateOrder(order)
	case models.MsgTypeBalanceUpdate:
		bal, ok := upd.Payload.(models.Balance)
		if !ok {
			return fmt.Errorf("invalid payload type %T for MsgType %q", upd.Payload, upd.MsgType)
		}
		a.UpdateBalance(upd.Symbol, bal.Total, bal.Total.Sub(bal.Available), upd.Timestamp)
	case models.MsgTypePositionUpdate:
		pos, ok := upd.Payload.(models.PositionUpdate)
		if !ok {
			return fmt.Errorf("invalid payload type %T for MsgType %q", upd.Payload, upd.MsgType)
		}
		a.UpdatePosition(upd.Symbol, pos.Amount, pos.Price, upd.Timestamp)
	case models.MsgTypeBBO:
		bbo, ok := upd.Payload.(models.BBO)
		if !ok {
			return fmt.Errorf("invalid payload type %T for MsgType %q", upd.Payload, upd.MsgType)
		}
		metrics.RecordBBO(a.exchange.Name(), upd.Symbol, bbo)
		position, ok := a.positions[upd.Symbol]
		if ok && !position.totalSize.IsZero() {
			position := position.Position()
			price := bbo.Ask.Price
			if position.Amount.Sign() > 0 {
				price = bbo.Bid.Price
			}
			metrics.RecordUnrealizedPnL(
				a.exchange.Name(),
				upd.Symbol,
				position.UnrealizedPnL(price).InexactFloat64(),
			)
		}
	}

	return nil
}

func (a *Account) PlaceOrder(ctx context.Context, order models.Order) (*models.Order, error) {
	order.PlacedAt = time.Now().UTC()
	order.Status = models.OrderStatusNew
	a.orders.Set(orderKey(order), order)
	o, err := a.exchange.PlaceOrder(ctx, order)
	if err != nil {
		// nolint:errcheck
		a.orders.Del(orderKey(order))
		return nil, err
	}

	if o.Final {
		// nolint:errcheck
		a.orders.Del(orderKey(order))
	}

	return o, nil
}

func (a *Account) CancelOrder(ctx context.Context, order models.Order) (*models.Order, error) {
	o, err := a.exchange.CancelOrder(ctx, order)
	if time.Since(order.PlacedAt) > time.Second*10 && errors.Is(err, models.ErrOrderNotFound) {
		// nolint:errcheck
		a.orders.Del(orderKey(order))
		order.Status = models.OrderStatusCanceled
		return &order, nil
	}

	return o, err
}

func (a *Account) CancelAllOrders(ctx context.Context, symbol string) error {
	return a.exchange.CancelAllOrders(ctx, symbol)
}

func (a *Account) syncOrders(ctx context.Context, symbol string) error {
	orders, _ := a.orders.ListByPrefix(symbol + ":")
	for _, o := range orders {
		order, err := a.exchange.GetOrderDetails(ctx, o)
		if err != nil {
			return fmt.Errorf("failed to get order details: %w", err)
		}

		a.UpdateOrder(*order)
	}

	return nil
}

func (a *Account) SyncWithExchange(ctx context.Context, symbols []string) error {
	// Wait for some time for ws updates to come in.
	time.Sleep(time.Second)

	for _, symbol := range symbols {
		if err := a.syncOrders(ctx, symbol); err != nil {
			return fmt.Errorf("failed to sync orders: %w", err)
		}
	}
	a.exchange.RequestReconnect("sync state")
	return nil
}
