package pintupro

import (
	"context"

	"degen/pkg/models"
)

func (api *API) PlaceOrder(ctx context.Context, order models.Order) (*models.Order, error) {
	return &order, nil
}

func (api *API) CancelOrder(ctx context.Context, order models.Order) (*models.Order, error) {
	return &order, nil
}
