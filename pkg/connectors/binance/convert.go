package binance

import (
	"strings"
	"time"

	"degen/pkg/models"
)

func timestampToTime(ms int64) time.Time {
	return time.Unix(0, ms*1000*1000).UTC()
}

func symbolToExchange(symbol string) string {
	return strings.ToUpper(symbol)
}

func symbolFromExchange(exchangeSymbol string) string {
	return strings.ToLower(exchangeSymbol)
}

var statusFromEx = map[string]models.OrderStatus{
	"NEW":              models.OrderStatusPlaced,
	"PARTIALLY_FILLED": models.OrderStatusPartiallyFilled,
	"FILLED":           models.OrderStatusFilled,
	"CANCELED":         models.OrderStatusCanceled,
	"EXPIRED":          models.OrderStatusCanceled,
}

func orderStatusFromExchange(status string) models.OrderStatus {
	return statusFromEx[status]
}

func typeToEx(tp models.OrderType) string {
	return strings.ToUpper(string(tp))
}
