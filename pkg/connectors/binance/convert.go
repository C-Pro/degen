package binance

import (
	"strings"

	"degen/pkg/models"
)

func symbolToExchange(symbol string) string {
	return strings.ToUpper(symbol)
}

func symbolFromExchange(exchangeSymbol string) string {
	return strings.ToLower(exchangeSymbol)
}

var statusToEx = map[models.OrderStatus]string{
	models.OrderStatusPlaced:          "NEW",
	models.OrderStatusPartiallyFilled: "PARTIALLY_FILLED",
	models.OrderStatusFilled:          "FILLED",
	models.OrderStatusCanceled:        "CANCELED",
}

func orderStatusToExchange(status models.OrderStatus) string {
	return statusToEx[status]
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

func typeFromEx(tp string) models.OrderType {
	return models.OrderType(strings.ToLower(tp))
}

func typeToEx(tp models.OrderType) string {
	return strings.ToUpper(string(tp))
}
