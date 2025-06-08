package models

import (
	"time"

	"github.com/shopspring/decimal"
)

type MsgType uint8

const (
	MsgTypeBBO = iota
	MsgTypeOrderStatus
	MsgTypeBalanceUpdate
	MsgTypePositionUpdate
	MsgTypePublicTrade
	MsgTypeMarketTicker
)

type ExchangeMessage struct {
	Exchange  string
	Symbol    string
	Timestamp time.Time
	MsgType   MsgType
	Payload   any
}

type PriceLevel struct {
	Price decimal.Decimal
	Size  decimal.Decimal
}

type BBO struct {
	Bid       PriceLevel
	Ask       PriceLevel
	Timestamp time.Time
}

func (b BBO) Midprice() decimal.Decimal {
	return b.Bid.Price.Add(b.Ask.Price).Div(decimal.NewFromInt(2))
}

func (b BBO) Spread() (decimal.Decimal, bool) {
	if b.Bid.Price.IsZero() || b.Ask.Price.IsZero() {
		return decimal.Zero, false
	}

	return b.Ask.Price.Sub(b.Bid.Price).Div(b.Ask.Price), true
}

type OrderBook struct {
	Symbol string
	// Public order book data is already outdated when we receive it. It does not
	// make sense to pay decimal.Decimal overhead for the precision.
	Bids      [][2]float64
	Asks      [][2]float64
	Timestamp time.Time
}

type BalanceUpdate struct {
	Asset   string
	Balance decimal.Decimal
}

type PositionUpdate struct {
	ID        string
	Symbol    string
	Amount    decimal.Decimal
	Price     decimal.Decimal
	Timestamp time.Time
}

type Trade struct {
	ID        string
	Side      OrderSide
	Size      decimal.Decimal
	Price     decimal.Decimal
	Timestamp time.Time
}
