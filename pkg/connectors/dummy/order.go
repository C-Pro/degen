package dummy

import (
	"context"

	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

func (api *API) PlaceOrder(ctx context.Context, order models.Order) (*models.Order, error) {
	return &order, nil
}

func (api *API) CancelOrder(ctx context.Context, order models.Order) (*models.Order, error) {
	return &order, nil
}

func (api *API) CancelAllOrders(ctx context.Context, symbol string) error {
	return nil
}

func (api *API) GetOrderDetails(ctx context.Context, order models.Order) (*models.Order, error) {
	return &order, nil
}

func (a *API) GetSymbols(ctx context.Context) (map[string]models.SymbolInfo, error) {
	return map[string]models.SymbolInfo{
		"BTCUSD": {
			Symbol:           "BTCUSD",
			Base:             "BTC",
			Quote:            "USD",
			PriceTickSize:    decimal.RequireFromString("0.01"),
			QuantityTickSize: decimal.RequireFromString("0.001"),
		},
	}, nil
}
