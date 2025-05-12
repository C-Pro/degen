package models

import "github.com/shopspring/decimal"

type SymbolInfo struct {
	Symbol           string
	Base             string
	Quote            string
	PriceTickSize    decimal.Decimal
	QuantityTickSize decimal.Decimal
	MinQuantity      decimal.Decimal
	MaxQuantity      decimal.Decimal
	MinPrice24h      decimal.Decimal
	MaxPrice24h      decimal.Decimal
	Volume24h        decimal.Decimal
	QuoteVolume24h   decimal.Decimal
}
