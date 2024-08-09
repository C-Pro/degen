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
}
