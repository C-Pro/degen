package models

import (
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

type Account struct {
	id        string
	exchange  string
	balances  map[string]Balance
	positions map[string]Position

	mux sync.RWMutex
}

func NewAccount(id, exchange string) *Account {
	return &Account{
		id:        id,
		exchange:  exchange,
		balances:  make(map[string]Balance),
		positions: make(map[string]Position),
	}
}

type Balance struct {
	Balance   decimal.Decimal
	UpdatedAt time.Time
}

type Position struct {
	Amount     decimal.Decimal
	EntryPrice decimal.Decimal
	UpdatedAt  time.Time
}

func (a *Account) UpdateBalance(
	asset string,
	balance decimal.Decimal,
	updatedAt time.Time,
) {
	// TODO: if all updates come from single channel,
	// can remove locking.
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
	// TODO: if all updates come from single channel,
	// can remove locking.
	a.mux.Lock()
	defer a.mux.Unlock()

	a.positions[symbol] = Position{
		Amount:     amount,
		EntryPrice: entryPrice,
		UpdatedAt:  updatedAt,
	}
}
