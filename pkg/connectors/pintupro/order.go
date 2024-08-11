package pintupro

import (
	"context"
	"fmt"
	"strings"

	"degen/pkg/models"

	"github.com/shopspring/decimal"
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

// easyjson:json
type getOpenOrdersRequest struct {
	Symbol   string `json:"symbol"`
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
	Side     string `json:"side,omitempty"`
}

/*
{
  "request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
  "timestamp": 16775774798300,
  "method": "private/get-open-orders",
  "code": 0,
  "message": "SUCCESS",
  "data": {
    "count": 345,
    "orders": [
      {
        "status": "PARTIALLY_FILLED",
        "symbol": "BTC-IDR",
        "type": "LIMIT",
        "time_in_force": "GTC",
        "exec_inst": "POST_ONLY",
        "side": "BUY",
        "price": "15000",
        "size": "2",
        "cum_price": "15000",
        "cum_size": "0.35",
        "cum_value": "5250",
        "order_id": "12345678-abcd-efgh-ijkl-1234567890ab",
        "client_order_id": "my-order-id-1",
        "created_at": 16775774795300,
        "updated_at": 16775774797678
      },
      ...
    ]
  }
}
*/

// easyjson:json
type orderResponse struct {
	Status        string          `json:"status"`
	Symbol        string          `json:"symbol"`
	Type          string          `json:"type"`
	TimeInForce   string          `json:"time_in_force"`
	ExecInst      string          `json:"exec_inst"`
	Side          string          `json:"side"`
	Price         decimal.Decimal `json:"price"`
	Size          decimal.Decimal `json:"size"`
	CumPrice      decimal.Decimal `json:"cum_price"`
	CumSize       decimal.Decimal `json:"cum_size"`
	CumValue      decimal.Decimal `json:"cum_value"`
	OrderID       string          `json:"order_id"`
	ClientOrderID string          `json:"client_order_id"`
	CreatedAt     int64           `json:"created_at"`
	UpdatedAt     int64           `json:"updated_at"`
}

// easyjson:json
type getOpenOrdersResponse struct {
	Count  int             `json:"count"`
	Orders []orderResponse `json:"orders"`
}

func (api *API) GetOpenOrders(ctx context.Context, symbol string) ([]models.Order, error) {
	req := getOpenOrdersRequest{
		Symbol: symbol,
	}

	resp := getOpenOrdersResponse{}
	if err := api.call("private/get-open-orders", &req, &resp); err != nil {
		return nil, fmt.Errorf("pintupro.GetOpenOrders: %w", err)
	}

	if len(resp.Orders) == 0 {
		return nil, nil
	}

	orders := make([]models.Order, 0, len(resp.Orders))
	for _, o := range resp.Orders {
		order := models.Order{
			ExchangeOrderID: o.OrderID,
			ClientOrderID:   o.ClientOrderID,
			Symbol:          o.Symbol,
			Side:            models.OrderSide(strings.ToLower(o.Side)),
			Type:            models.OrderType(strings.ToLower(o.Type)),
			Price:           o.Price,
			Size:            o.Size,
			NotionalSize:    o.CumValue,
			FilledSize:      o.CumSize,
			AveragePrice:    o.CumPrice,

			TimeInForce: models.TimeInForce(o.TimeInForce),
			Status:      models.OrderStatus(strings.ToLower(o.Status)),
			UpdatedAt:   tsToTime(o.UpdatedAt),
			CreatedAt:   tsToTime(o.CreatedAt),
		}
		orders = append(orders, order)
	}

	return orders, nil
}

/*
{
"request_id": "a88b9054-bde2-4fd7-8a4e-c6ff6de212e2",
"timestamp": 16775774797462,
"api_key": "my-key",
"signature": "my-signature",
"method": "private/get-order-details",
"params": {
"symbol": "BTC-IDR",
"order_id": "aaa-bbb-ccc",
"t_start": 1676869976772,
"t_end": 1677869976772,
}
}
*/

// easyjson:json
type getOrderDetailsRequest struct {
	Symbol        string `json:"symbol"`
	OrderID       string `json:"order_id,omitempty"`
	ClientOrderID string `json:"client_order_id,omitempty"`
}

// easyjson:json
type getOrderDetailsResponse struct {
	Order orderResponse `json:"order_info"`
	// TODO: Trades
}

func (api *API) GetOrderDetails(ctx context.Context, order models.Order) (*models.Order, error) {
	req := getOrderDetailsRequest{
		Symbol:  order.Symbol,
	}

	if order.ExchangeOrderID != "" {
		req.OrderID = order.ExchangeOrderID
	} else {
		req.ClientOrderID = order.ClientOrderID
	}

	resp := getOrderDetailsResponse{}
	if err := api.call("private/get-order-details", &req, &resp); err != nil {
		return nil, fmt.Errorf("pintupro.GetOrderDetails: %w", err)
	}

	o := resp.Order
	order.ExchangeOrderID = o.OrderID
	order.ClientOrderID = o.ClientOrderID
	order.Symbol = o.Symbol
	order.Side = models.OrderSide(strings.ToLower(o.Side))
	order.Type = models.OrderType(strings.ToLower(o.Type))
	order.Price = o.Price
	order.Size = o.Size
	order.NotionalSize = o.CumValue
	order.FilledSize = o.CumSize
	order.AveragePrice = o.CumPrice

	order.TimeInForce = models.TimeInForce(o.TimeInForce)
	order.Status = models.OrderStatus(strings.ToLower(o.Status))
	order.UpdatedAt = tsToTime(o.UpdatedAt)
	order.CreatedAt = tsToTime(o.CreatedAt)

	return &order, nil
}
