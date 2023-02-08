package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

type placeOrderReq struct {
	ClientOrderID string
	Symbol        string
	Side          string
	Type          string
	Quantity      string
	Price         string
	ReduceOnly    string
	StopPrice     string
	ClosePosition string
	TimeInForce   string
}

func newPlaceOrderReq(order models.Order) (*placeOrderReq, error) {
	req := &placeOrderReq{
		ClientOrderID: order.ClientOrderID,
		Symbol:        symbolToExchange(order.Symbol),
		Side:          strings.ToUpper(string(order.Side)),
		Type:          typeToEx(order.Type),
		TimeInForce:   string(order.TimeInForce),
	}

	if order.Size.IsPositive() {
		// order.Size is expected to be rounded to exchange contract spec
		req.Quantity = order.Size.String()
	}

	if order.Price.IsPositive() {
		// order.Price is expected to be rounded to exchange contract spec
		req.Price = order.Price.String()
	}

	return req, nil
}

func (r *placeOrderReq) Values() url.Values {
	values := url.Values{}
	values.Add("newClientOrderId", r.ClientOrderID)
	values.Add("symbol", r.Symbol)
	values.Add("side", r.Side)
	values.Add("type", r.Type)
	if r.Quantity != "" {
		values.Add("quantity", r.Quantity)
	}
	if r.Price != "" {
		values.Add("price", r.Price)
	}
	if r.TimeInForce != "" {
		values.Add("timeInForce", r.TimeInForce)
	}

	return values
}

//easyjson:json
type placeOrderResp struct {
	ClientOrderID   string `json:"newClientOrderId"`
	Symbol          string `json:"symbol"`
	Side            string `json:"side"`
	Type            string `json:"type"`
	Quantity        string `json:"origQty"`
	Price           string `json:"price"`
	ReduceOnly      bool   `json:"reduceOnly"`
	StopPrice       string `json:"stopPrice"`
	Status          string `json:"status"`
	ExchangeOrderID int64  `json:"orderId"`
	FilledQty       string `json:"executedQty"`
	FilledPrice     string `json:"avgPrice"`
	TimeInForce     string `json:"timeInForce"`
	UpdateTime      int64  `json:"updateTime"`
}

func (api *API) PlaceOrder(ctx context.Context, order models.Order) (*models.Order, error) {
	reqOrder, _ := newPlaceOrderReq(order)
	query := reqOrder.Values().Encode()
	path := "/fapi/v1/order"
	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		api.baseURL+path,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("binance.PlaceOrder failed to create request: %w", err)
	}

	req.URL.RawQuery = api.signRequest(query, "", order.CreatedAt)
	req.Header.Add("X-MBX-APIKEY", api.key)

	resp, err := api.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("binance.PlaceOrder failed perform request: %w", err)
	}

	defer resp.Body.Close()
	var respData placeOrderResp

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("binance.PlaceOrder failed to read response: %w", err)
	}

	log.Printf("RESP: \n%s\n", string(b))

	if err := json.Unmarshal(b, &respData); err != nil {
		return nil, fmt.Errorf("binance.PlaceOrder failed to unmarshal response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("binance.PlaceOrder returned code %d: %s", resp.StatusCode, string(b))
	}

	order.ExchangeOrderID = strconv.FormatInt(respData.ExchangeOrderID, 10)
	order.Status = orderStatusFromExchange(respData.Status)
	order.UpdatedAt = time.Unix(0, respData.UpdateTime*1000*1000).UTC()

	if respData.FilledQty != "" {
		order.FilledSize, err = decimal.NewFromString(respData.FilledQty)
		if err != nil {
			return nil, fmt.Errorf("binance.PlaceOrder failed to parse executedQty(%q): %w", respData.FilledQty, err)
		}

		order.AveragePrice, err = decimal.NewFromString(respData.FilledPrice)
		if err != nil {
			return nil, fmt.Errorf("binance.PlaceOrder failed to parse avgPrice(%q): %w", respData.FilledPrice, err)
		}
	}

	return &order, nil
}

func (api *API) CancelOrder(ctx context.Context, order models.Order) (*models.Order, error) {
	query := fmt.Sprintf("symbol=%s&orderId=%s",
		symbolToExchange(order.Symbol),
		order.ExchangeOrderID,
	)
	path := "/fapi/v1/order"
	req, err := http.NewRequestWithContext(
		ctx,
		"DELETE",
		api.baseURL+path,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("binance.CancelOrder failed to create request: %w", err)
	}

	req.URL.RawQuery = api.signRequest(query, "", order.CreatedAt)
	req.Header.Add("X-MBX-APIKEY", api.key)

	resp, err := api.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("binance.CancelOrder failed perform request: %w", err)
	}

	defer resp.Body.Close()
	var respData placeOrderResp

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("binance.CancelOrder failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("binance.CancelOrder returned code %d: %s", resp.StatusCode, string(b))
	}

	if err := json.Unmarshal(b, &respData); err != nil {
		return nil, fmt.Errorf("binance.CancelOrder failed to unmarshal response: %w", err)
	}

	order.Status = orderStatusFromExchange(respData.Status)
	order.UpdatedAt = time.Unix(0, respData.UpdateTime*1000*1000).UTC()

	if respData.FilledQty != "" {
		order.FilledSize, err = decimal.NewFromString(respData.FilledQty)
		if err != nil {
			return nil, fmt.Errorf("binance.PlaceOrder failed to parse executedQty(%q): %w", respData.FilledQty, err)
		}

		order.AveragePrice, err = decimal.NewFromString(respData.FilledPrice)
		if err != nil {
			return nil, fmt.Errorf("binance.PlaceOrder failed to parse avgPrice(%q): %w", respData.FilledPrice, err)
		}
	}

	return &order, nil
}
