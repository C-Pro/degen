package pintupro

import (
	"context"
	"fmt"
	"strings"

	"degen/pkg/models"
)

// easyjson:json
type placeOrderRequest struct {
	Symbol        string `json:"symbol"`
	Side          string `json:"side"`
	Type          string `json:"type"`
	Price         string `json:"price,omitempty"`
	Size          string `json:"size,omitempty"`
	Notional      string `json:"notional,omitempty"`
	ClientOrderID string `json:"client_order_id,omitempty"`
	TimeInForce   string `json:"time_in_force,omitempty"`
	ExecInst      string `json:"exec_inst,omitempty"`
}

// easyjson:json
type placeOrderResponse struct {
	OrderID       string `json:"order_id"`
	ClientOrderID string `json:"client_order_id"`
}

func (api *API) PlaceOrder(ctx context.Context, order models.Order) (*models.Order, error) {
	if order.TimeInForce == "" {
		order.TimeInForce = models.TimeInForceGTC
	}
	price := ""
	if order.Price.IsPositive() {
		price = order.Price.String()
	}

	size := ""
	if order.Size.IsPositive() {
		size = order.Size.String()
	}

	notional := ""
	if order.NotionalSize.IsPositive() {
		notional = order.NotionalSize.String()
	}
	orderRequest := placeOrderRequest{
		Symbol:        order.Symbol,
		Side:          strings.ToUpper(string(order.Side)),
		Type:          strings.ToUpper(string(order.Type)),
		Price:         price,
		Notional:      notional,
		Size:          size,
		ClientOrderID: order.ClientOrderID,
		TimeInForce:   strings.ToUpper(string(order.TimeInForce)),
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
	OrderID string `json:"order_id,omitempty"`
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

	if resp.Code == 7 {
		return nil, models.ErrOrderNotFound
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

func (api *API) CancelAllOrders(ctx context.Context, symbol string) error {
	req := cancelOrderRequest{
		Symbol: symbol,
	}

	resp := responseMessage{}
	if err := api.call("private/cancel-all-orders", &req, &resp); err != nil {
		return fmt.Errorf("pintupro.CancelAllOrders: %w", err)
	}

	if resp.Code != 0 {
		return fmt.Errorf("pintupro.CancelAllOrders: unexpected code %d %s %s",
			resp.Code, resp.Message, resp.Reason,
		)
	}

	return nil
}
