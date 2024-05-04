package models

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/c-pro/geche"
	"github.com/shopspring/decimal"
)

type exchange interface {
	GetAccountInfo(ctx context.Context) (*AccountInfo, error)
	PlaceOrder(ctx context.Context, order Order) (*Order, error)
	CancelOrder(ctx context.Context, order Order) (*Order, error)
	Listen(ctx context.Context, ch chan<- ExchangeMessage)
	SubscribeBookTickers(ctx context.Context, symbols []string) error
	SubscribeBookAggTrades(ctx context.Context, symbols []string) error
}

type AccountInfo struct {
	Balances  map[string]Balance
	Positions map[string]Position
	UpdatedAt time.Time
}

type Account struct {
	id        string
	api       exchange
	balances  map[string]Balance
	positions map[string]Position
	orders    geche.Geche[string, Order]
	stopWg    sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc

	mux sync.RWMutex
}

func NewAccount(id string, api exchange) *Account {
	return &Account{
		id:        id,
		api:       api,
		balances:  make(map[string]Balance),
		positions: make(map[string]Position),
		orders:    geche.NewKV[Order](geche.NewMapCache[string, Order]()),
	}
}

func (a *Account) Start(ctx context.Context) error {
	info, err := a.api.GetAccountInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get initial account info: %w", err)
	}

	a.balances = info.Balances
	a.positions = info.Positions

	a.ctx, a.cancel = context.WithCancel(ctx)
	a.stopWg.Add(2)
	ch := make(chan ExchangeMessage, 100)
	go func() {
		a.api.Listen(a.ctx, ch)
		a.stopWg.Done()
	}()

	go func() {
		a.updateLoop(a.ctx, ch)
		a.stopWg.Done()
	}()

	return nil
}

func (a *Account) SubscribeSymbols(symbols []string) error {
	if err := a.api.SubscribeBookAggTrades(a.ctx, symbols); err != nil {
		return fmt.Errorf("failed to subscribe %v", err)
	}
	if err := a.api.SubscribeBookTickers(a.ctx, symbols); err != nil {
		return fmt.Errorf("failed to subscribe %v", err)
	}
	return nil
}

func (a *Account) Stop() {
	a.cancel()
	a.stopWg.Wait()
}

func (a *Account) updateLoop(ctx context.Context, ch chan ExchangeMessage) {
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

type Balance struct {
	Total     decimal.Decimal
	Available decimal.Decimal
	UpdatedAt time.Time
}

type Position struct {
	// Positive amount means long position, negative - short.
	Amount     decimal.Decimal
	EntryPrice decimal.Decimal
	UpdatedAt  time.Time
}

func (a *Account) UpdateBalance(
	asset string,
	balance decimal.Decimal,
	locked decimal.Decimal,
	updatedAt time.Time,
) {
	a.mux.Lock()
	defer a.mux.Unlock()

	a.balances[asset] = Balance{
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

	a.positions[symbol] = Position{
		Amount:     amount,
		EntryPrice: entryPrice,
		UpdatedAt:  updatedAt,
	}
}

func orderKey(order Order) string {
	return fmt.Sprintf("%s:%s", order.Symbol, order.ClientOrderID)
}

func (a *Account) UpdateOrder(order Order) {
	key := orderKey(order)
	if order.Final {
		a.orders.Del(key)
		return
	}

	a.orders.Set(key, order)
}

func (a *Account) GetBalance(asset string) Balance {
	a.mux.RLock()
	defer a.mux.RUnlock()

	return a.balances[asset]
}

func (a *Account) GetPosition(symbol string) Position {
	a.mux.RLock()
	defer a.mux.RUnlock()

	return a.positions[symbol]
}

func (a *Account) Update(upd ExchangeMessage) error {
	switch upd.MsgType {
	case MsgTypeOrderStatus:
		order, ok := upd.Payload.(Order)
		if !ok {
			return fmt.Errorf("invalid payload type %T for MsgType %q", upd.Payload, upd.MsgType)
		}
		a.UpdateOrder(order)
	case MsgTypeBalanceUpdate:
		bal, ok := upd.Payload.(BalanceUpdate)
		if !ok {
			return fmt.Errorf("invalid payload type %T for MsgType %q", upd.Payload, upd.MsgType)
		}
		a.UpdateBalance(bal.Asset, bal.Balance, decimal.Zero, upd.Timestamp)
	case MsgTypePositionUpdate:
		pos, ok := upd.Payload.(PositionUpdate)
		if !ok {
			return fmt.Errorf("invalid payload type %T for MsgType %q", upd.Payload, upd.MsgType)
		}
		a.UpdatePosition(upd.Symbol, pos.Amount, pos.EntryPrice, upd.Timestamp)
	}

	return nil
}

func (a *Account) PlaceOrder(ctx context.Context, order Order) (*Order, error) {
	return a.api.PlaceOrder(ctx, order)
}

func (a *Account) CancelOrder(ctx context.Context, order Order) (*Order, error) {
	return a.api.CancelOrder(ctx, order)
}
