package models

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestUnrealizedPnL(t *testing.T) {
	cases := []struct {
		name     string
		position Position
		price    decimal.Decimal
		want     decimal.Decimal
	}{
		{
			name: "long position, curr price is higher",
			position: Position{
				Amount:       decimal.NewFromInt(1),
				AveragePrice: decimal.NewFromInt(100),
			},
			price: decimal.NewFromInt(110),
			want:  decimal.NewFromInt(10),
		},
		{
			name: "long position, curr price is lower",
			position: Position{
				Amount:       decimal.NewFromInt(1),
				AveragePrice: decimal.NewFromInt(100),
			},
			price: decimal.NewFromInt(90),
			want:  decimal.NewFromInt(-10),
		},
		{
			name: "long position, price is the same",
			position: Position{
				Amount:       decimal.NewFromInt(1),
				AveragePrice: decimal.NewFromInt(100),
			},
			price: decimal.NewFromInt(100),
			want:  decimal.Zero,
		},
		{
			name: "no position",
			position: Position{
				Amount:       decimal.Zero,
				AveragePrice: decimal.Zero,
			},
			price: decimal.NewFromInt(100),
			want:  decimal.Zero,
		},
		{
			name: "short position, curr price is higher",
			position: Position{
				Amount:       decimal.NewFromInt(-1),
				AveragePrice: decimal.NewFromInt(100),
			},
			price: decimal.NewFromInt(110),
			want:  decimal.NewFromInt(-10),
		},
		{
			name: "short position, curr price is lower",
			position: Position{
				Amount:       decimal.NewFromInt(-1),
				AveragePrice: decimal.NewFromInt(100),
			},
			price: decimal.NewFromInt(90),
			want:  decimal.NewFromInt(10),
		},
		{
			name: "short position, price is the same",
			position: Position{
				Amount:       decimal.NewFromInt(-1),
				AveragePrice: decimal.NewFromInt(100),
			},
			price: decimal.NewFromInt(100),
			want:  decimal.Zero,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.position.UnrealizedPnL(tc.price)
			if !got.Equal(tc.want) {
				t.Errorf("UnrealizedPnL() = %v; want %v", got, tc.want)
			}
		})
	}
}
