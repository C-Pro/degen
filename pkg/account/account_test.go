package account

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/shopspring/decimal"

	"degen/pkg/connectors/dummy"
	"degen/pkg/models"
)

func TestUpdatePosition(t *testing.T) {
	ts := time.Now()
	cases := []struct {
		name      string
		postition models.Position
		symbol    string
		amount    decimal.Decimal
		price     decimal.Decimal
		expected  models.Position
	}{
		{
			name:   "new position",
			symbol: "BTCUSD",
			amount: decimal.NewFromFloat(1),
			price:  decimal.NewFromFloat(10000),
			expected: models.Position{
				Amount:       decimal.NewFromFloat(1),
				AveragePrice: decimal.NewFromFloat(10000),
				UpdatedAt:    ts,
			},
		},
		{
			name: "add to long position",
			postition: models.Position{
				Amount:       decimal.NewFromFloat(1),
				AveragePrice: decimal.NewFromFloat(10000),
				UpdatedAt:    ts,
			},
			symbol: "BTCUSD",
			amount: decimal.NewFromFloat(1),
			price:  decimal.NewFromFloat(11000),
			expected: models.Position{
				Amount:       decimal.NewFromFloat(2),
				AveragePrice: decimal.NewFromFloat(10500),
				UpdatedAt:    ts,
			},
		},
		{
			name: "reduce long position",
			postition: models.Position{
				Amount:       decimal.NewFromFloat(2),
				AveragePrice: decimal.NewFromFloat(10000),
				UpdatedAt:    ts,
			},
			symbol: "BTCUSD",
			amount: decimal.NewFromFloat(-1),
			price:  decimal.NewFromFloat(11000),
			expected: models.Position{
				Amount:       decimal.NewFromFloat(1),
				AveragePrice: decimal.NewFromFloat(10000),
				UpdatedAt:    ts,
				RealizedPnL:  decimal.NewFromFloat(1000),
			},
		},
		{
			name: "reduce long position to zero",
			postition: models.Position{
				Amount:       decimal.NewFromFloat(1),
				AveragePrice: decimal.NewFromFloat(10000),
				UpdatedAt:    ts,
				RealizedPnL:  decimal.NewFromFloat(1000),
			},
			symbol: "BTCUSD",
			amount: decimal.NewFromFloat(-1),
			price:  decimal.NewFromFloat(11000),
			expected: models.Position{
				Amount:       decimal.Zero,
				AveragePrice: decimal.Zero,
				UpdatedAt:    ts,
				RealizedPnL:  decimal.NewFromFloat(2000),
			},
		},
		{
			name: "big trade changing long position to opposite",
			postition: models.Position{
				Amount:       decimal.NewFromFloat(1),
				AveragePrice: decimal.NewFromFloat(10000),
				UpdatedAt:    ts,
			},
			symbol: "BTCUSD",
			amount: decimal.NewFromFloat(-2),
			price:  decimal.NewFromFloat(9000),
			expected: models.Position{
				Amount:       decimal.NewFromFloat(-1),
				AveragePrice: decimal.NewFromFloat(9000),
				UpdatedAt:    ts,
				RealizedPnL:  decimal.NewFromFloat(-1000),
			},
		},
		{
			name: "big trade changing short position to opposite",
			postition: models.Position{
				Amount:       decimal.NewFromFloat(-1),
				AveragePrice: decimal.NewFromFloat(10000),
				UpdatedAt:    ts,
			},
			symbol: "BTCUSD",
			amount: decimal.NewFromFloat(2),
			price:  decimal.NewFromFloat(11000),
			expected: models.Position{
				Amount:       decimal.NewFromFloat(1),
				AveragePrice: decimal.NewFromFloat(11000),
				UpdatedAt:    ts,
				RealizedPnL:  decimal.NewFromFloat(-1000),
			},
		},
		{
			name: "add to short position",
			postition: models.Position{
				Amount:       decimal.NewFromFloat(-1),
				AveragePrice: decimal.NewFromFloat(10000),
				UpdatedAt:    ts,
			},
			symbol: "BTCUSD",
			amount: decimal.NewFromFloat(-1),
			price:  decimal.NewFromFloat(9000),
			expected: models.Position{
				Amount:       decimal.NewFromFloat(-2),
				AveragePrice: decimal.NewFromFloat(9500),
				UpdatedAt:    ts,
			},
		},
		{
			name: "reduce short position",
			postition: models.Position{
				Amount:       decimal.NewFromFloat(-2),
				AveragePrice: decimal.NewFromFloat(10000),
				UpdatedAt:    ts,
			},
			symbol: "BTCUSD",
			amount: decimal.NewFromFloat(1),
			price:  decimal.NewFromFloat(9000),
			expected: models.Position{
				Amount:       decimal.NewFromFloat(-1),
				AveragePrice: decimal.NewFromFloat(10000),
				UpdatedAt:    ts,
				RealizedPnL:  decimal.NewFromFloat(1000),
			},
		},
		{
			name: "reduce short position to zero",
			postition: models.Position{
				Amount:       decimal.NewFromFloat(-1),
				AveragePrice: decimal.NewFromFloat(10000),
				UpdatedAt:    ts,
				RealizedPnL:  decimal.NewFromFloat(1000),
			},
			symbol: "BTCUSD",
			amount: decimal.NewFromFloat(1),
			price:  decimal.NewFromFloat(9000),
			expected: models.Position{
				Amount:       decimal.Zero,
				AveragePrice: decimal.Zero,
				UpdatedAt:    ts,
				RealizedPnL:  decimal.NewFromFloat(2000),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "reduce long position to zero" {
				t.Log("here")
			}

			a, err := NewAccount("test", dummy.NewDummy(
				context.Background(),
				"key", "secret", "https://test.com", "wss://test.com/ws",
			))
			if err != nil {
				t.Fatalf("unexpected error in NewAccount: %v", err)
			}
			upd := models.PositionUpdate{
				Amount:    tc.postition.Amount,
				Price:     tc.postition.AveragePrice,
				Timestamp: tc.postition.UpdatedAt,
			}
			ps := &positionStructure{}
			ps.Update(upd)
			ps.realizedPnL = tc.postition.RealizedPnL

			a.positions[tc.symbol] = ps
			a.UpdatePosition(tc.symbol, tc.amount, tc.price, ts)
			if diff := cmp.Diff(tc.expected, a.positions[tc.symbol].Position()); diff != "" {
				t.Errorf("UpdatePosition() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}


func TestOpenInterest(t *testing.T) {
	cases := []struct {
		name     string
		orders   []models.Order
		expBidOI decimal.Decimal
		expAskOI decimal.Decimal
	}{
		{
			name:     "empty orders",
			orders:   []models.Order{},
			expBidOI: decimal.Zero,
			expAskOI: decimal.Zero,
		},
		{
			name: "one bid order",
			orders: []models.Order{
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(1),
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusPlaced,
					ClientOrderID: "1",
				},
			},
			expBidOI: decimal.NewFromFloat(1),
			expAskOI: decimal.Zero,
		},
		{
			name: "one ask order",
			orders: []models.Order{
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(1),
					Side:          models.OrderSideSell,
					Status:        models.OrderStatusPlaced,
					ClientOrderID: "1",
				},
			},
			expBidOI: decimal.Zero,
			expAskOI: decimal.NewFromFloat(1),
		},
		{
			name: "multiple orders",
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
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusPlaced,
					ClientOrderID: "2",
				},
				{
					Price:         decimal.NewFromFloat(102),
					Size:          decimal.NewFromFloat(1),
					Side:          models.OrderSideSell,
					Status:        models.OrderStatusPlaced,
					ClientOrderID: "3",
				},
				{
					Price:         decimal.NewFromFloat(103),
					Size:          decimal.NewFromFloat(1),
					Side:          models.OrderSideSell,
					Status:        models.OrderStatusPlaced,
					ClientOrderID: "4",
				},
			},
			expBidOI: decimal.NewFromFloat(2),
			expAskOI: decimal.NewFromFloat(2),
		},
		{
			name: "multiple orders with cancel",
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
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(1),
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusCanceled,
					ClientOrderID: "1",
				},
				{
					Price:         decimal.NewFromFloat(101),
					Size:          decimal.NewFromFloat(1),
					Side:          models.OrderSideSell,
					Status:        models.OrderStatusCanceled,
					ClientOrderID: "2",
				},
			},
			expBidOI: decimal.Zero,
			expAskOI: decimal.Zero,
		},
		{
			name: "multiple orders with fill",
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
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(1),
					FilledSize:    decimal.NewFromFloat(1),
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusFilled,
					ClientOrderID: "1",
				},
				{
					Price:         decimal.NewFromFloat(101),
					Size:          decimal.NewFromFloat(1),
					FilledSize:    decimal.NewFromFloat(1),
					Side:          models.OrderSideSell,
					Status:        models.OrderStatusFilled,
					ClientOrderID: "2",
				},
			},
			expBidOI: decimal.Zero,
			expAskOI: decimal.Zero,
		},
		{
			name: "market immediately filled",
			orders: []models.Order{
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(1),
					FilledSize:    decimal.NewFromFloat(1),
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusFilled,
					ClientOrderID: "1",
				},
				{
					Price:         decimal.NewFromFloat(101),
					Size:          decimal.NewFromFloat(1),
					FilledSize:    decimal.NewFromFloat(1),
					Status:        models.OrderStatusFilled,
					Side:          models.OrderSideSell,
					ClientOrderID: "2",
				},
			},
			expBidOI: decimal.Zero,
			expAskOI: decimal.Zero,
		},
		{
			name: "market immediately filled with partial fill",
			orders: []models.Order{
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(1),
					FilledSize:    decimal.NewFromFloat(0.5),
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusPartiallyFilled,
					ClientOrderID: "1",
				},
				{
					Price:         decimal.NewFromFloat(101),
					Size:          decimal.NewFromFloat(1),
					FilledSize:    decimal.NewFromFloat(0.5),
					Status:        models.OrderStatusPartiallyFilled,
					Side:          models.OrderSideSell,
					ClientOrderID: "2",
				},
			},
			expBidOI: decimal.NewFromFloat(0.5),
			expAskOI: decimal.NewFromFloat(0.5),
		},
		{
			name: "place then partially fill",
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
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(1),
					FilledSize:    decimal.NewFromFloat(0.5),
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusPartiallyFilled,
					ClientOrderID: "1",
				},
				{
					Price:         decimal.NewFromFloat(101),
					Size:          decimal.NewFromFloat(1),
					FilledSize:    decimal.NewFromFloat(0.5),
					Status:        models.OrderStatusPartiallyFilled,
					Side:          models.OrderSideSell,
					ClientOrderID: "2",
				},
			},
			expBidOI: decimal.NewFromFloat(0.5),
			expAskOI: decimal.NewFromFloat(0.5),
		},
		{
			name: "partially fill then cancel",
			orders: []models.Order{
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(1),
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusPlaced,
					ClientOrderID: "1",
				},
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(1),
					FilledSize:    decimal.NewFromFloat(0.5),
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusPartiallyFilled,
					ClientOrderID: "1",
				},
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(1),
					FilledSize:    decimal.NewFromFloat(0.5),
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusCanceled,
					ClientOrderID: "1",
				},
			},
			expBidOI: decimal.Zero,
			expAskOI: decimal.Zero,
		},
		{
			name: "multiple partial fills",
			orders: []models.Order{
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(1),
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusPlaced,
					ClientOrderID: "1",
				},
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(1),
					FilledSize:    decimal.NewFromFloat(0.3),
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusPartiallyFilled,
					ClientOrderID: "1",
				},
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(1),
					FilledSize:    decimal.NewFromFloat(0.7),
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusPartiallyFilled,
					ClientOrderID: "1",
				},
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(1),
					FilledSize:    decimal.NewFromFloat(1.0),
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusFilled,
					ClientOrderID: "1",
				},
			},
			expBidOI: decimal.Zero,
			expAskOI: decimal.Zero,
		},
		{
			name: "cancel multiple orders",
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
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusPlaced,
					ClientOrderID: "2",
				},
				{
					Price:         decimal.NewFromFloat(100),
					Size:          decimal.NewFromFloat(1),
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusCanceled,
					ClientOrderID: "1",
				},
				{
					Price:         decimal.NewFromFloat(101),
					Size:          decimal.NewFromFloat(1),
					Side:          models.OrderSideBuy,
					Status:        models.OrderStatusCanceled,
					ClientOrderID: "2",
				},
			},
			expBidOI: decimal.Zero,
			expAskOI: decimal.Zero,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oi := newOpenInterest()
			for _, order := range tc.orders {
				if err := oi.observe(order); err != nil {
					t.Errorf("error observing order: %v", err)
				}
			}

			if !oi.totalBidSize.Equal(tc.expBidOI) {
				t.Errorf("expected bidOI %s, got %s", tc.expBidOI, oi.totalBidSize)
			}

			if !oi.totalAskSize.Equal(tc.expAskOI) {
				t.Errorf("expected askOI %s, got %s", tc.expAskOI, oi.totalAskSize)
			}
		})
	}
}

