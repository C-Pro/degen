package account

import (
	"fmt"
	"maps"
	"slices"
	"sort"

	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

// openInterest holds information about currently open oders.
type openInterest struct {
	bids          map[string]models.Order
	asks          map[string]models.Order
	totalBidSize  decimal.Decimal
	totalBidPrice decimal.Decimal
	totalAskSize  decimal.Decimal
	totalAskPrice decimal.Decimal
}

func newOpenInterest() *openInterest {
	return &openInterest{
		bids: make(map[string]models.Order),
		asks: make(map[string]models.Order),
	}
}

// setFromOrders rebuilds the open interest from a full snapshot of open orders.
// It returns an error (rather than panicking) if a snapshot order is
// inconsistent, so a resync during a reconnect can degrade gracefully. [L2]
func (oi *openInterest) setFromOrders(orders []models.Order) error {
	oi.bids = make(map[string]models.Order)
	oi.asks = make(map[string]models.Order)
	oi.totalBidSize = decimal.Zero
	oi.totalAskSize = decimal.Zero
	oi.totalBidPrice = decimal.Zero
	oi.totalAskPrice = decimal.Zero

	for _, o := range orders {
		if err := oi.observe(o); err != nil {
			return fmt.Errorf("failed to set open interest from orders: %w", err)
		}
	}

	return nil
}

func (oi *openInterest) observe(o models.Order) error {
	switch o.Side {
	case models.OrderSideBuy:
		old, ok := oi.bids[o.ClientOrderID]
		oi.bids[o.ClientOrderID] = o
		switch o.Status {
		case models.OrderStatusPlaced:
			oi.totalBidSize = oi.totalBidSize.Add(o.Size)
			oi.totalBidPrice = oi.totalBidPrice.Add(o.Price.Mul(o.Size))
		case models.OrderStatusCanceled:
			if ok {
				cancelledSize := o.Size.Sub(o.FilledSize)
				oi.totalBidSize = oi.totalBidSize.Sub(cancelledSize)
				oi.totalBidPrice = oi.totalBidPrice.Sub(o.Price.Mul(cancelledSize))
			}
		case models.OrderStatusPartiallyFilled:
			if ok {
				filledSize := o.FilledSize.Sub(old.FilledSize)
				oi.totalBidSize = oi.totalBidSize.Sub(filledSize)
				oi.totalBidPrice = oi.totalBidPrice.Sub(o.Price.Mul(filledSize))
			} else {
				openSize := o.Size.Sub(o.FilledSize)
				oi.totalBidSize = oi.totalBidSize.Add(openSize)
				oi.totalBidPrice = oi.totalBidPrice.Add(o.Price.Mul(openSize))
			}
		case models.OrderStatusFilled:
			if ok {
				filledSize := o.FilledSize.Sub(old.FilledSize)
				oi.totalBidSize = oi.totalBidSize.Sub(filledSize)
				oi.totalBidPrice = oi.totalBidPrice.Sub(o.Price.Mul(filledSize))
			}
		}

		if o.Final {
			delete(oi.bids, o.ClientOrderID)
		}

		if oi.totalBidSize.IsNegative() {
			return fmt.Errorf("negative total bid size after observing order %s", o.ClientOrderID)
		}

	case models.OrderSideSell:
		old, ok := oi.asks[o.ClientOrderID]
		oi.asks[o.ClientOrderID] = o
		switch o.Status {
		case models.OrderStatusPlaced:
			oi.totalAskSize = oi.totalAskSize.Add(o.Size)
			oi.totalAskPrice = oi.totalAskPrice.Add(o.Price.Mul(o.Size))
		case models.OrderStatusCanceled:
			if ok {
				cancelledSize := o.Size.Sub(o.FilledSize)
				oi.totalAskSize = oi.totalAskSize.Sub(cancelledSize)
				oi.totalAskPrice = oi.totalAskPrice.Sub(o.Price.Mul(cancelledSize))
			}
		case models.OrderStatusPartiallyFilled:
			if ok {
				filledSize := o.FilledSize.Sub(old.FilledSize)
				oi.totalAskSize = oi.totalAskSize.Sub(filledSize)
				oi.totalAskPrice = oi.totalAskPrice.Sub(o.Price.Mul(filledSize))
			} else {
				openSize := o.Size.Sub(o.FilledSize)
				oi.totalAskSize = oi.totalAskSize.Add(openSize)
				oi.totalAskPrice = oi.totalAskPrice.Add(o.Price.Mul(openSize))
			}
		case models.OrderStatusFilled:
			if ok {
				filledSize := o.FilledSize.Sub(old.FilledSize)
				oi.totalAskSize = oi.totalAskSize.Sub(filledSize)
				oi.totalAskPrice = oi.totalAskPrice.Sub(o.Price.Mul(filledSize))
			}
		}

		if o.Final {
			delete(oi.asks, o.ClientOrderID)
		}

		if oi.totalAskSize.IsNegative() {
			return fmt.Errorf("negative total ask size after observing order %s", o.ClientOrderID)
		}
	}

	return nil
}

func (oi *openInterest) GetBids() []models.Order {
	orders := slices.Collect(maps.Values(oi.bids))
	sort.Slice(orders, func(i, j int) bool {
		if orders[i].Price.Equal(orders[j].Price) {
			return orders[i].CreatedAt.Before(orders[j].CreatedAt)
		}
		return orders[i].Price.GreaterThan(orders[j].Price)
	})

	return orders
}

func (oi *openInterest) GetAsks() []models.Order {
	orders := slices.Collect(maps.Values(oi.asks))
	sort.Slice(orders, func(i, j int) bool {
		if orders[i].Price.Equal(orders[j].Price) {
			return orders[i].CreatedAt.Before(orders[j].CreatedAt)
		}
		return orders[i].Price.LessThan(orders[j].Price)
	})

	return orders
}

func (oi *openInterest) GetTotalBidSize() decimal.Decimal {
	return oi.totalBidSize
}

func (oi *openInterest) GetTotalAskSize() decimal.Decimal {
	return oi.totalAskSize
}

func (oi *openInterest) GetAvgBidPrice() decimal.Decimal {
	if oi.totalBidSize.IsZero() {
		return decimal.Zero
	}

	return oi.totalBidPrice.Div(oi.totalBidSize)
}

func (oi *openInterest) GetAvgAskPrice() decimal.Decimal {
	if oi.totalAskSize.IsZero() {
		return decimal.Zero
	}

	return oi.totalAskPrice.Div(oi.totalAskSize)
}
