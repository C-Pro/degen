package account

import (
	"testing"

	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

func TestSpreadPenalty(t *testing.T) {
	eq := func(a, b decimal.Decimal) bool {
		return a.Sub(b).Abs().LessThan(decimal.NewFromFloat(0.0001))
	}
	cases := []struct {
		name          string
		orders        []models.Order
		expBidPenalty decimal.Decimal
		expAskPenalty decimal.Decimal
	}{
		{
			name:          "empty orders",
			orders:        []models.Order{},
			expBidPenalty: decimal.Zero,
			expAskPenalty: decimal.Zero,
		},
		{
			name: "1% bid heavy imbalance",
			orders: []models.Order{
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(101),
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusPlaced,
					ClientOrderID: "1",
				},
				{
					Price:         decimal.NewFromFloat(101),
					Size:          decimal.NewFromFloat(100),
					Side:          models.OrderSideSell,
					Status:        models.OrderStatusPlaced,
					ClientOrderID: "2",
				},
			},
			expBidPenalty: decimal.NewFromFloat(0.0005),
			expAskPenalty: decimal.Zero,
		},
		{
			name: "1% ask heavy imbalance",
			orders: []models.Order{
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(100),
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusPlaced,
					ClientOrderID: "1",
				},
				{
					Price:         decimal.NewFromFloat(101),
					Size:          decimal.NewFromFloat(101),
					Side:          models.OrderSideSell,
					Status:        models.OrderStatusPlaced,
					ClientOrderID: "2",
				},
			},
			expBidPenalty: decimal.Zero,
			expAskPenalty: decimal.NewFromFloat(0.0005),
		},
		{
			name: "balanced orders",
			orders: []models.Order{
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(1),
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusPlaced,
					ClientOrderID: "1",
				},
				{
					Price:         decimal.NewFromFloat(101),
					Size:          decimal.NewFromFloat(1),
					Side:          models.OrderSideSell,
					Status:        models.OrderStatusPlaced,
					ClientOrderID: "2",
				},
			},
			expBidPenalty: decimal.Zero,
			expAskPenalty: decimal.Zero,
		},
		{
			name: "2% bid heavy imbalance",
			orders: []models.Order{
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(102),
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusPlaced,
					ClientOrderID: "1",
				},
				{
					Price:         decimal.NewFromFloat(101),
					Size:          decimal.NewFromFloat(100),
					Side:          models.OrderSideSell,
					Status:        models.OrderStatusPlaced,
					ClientOrderID: "2",
				},
			},
			expBidPenalty: decimal.NewFromFloat(0.001),
			expAskPenalty: decimal.Zero,
		},
		{
			name: "empty asks",
			orders: []models.Order{
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(3),
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusPlaced,
					ClientOrderID: "1",
				},
			},
			expBidPenalty: decimal.NewFromFloat(0.05),
			expAskPenalty: decimal.Zero,
		},
		{
			name: "empty bids",
			orders: []models.Order{
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(1),
					Side:          models.OrderSideSell,
					Status:        models.OrderStatusPlaced,
					ClientOrderID: "1",
				},
			},
			expBidPenalty: decimal.Zero,
			expAskPenalty: decimal.NewFromFloat(0.05),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// acc := account.NewAccount()
			oi := newOpenInterest()

			for _, order := range tc.orders {
				if err := oi.observe(order); err != nil {
					t.Errorf("error observing order: %v", err)
				}
			}

			if !eq(oi.bidSpreadPenalty(), tc.expBidPenalty) {
				t.Errorf("expected bidPenalty %s, got %s", tc.expBidPenalty, oi.bidSpreadPenalty())
			}

			if !eq(oi.askSpreadPenalty(), tc.expAskPenalty) {
				t.Errorf("expected askPenalty %s, got %s", tc.expAskPenalty, oi.askSpreadPenalty())
			}
		})
	}
}
