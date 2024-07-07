package account

import (
	"context"
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
	PlaceOrder(ctx context.Context, order models.Order) (*models.Order, error)
	CancelOrder(ctx context.Context, order models.Order) (*models.Order, error)
	Listen(ctx context.Context, ch chan<- models.ExchangeMessage)
	SubscribeBookTickers(ctx context.Context, symbols []string) error
	SubscribeBookAggTrades(ctx context.Context, symbols []string) error
	SubscribeUserOrders(ctx context.Context) error
	SubscribeUserBalance(ctx context.Context) error
}

type Account struct {
	exchange
	id        string
	balances  map[string]models.Balance
	positions map[string]models.Position
	orders    *geche.KV[models.Order]
	stopWg    sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc

	mux sync.RWMutex
}

func NewAccount(id string, api exchange) *Account {
	return &Account{
		id:        id,
		exchange:  api,
		balances:  make(map[string]models.Balance),
		positions: make(map[string]models.Position),
		orders:    geche.NewKV[models.Order](geche.NewMapCache[string, models.Order]()),
	}
}

func (a *Account) Start(ctx context.Context) error {
	info, err := a.GetAccountInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get initial account info: %w", err)
	}

	a.balances = info.Balances
	a.positions = info.Positions

	a.ctx, a.cancel = context.WithCancel(ctx)
	a.stopWg.Add(2)
	ch := make(chan models.ExchangeMessage, 100)
	go func() {
		a.Listen(a.ctx, ch)
		a.stopWg.Done()
	}()

	go func() {
		a.updateLoop(a.ctx, ch)
		a.stopWg.Done()
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

func (a *Account) Stop() {
	a.cancel()
	a.stopWg.Wait()
}

func (a *Account) updateLoop(ctx context.Context, ch chan models.ExchangeMessage) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			if err := a.Update(msg); err != nil {
				return
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

	a.balances[asset] = models.Balance{
		Total:     balance,
		UpdatedAt: updatedAt,
	}
}

func (a *Account) UpdatePosition(
	symbol string,
	amount decimal.Decimal,
	entryPrice decimal.Decimal,
	updatedAt time.Time,
) {
	a.mux.Lock()
	defer a.mux.Unlock()

	a.positions[symbol] = models.Position{
		Amount:     amount,
		EntryPrice: entryPrice,
		UpdatedAt:  updatedAt,
	}
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
		log.Printf("%#v", order)
		metrics.RecordPlaceOrderDuration(
			a.exchange.Name(),
			existing.PlacedAt,
		)
	}
	if order.Final {
		// nolint:errcheck
		a.orders.Del(key)
		return
	}

	a.orders.Set(key, order)
}

func (a *Account) GetBalance(asset string) models.Balance {
	a.mux.RLock()
	defer a.mux.RUnlock()

	return a.balances[asset]
}

func (a *Account) GetPosition(symbol string) models.Position {
	a.mux.RLock()
	defer a.mux.RUnlock()

	return a.positions[symbol]
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
		bal, ok := upd.Payload.(models.BalanceUpdate)
		if !ok {
			return fmt.Errorf("invalid payload type %T for MsgType %q", upd.Payload, upd.MsgType)
		}
		a.UpdateBalance(bal.Asset, bal.Balance, decimal.Zero, upd.Timestamp)
	case models.MsgTypePositionUpdate:
		pos, ok := upd.Payload.(models.PositionUpdate)
		if !ok {
			return fmt.Errorf("invalid payload type %T for MsgType %q", upd.Payload, upd.MsgType)
		}
		a.UpdatePosition(upd.Symbol, pos.Amount, pos.EntryPrice, upd.Timestamp)
	}

	return nil
}

func (a *Account) PlaceOrder(ctx context.Context, order models.Order) (*models.Order, error) {
	order.PlacedAt = time.Now().UTC()
	order.Status = models.OrderStatusNew
	o, err := a.exchange.PlaceOrder(ctx, order)
	if err != nil {
		return nil, err
	}

	a.orders.Set(orderKey(order), *o)
	return o, nil
}

func (a *Account) CancelOrder(ctx context.Context, order models.Order) (*models.Order, error) {
	return a.exchange.CancelOrder(ctx, order)
}

func (a *Account) GetOrder(symbol, clientOrderID string) *models.Order {
	key := fmt.Sprintf("%s:%s", symbol, clientOrderID)
	o, _ := a.orders.Get(key)
	return &o
}

func (a *Account) GetOrders(symbol string) []models.Order {
	orders, _ := a.orders.ListByPrefix(symbol + ":")
	return orders
}
