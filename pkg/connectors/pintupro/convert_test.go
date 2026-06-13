package pintupro

import (
	"encoding/json"
	"testing"

	"degen/pkg/models"
)

func TestParseSideCaseInsensitive(t *testing.T) {
	for _, s := range []string{"sell", "SELL", "Sell"} {
		if got := parseSide(s); got != models.OrderSideSell {
			t.Errorf("parseSide(%q) = %q, want sell", s, got)
		}
	}
	for _, s := range []string{"buy", "BUY", "Buy", "", "anything"} {
		if got := parseSide(s); got != models.OrderSideBuy {
			t.Errorf("parseSide(%q) = %q, want buy", s, got)
		}
	}
}

// Regression for C3: a lowercase "sell" user-trade must produce a NEGATIVE
// position amount (a reduce), not a positive one.
func TestHandleUserTrades_LowercaseSellIsNegative(t *testing.T) {
	p := &PintuPro{}
	ch := make(chan models.ExchangeMessage, 4)

	data := `{"trades":[{"trade_id":"t1","order_id":"o1","client_order_id":"c1",` +
		`"symbol":"WLD-IDR","side":"sell","price":"100","traded_size":"3","traded_at":1676869976772}]}`

	msg := wsMessage{Data: json.RawMessage(data)}
	if err := p.handleUserTrades(msg, ch); err != nil {
		t.Fatalf("handleUserTrades: %v", err)
	}

	select {
	case m := <-ch:
		pu, ok := m.Payload.(models.PositionUpdate)
		if !ok {
			t.Fatalf("payload type = %T, want PositionUpdate", m.Payload)
		}
		if !pu.Amount.IsNegative() {
			t.Errorf("sell trade amount = %s, want negative", pu.Amount)
		}
	default:
		t.Fatal("expected a position update message")
	}
}

func TestIsFinal(t *testing.T) {
	cases := []struct {
		otype  models.OrderType
		tif    models.TimeInForce
		status models.OrderStatus
		want   bool
	}{
		{models.OrderTypeLimit, models.TimeInForceGTC, models.OrderStatusPlaced, false},
		{models.OrderTypeLimit, models.TimeInForceGTC, models.OrderStatusPartiallyFilled, false},
		{models.OrderTypeLimit, models.TimeInForceGTC, models.OrderStatusFilled, true},
		{models.OrderTypeLimit, models.TimeInForceGTC, models.OrderStatusCanceled, true},
		{models.OrderTypeLimit, models.TimeInForceGTC, models.OrderStatusRejected, true},
		{models.OrderTypeMarket, models.TimeInForceIOC, models.OrderStatusPlaced, true},
	}
	for _, c := range cases {
		if got := isFinal(c.otype, c.tif, c.status); got != c.want {
			t.Errorf("isFinal(%s,%s,%s) = %v, want %v", c.otype, c.tif, c.status, got, c.want)
		}
	}
}

// Regression for L8: cancel responses that mean "order already gone" map to
// ErrOrderNotFound (not just the bare code 7), so callers can clean up.
func TestIsOrderGone(t *testing.T) {
	cases := []struct {
		code    int
		message string
		reason  string
		want    bool
	}{
		{7, "", "", true},
		{0, "", "", false},
		{12, "ORDER_NOT_FOUND", "", true},
		{12, "", "order already in final state", true},
		{12, "does not exist", "", true},
		{12, "RATE_LIMIT", "slow down", false},
	}
	for _, c := range cases {
		if got := isOrderGone(c.code, c.message, c.reason); got != c.want {
			t.Errorf("isOrderGone(%d,%q,%q) = %v, want %v", c.code, c.message, c.reason, got, c.want)
		}
	}
}

// Regression for H6: a REST order in a terminal status must come back with
// Final set (so account tracking deletes it instead of keeping a phantom),
// plus Base/Quote/PostOnly populated like the WS path.
func TestRestOrderToModel_SetsFinalAndFields(t *testing.T) {
	o := orderResponse{
		OrderID:       "o1",
		ClientOrderID: "c1",
		Symbol:        "WLD-IDR",
		Side:          "SELL",
		Type:          "LIMIT",
		TimeInForce:   "GTC",
		ExecInst:      "POST_ONLY",
		Status:        "FILLED",
	}
	got, err := restOrderToModel(o, models.Order{})
	if err != nil {
		t.Fatalf("restOrderToModel: %v", err)
	}
	if !got.Final {
		t.Errorf("Final = false, want true for a FILLED REST order")
	}
	if got.Side != models.OrderSideSell {
		t.Errorf("Side = %q, want sell", got.Side)
	}
	if !got.PostOnly {
		t.Errorf("PostOnly = false, want true for POST_ONLY")
	}
	if got.Base != "WLD" || got.Quote != "IDR" {
		t.Errorf("Base/Quote = %q/%q, want WLD/IDR", got.Base, got.Quote)
	}
}
