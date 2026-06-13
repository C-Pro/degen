package pintupro

import (
	"fmt"
	"strings"

	"degen/pkg/models"
)

// This file centralizes parsing of PintuPro enum strings and order conversion
// so that the REST and WebSocket code paths cannot drift. The exchange is not
// consistent about casing (the WS samples use lowercase "buy"/"sell" while some
// REST samples use uppercase), so every parser is case-insensitive.

// parseSide maps an exchange side string to a models.OrderSide. It is
// case-insensitive; anything that is not "sell" is treated as a buy, matching
// the historical default but no longer silently mis-classifying lowercase
// "sell" as a buy.
func parseSide(s string) models.OrderSide {
	if strings.EqualFold(s, "SELL") {
		return models.OrderSideSell
	}

	return models.OrderSideBuy
}

func parseType(s string) (models.OrderType, error) {
	switch strings.ToUpper(s) {
	case "LIMIT":
		return models.OrderTypeLimit, nil
	case "MARKET":
		return models.OrderTypeMarket, nil
	default:
		return "", fmt.Errorf("unknown order type: %s", s)
	}
}

func parseStatus(s string) (models.OrderStatus, error) {
	switch strings.ToUpper(s) {
	case "PLACED":
		return models.OrderStatusPlaced, nil
	case "PARTIALLY_FILLED":
		return models.OrderStatusPartiallyFilled, nil
	case "FILLED":
		return models.OrderStatusFilled, nil
	case "CANCELED":
		return models.OrderStatusCanceled, nil
	case "REJECTED":
		return models.OrderStatusRejected, nil
	default:
		return "", fmt.Errorf("unknown order status: %s", s)
	}
}

func parseTimeInForce(s string) (models.TimeInForce, error) {
	switch strings.ToUpper(s) {
	case "GTC":
		return models.TimeInForceGTC, nil
	case "IOC":
		return models.TimeInForceIOC, nil
	case "FOK":
		return models.TimeInForceFOK, nil
	case "GTX":
		return models.TimeInForceGTX, nil
	default:
		return "", fmt.Errorf("unknown time in force: %s", s)
	}
}

// isFinal reports whether an order in the given state is terminal (will receive
// no further updates), so it should be removed from local tracking. A terminal
// status is final regardless of order type; a market IOC order is also final.
func isFinal(otype models.OrderType, tif models.TimeInForce, status models.OrderStatus) bool {
	switch status {
	case models.OrderStatusFilled, models.OrderStatusCanceled, models.OrderStatusRejected:
		return true
	}

	if otype == models.OrderTypeMarket && tif == models.TimeInForceIOC {
		return true
	}

	return false
}

// isOrderGone reports whether a non-zero cancel response indicates the order no
// longer exists / is already in a final state (so the caller should treat it as
// ErrOrderNotFound and drop local tracking). Code 7 is the known not-found code;
// the message/reason are also checked since the exact code set is not exhaustive.
func isOrderGone(code int, message, reason string) bool {
	if code == 7 {
		return true
	}

	// Keep markers specific to "the order is gone" to avoid mistaking an
	// unrelated message for a not-found and abandoning a live order.
	text := strings.ToUpper(message + " " + reason)
	for _, marker := range []string{
		"ORDER_NOT_FOUND",
		"NOT_FOUND",
		"DOES NOT EXIST",
		"ALREADY CANCEL", // ALREADY CANCELED / CANCELLED
		"ALREADY FILL",   // ALREADY FILLED
		"FINAL STATE",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}

	return false
}

// splitSymbol splits a "BASE-QUOTE" symbol. It returns empty strings rather than
// an error for an unexpected shape so a single odd symbol never drops an order;
// Base/Quote are informational and the Symbol field is always preserved.
func splitSymbol(symbol string) (base, quote string) {
	parts := strings.Split(symbol, "-")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}

	return "", ""
}
