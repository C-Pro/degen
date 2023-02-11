package models

import (
	"time"

	"github.com/shopspring/decimal"
)

type OrderSide string

const (
	OrderSideBuy  OrderSide = "buy"
	OrderSideSell OrderSide = "sell"
)

type OrderType string

const (
	OrderTypeMarket OrderType = "market"
	OrderTypeLimit  OrderType = "limit"
	OrderTypeStop   OrderType = "stop"
	OrderTypeTP     OrderType = "take_profit"
)

type OrderStatus string

const (
	OrderStatusNew             OrderStatus = "new"
	OrderStatusPlaced          OrderStatus = "placed"
	OrderStatusPartiallyFilled OrderStatus = "partially_filled"
	OrderStatusFilled          OrderStatus = "filled"
	OrderStatusRejected        OrderStatus = "rejected"
	OrderStatusCanceled        OrderStatus = "canceled"
)

type TimeInForce string

const (
	TimeInForceGTC TimeInForce = "GTC"
	TimeInForceIOC TimeInForce = "IOC"
	TimeInForceFOK TimeInForce = "FOK"
	TimeInForceGTX TimeInForce = "GTX"
)

type Order struct {
	ClientOrderID   string
	ExchangeOrderID string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Symbol          string
	Base            string
	Quote           string
	Side            OrderSide
	Type            OrderType
	TimeInForce     TimeInForce
	Status          OrderStatus

	Size  decimal.Decimal
	Price decimal.Decimal

	FilledSize   decimal.Decimal
	AveragePrice decimal.Decimal
}

type OrderUpdate struct {
	ClientOrderID   string
	ExchangeOrderID string
	UpdatedAt       time.Time
	Status          OrderStatus
	Side            OrderSide
	Symbol          string

	FilledSize   decimal.Decimal
	AveragePrice decimal.Decimal
}
