package models

import (
	"fmt"
	"sync"
	"time"

	"github.com/c-pro/geche"
	"github.com/shopspring/decimal"
)

type Account struct {
	id        string
	exchange  string
	balances  map[string]Balance
	positions map[string]Position
	orders    geche.Geche[string, Order]

	mux sync.RWMutex
}

func NewAccount(id, exchange string) *Account {
	return &Account{
		id:        id,
		exchange:  exchange,
		balances:  make(map[string]Balance),
		positions: make(map[string]Position),
		orders:    geche.NewKV[Order](geche.NewMapCache[string, Order]()),
	}
}

type Balance struct {
	Balance   decimal.Decimal
	Locked    decimal.Decimal
	UpdatedAt time.Time
}

func (b *Balance) Available() decimal.Decimal {
	return b.Balance.Sub(b.Locked)
}

type Position struct {
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
		Balance:   balance,
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
