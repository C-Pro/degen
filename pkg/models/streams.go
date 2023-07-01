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
	MsgTypeTrade
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

type BalanceUpdate struct {
	Asset   string
	Balance decimal.Decimal
}

type PositionUpdate struct {
	Symbol     string
	Amount     decimal.Decimal
	EntryPrice decimal.Decimal
}

type Trade struct {
	Side      OrderSide
	Size      decimal.Decimal
	Price     decimal.Decimal
	Timestamp time.Time
}
