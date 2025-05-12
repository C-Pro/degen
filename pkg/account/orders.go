package account

import (
	"fmt"

	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

// openInterest holds information about currently open oders.
type openInterest struct {
	bids         map[string]models.Order
	asks         map[string]models.Order
	totalBidSize decimal.Decimal
	totalAskSize decimal.Decimal
}

func newOpenInterest() *openInterest {
	return &openInterest{
		bids: make(map[string]models.Order),
		asks: make(map[string]models.Order),
	}
}

func (oi *openInterest) setFromOrders(orders []models.Order) {
	oi.bids = make(map[string]models.Order)
	oi.asks = make(map[string]models.Order)
	oi.totalBidSize = decimal.Zero
	oi.totalAskSize = decimal.Zero

	for _, o := range orders {
		if err := oi.observe(o); err != nil {
			panic(fmt.Sprintf("failed to set open interest from orders: %v\n %#v", err, orders))
		}
	}
}

func (oi *openInterest) observe(o models.Order) error {
	switch o.Side {
	case models.OrderSideBuy:
		old, ok := oi.bids[o.ClientOrderID]
		oi.bids[o.ClientOrderID] = o
		switch o.Status {
		case models.OrderStatusPlaced:
			oi.totalBidSize = oi.totalBidSize.Add(o.Size)
		case models.OrderStatusCanceled:
			cancelledSize := o.Size.Sub(o.FilledSize)
			oi.totalBidSize = oi.totalBidSize.Sub(cancelledSize)
		case models.OrderStatusPartiallyFilled, models.OrderStatusFilled:
			if ok {
				filledSize := o.FilledSize.Sub(old.FilledSize)
				oi.totalBidSize = oi.totalBidSize.Sub(filledSize)
			} else {
				openSize := o.Size.Sub(o.FilledSize)
				oi.totalBidSize = oi.totalBidSize.Add(openSize)
			}
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
		case models.OrderStatusCanceled:
			cancelledSize := o.Size.Sub(o.FilledSize)
			oi.totalAskSize = oi.totalAskSize.Sub(cancelledSize)
		case models.OrderStatusPartiallyFilled, models.OrderStatusFilled:
			if ok {
				filledSize := o.FilledSize.Sub(old.FilledSize)
				oi.totalAskSize = oi.totalAskSize.Sub(filledSize)
			} else {
				openSize := o.Size.Sub(o.FilledSize)
				oi.totalAskSize = oi.totalAskSize.Add(openSize)
			}
		}

		if oi.totalAskSize.IsNegative() {
			return fmt.Errorf("negative total ask size after observing order %s", o.ClientOrderID)
		}
	}

	return nil
}
