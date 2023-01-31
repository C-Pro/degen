package models

import (
	"time"

	"github.com/shopspring/decimal"
)

type MsgType uint8

const (
	MsgTypeTopBid = iota
	MsgTypeTopAsk
)

type ExchangeMessage struct {
	Exchange  string
	Symbol    string
	Timestamp time.Time
	MsgType MsgType
	Payload   any
}

type PriceLevel struct {
	Price decimal.Decimal
	Size  decimal.Decimal
}
