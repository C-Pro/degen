package dummy

import (
	"context"
	"errors"
	"time"

	"degen/pkg/models"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func (d *Dummy) PlaceOrder(ctx context.Context, order models.Order) (*models.Order, error) {
	order.UpdatedAt = time.Now()
	switch order.Text {
	case "error":
		return nil, errors.New("PlaceOrder error")
	case "reject":
		order.Status = models.OrderStatusRejected
		order.Final = true
		d.SetOrder(order)
		return &order, nil
	case "place":
		order.Status = models.OrderStatusPlaced
		order.ExchangeOrderID = uuid.NewString()
		d.SetOrder(order)
		return &order, nil
	case "fill":
		order.Status = models.OrderStatusFilled
		order.FilledSize = order.Size
		// TODO: use price from "exchange bbo"
		order.AveragePrice = decimal.RequireFromString("100.0")
		if !order.Price.IsZero() {
			order.AveragePrice = order.Price
		}
		order.Final = true
		d.SetOrder(order)
		return &order, nil
	case "partial":
		order.Status = models.OrderStatusPartiallyFilled
		order.FilledSize = order.Size.Div(decimal.NewFromInt(2))
		// TODO: use price from "exchange bbo"
		order.AveragePrice = decimal.RequireFromString("100.0")
		if !order.Price.IsZero() {
			order.AveragePrice = order.Price
		}
		if order.Type == models.OrderTypeMarket {
			order.Final = true
		}
		d.SetOrder(order)
		return &order, nil
	default:
		switch order.Type {
		case models.OrderTypeLimit:
			order.Status = models.OrderStatusPlaced
			order.ExchangeOrderID = uuid.NewString()
			d.SetOrder(order)
			return &order, nil
		case models.OrderTypeMarket:
			order.Status = models.OrderStatusFilled
			order.ExchangeOrderID = uuid.NewString()
			order.FilledSize = order.Size
			// TODO: use price from "exchange bbo"
			order.AveragePrice = decimal.RequireFromString("100.0")
			order.Final = true
			d.SetOrder(order)
			return &order, nil
		}
	}

	return nil, errors.New("unknown order type")
}

func (d *Dummy) CancelOrder(ctx context.Context, in models.Order) (*models.Order, error) {
	o, err := d.GetOrderDetails(ctx, in)
	if err != nil {
		return nil, models.ErrOrderNotFound
	}
	o.Status = models.OrderStatusCanceled
	o.Final = true
	d.SetOrder(*o)

	return o, nil
}

func (d *Dummy) CancelAllOrders(ctx context.Context, symbol string) error {
	orders, err := d.GetOpenOrders(ctx, symbol)
	if err != nil {
		return err
	}

	for _, o := range orders {
		_, err := d.CancelOrder(ctx, o)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *Dummy) GetOrderDetails(ctx context.Context, order models.Order) (*models.Order, error) {
	if order.ExchangeOrderID == "" {
		key := orderKey(order)
		o, err := d.orders.Get(key)
		if err != nil {
			return nil, models.ErrOrderNotFound
		}

		return &o, nil
	}

	if order.ClientOrderID != "" {
		exchangeOrderID, err := d.ordersByClientID.Get(order.ClientOrderID)
		if err != nil {
			return nil, models.ErrOrderNotFound
		}

		order.ExchangeOrderID = exchangeOrderID
		key := orderKey(order)
		o, err := d.orders.Get(key)
		if err != nil {
			return nil, err
		}

		return &o, nil
	}

	return nil, models.ErrOrderNotFound
}

func (d *Dummy) GetSymbols(ctx context.Context) (map[string]models.SymbolInfo, error) {
	return d.symbols.Snapshot(), nil
}

func (d *Dummy) GetOpenOrders(ctx context.Context, symbol string) ([]models.Order, error) {
	var orders []models.Order
	for _, o := range d.orders.Snapshot() {
		if o.Symbol == symbol {
			orders = append(orders, o)
		}
	}

	return orders, nil
}
