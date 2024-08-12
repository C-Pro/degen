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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAccount("test", dummy.NewDummy(
				context.Background(),
				"key", "secret", "https://test.com", "wss://test.com/ws",
			))
			a.positions[tc.symbol] = tc.postition
			a.UpdatePosition(tc.symbol, tc.amount, tc.price, ts)
			if diff := cmp.Diff(tc.expected, a.positions[tc.symbol]); diff != "" {
				t.Errorf("UpdatePosition() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
