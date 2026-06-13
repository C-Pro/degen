package pintupro

import (
	"context"
	"fmt"
	"log"
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
	if err := api.call(ctx, "private/place-order", &orderRequest, &resp); err != nil {
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
	if err := api.call(ctx, "private/cancel-order", &orderRequest, &resp); err != nil {
		return nil, fmt.Errorf("pintupro.CancelOrder: %w", err)
	}

	// Map any "order is already gone" response to ErrOrderNotFound so callers
	// can clean up local state, instead of only recognizing code 7 and leaving
	// a phantom order to be re-cancelled every tick. [L8]
	if resp.Code != 0 && isOrderGone(resp.Code, resp.Message, resp.Reason) {
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
	if err := api.call(ctx, "private/cancel-all-orders", &req, &resp); err != nil {
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

// restOrderToModel converts a REST order payload into a models.Order using the
// same validated, case-insensitive parsing and Final computation as the
// WebSocket path (see convert.go), so the two paths cannot diverge. Fields not
// present in the payload are taken from base (used by GetOrderDetails to
// preserve e.g. PlacedAt).
func restOrderToModel(o orderResponse, base models.Order) (models.Order, error) {
	otype, err := parseType(o.Type)
	if err != nil {
		return models.Order{}, err
	}

	status, err := parseStatus(o.Status)
	if err != nil {
		return models.Order{}, err
	}

	tifStr := o.TimeInForce
	if tifStr == "" {
		// Resting orders should always carry a TIF; default a missing one to GTC
		// rather than dropping the order.
		tifStr = string(models.TimeInForceGTC)
	}
	tif, err := parseTimeInForce(tifStr)
	if err != nil {
		return models.Order{}, err
	}

	baseAsset, quoteAsset := splitSymbol(o.Symbol)

	base.ExchangeOrderID = o.OrderID
	base.ClientOrderID = o.ClientOrderID
	base.Symbol = o.Symbol
	base.Base = baseAsset
	base.Quote = quoteAsset
	base.Side = parseSide(o.Side)
	base.Type = otype
	base.Status = status
	base.TimeInForce = tif
	base.Price = o.Price
	base.Size = o.Size
	base.NotionalSize = o.CumValue
	base.FilledSize = o.CumSize
	base.AveragePrice = o.CumPrice
	base.PostOnly = o.ExecInst == "POST_ONLY"
	base.Final = isFinal(otype, tif, status)
	base.CreatedAt = tsToTime(o.CreatedAt)
	base.UpdatedAt = tsToTime(o.UpdatedAt)

	return base, nil
}

func (api *API) GetOpenOrders(ctx context.Context, symbol string) ([]models.Order, error) {
	req := getOpenOrdersRequest{
		Symbol: symbol,
	}

	resp := getOpenOrdersResponse{}
	if err := api.call(ctx, "private/get-open-orders", &req, &resp); err != nil {
		return nil, fmt.Errorf("pintupro.GetOpenOrders: %w", err)
	}

	if len(resp.Orders) == 0 {
		return nil, nil
	}

	orders := make([]models.Order, 0, len(resp.Orders))
	for _, o := range resp.Orders {
		order, err := restOrderToModel(o, models.Order{})
		if err != nil {
			// Skip the odd order rather than failing the whole snapshot.
			log.Printf("pintupro.GetOpenOrders: skipping order %s: %v", o.OrderID, err)
			continue
		}
		orders = append(orders, order)
	}

	return orders, nil
}

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
		Symbol: order.Symbol,
	}

	if order.ExchangeOrderID != "" {
		req.OrderID = order.ExchangeOrderID
	} else {
		req.ClientOrderID = order.ClientOrderID
	}

	resp := getOrderDetailsResponse{}
	if err := api.call(ctx, "private/get-order-details", &req, &resp); err != nil {
		return nil, fmt.Errorf("pintupro.GetOrderDetails: %w", err)
	}

	out, err := restOrderToModel(resp.Order, order)
	if err != nil {
		return nil, fmt.Errorf("pintupro.GetOrderDetails: %w", err)
	}

	return &out, nil
}
