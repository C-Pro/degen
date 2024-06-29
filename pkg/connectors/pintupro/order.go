package pintupro

import (
	"context"
	"fmt"

	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

// easyjson:json
type placeOrderRequest struct {
	Symbol        string          `json:"symbol"`
	Side          string          `json:"side"`
	Type          string          `json:"type"`
	Price         decimal.Decimal `json:"price,omitempty"`
	Size          decimal.Decimal `json:"size,omitempty"`
	Notional      decimal.Decimal `json:"notional,omitempty"`
	ClientOrderID string          `json:"client_order_id,omitempty"`
	TimeInForce   string          `json:"time_in_force,omitempty"`
	ExecInst      string          `json:"exec_inst,omitempty"`
}

// easyjson:json
type placeOrderResponse struct {
	OrderID       string `json:"order_id"`
	ClientOrderID string `json:"client_order_id"`
}

func (api *API) PlaceOrder(ctx context.Context, order models.Order) (*models.Order, error) {
	orderRequest := placeOrderRequest{
		Symbol:        order.Symbol,
		Side:          string(order.Side),
		Type:          string(order.Type),
		Price:         order.Price,
		Notional:      order.NotionalSize,
		Size:          order.Size,
		ClientOrderID: order.ClientOrderID,
		TimeInForce:   string(order.TimeInForce),
	}

	if order.PostOnly {
		orderRequest.ExecInst = "POST_ONLY"
	}

	var data placeOrderResponse
	resp := responseMessage{
		Data: &data,
	}
	if err := api.call("private/place-order", &orderRequest, &resp); err != nil {
		return nil, fmt.Errorf("pintupro.PlaceOrder: %w", err)
	}

	if resp.Code != 0 {
		return nil,
			fmt.Errorf("pintupro.PlaceOrder: unexpected code %d %s %s",
				resp.Code, resp.Message, resp.Reason,
			)
	}

	order.ExchangeOrderID = data.OrderID
	order.Status = models.OrderStatusNew

	return &order, nil
}

// easyjson:json
type cancelOrderRequest struct {
	Symbol  string `json:"symbol"`
	OrderID string `json:"order_id"`
}

func (api *API) CancelOrder(ctx context.Context, order models.Order) (*models.Order, error) {
	orderRequest := cancelOrderRequest{
		Symbol:  order.Symbol,
		OrderID: order.ExchangeOrderID,
	}

	resp := responseMessage{}
	if err := api.call("private/cancel-order", &orderRequest, &resp); err != nil {
		return nil, fmt.Errorf("pintupro.CancelOrder: %w", err)
	}

	if resp.Code != 0 {
		return nil,
			fmt.Errorf("pintupro.CancelOrder: unexpected code %d %s %s",
				resp.Code, resp.Message, resp.Reason,
			)
	}

	// We don't set order status right away, becase it may still be filled in the exchange.
	// Need to wait for next update on websocket.

	return &order, nil
}
