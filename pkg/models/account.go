package models

import (
	"time"

	"github.com/shopspring/decimal"
)

type AccountInfo struct {
	Balances  map[string]Balance
	Positions map[string]Position
	UpdatedAt time.Time
}

type Balance struct {
	Total     decimal.Decimal
	Available decimal.Decimal
	UpdatedAt time.Time
}

type Position struct {
	// Positive amount means long position, negative - short.
	Amount       decimal.Decimal
	AveragePrice decimal.Decimal
	UpdatedAt    time.Time
	RealizedPnL  decimal.Decimal
}

func (p *Position) UnrealizedPnL(price decimal.Decimal) decimal.Decimal {
	return p.Amount.Mul(price.Sub(p.AveragePrice))
}
