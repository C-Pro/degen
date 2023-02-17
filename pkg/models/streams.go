package models

import (
	"time"

	"github.com/shopspring/decimal"
)

type MsgType uint8

const (
	MsgTypeTopBid = iota
	MsgTypeTopAsk
	MsgTypeOrderStatus
	MsgTypeBalanceUpdate
	MsgTypePositionUpdate
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

type BalanceUpdate struct {
	Asset   string
	Balance decimal.Decimal
}

type PositionUpdate struct {
	Symbol     string
	Amount     decimal.Decimal
	EntryPrice decimal.Decimal
}
