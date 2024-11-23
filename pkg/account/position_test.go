package account

import (
	"math"
	"math/rand"
	"testing"

	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

func TestPositionAddAvgPrice(t *testing.T) {
	cases := []struct {
		name         string
		trades       [][2]float64
		expAvgPrice  float64
		expTotalSize float64
		expLong      bool
	}{
		{
			name: "add one trade long",
			trades: [][2]float64{
				{100, 1},
			},
			expAvgPrice:  100,
			expTotalSize: 1,
			expLong:      true,
		},
		{
			name: "one trade short",
			trades: [][2]float64{
				{100, -1},
			},
			expAvgPrice:  100,
			expTotalSize: -1,
			expLong:      false,
		},
		{
			name: "two trades long",
			trades: [][2]float64{
				{100, 1},
				{101, 1},
			},
			expAvgPrice:  100.5,
			expTotalSize: 2,
			expLong:      true,
		},
		{
			name: "close long position 1",
			trades: [][2]float64{
				{100, 1},
				{101, -1},
			},
			expAvgPrice:  0,
			expTotalSize: 0,
			expLong:      true,
		},
		{
			name: "close long position 2",
			trades: [][2]float64{
				{100, 1},
				{101, 1},
				{102, -2},
			},
			expAvgPrice:  0,
			expTotalSize: 0,
			expLong:      true,
		},
		{
			name: "add multiple trades long",
			trades: [][2]float64{
				{100, 1},
				{101, 1},
				{102, 1},
			},
			expAvgPrice:  101,
			expTotalSize: 3,
			expLong:      true,
		},
		{
			name: "add multiple trades short",
			trades: [][2]float64{
				{100, -1},
				{101, -1},
				{102, -1},
			},
			expAvgPrice:  101,
			expTotalSize: -3,
			expLong:      false,
		},
		{
			name: "reduce long position partially",
			trades: [][2]float64{
				{100, 2},
				{101, -1},
			},
			expAvgPrice:  100,
			expTotalSize: 1,
			expLong:      true,
		},
		{
			name: "reduce short position partially",
			trades: [][2]float64{
				{100, -2},
				{101, 1},
			},
			expAvgPrice:  100,
			expTotalSize: -1,
			expLong:      false,
		},
		{
			name: "flip position from long to short",
			trades: [][2]float64{
				{100, 1},
				{101, -2},
			},
			expAvgPrice:  101,
			expTotalSize: -1,
			expLong:      false,
		},
		{
			name: "flip position from short to long",
			trades: [][2]float64{
				{100, -1},
				{101, 2},
			},
			expAvgPrice:  101,
			expTotalSize: 1,
			expLong:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := positionStructure{}
			if tc.name == "close long position 2" {
				t.Log("here")
			}

			for _, trade := range tc.trades {
				upd := models.PositionUpdate{
					Price:  decimal.NewFromFloat(trade[0]),
					Amount: decimal.NewFromFloat(trade[1]),
				}
				p.Update(upd)
			}

			if p.avgPrice.InexactFloat64() != tc.expAvgPrice {
				t.Errorf("expected avgPrice %v, got %v", tc.expAvgPrice, p.avgPrice)
			}

			if p.totalSize.InexactFloat64() != tc.expTotalSize {
				t.Errorf("expected totalSize %v, got %v", tc.expTotalSize, p.totalSize)
			}

			if p.long != tc.expLong {
				t.Errorf("expected long %v, got %v", tc.expLong, p.long)
			}
		})
	}
}

func TestGetMinReducePrice(t *testing.T) {
	cases := []struct {
		name           string
		trades         [][2]float64
		reduceSize     float64
		expReducePrice float64
		expReduceSize  float64
	}{
		{
			name:           "empty position",
			trades:         [][2]float64{},
			reduceSize:     1,
			expReducePrice: 0,
			expReduceSize:  0,
		},
		{
			name: "reduce long position 1 (partial)",
			trades: [][2]float64{
				{100, 1},
			},
			reduceSize:     0.5,
			expReducePrice: 100,
			expReduceSize:  0.5,
		},
		{
			name: "reduce long position 1 (full)",
			trades: [][2]float64{
				{100, 1},
			},
			reduceSize:     1,
			expReducePrice: 100,
			expReduceSize:  1,
		},
		{
			name: "reduce long position 2 (partial)",
			trades: [][2]float64{
				{100, 1},
				{101, 1},
			},
			reduceSize:     1.5,
			expReducePrice: 100.33333333333333,
			expReduceSize:  1.5,
		},
		{
			name: "reduce long position 2 (full, not enough)",
			trades: [][2]float64{
				{100, 1},
				{101, 1},
			},
			reduceSize:     3,
			expReducePrice: 100.5,
			expReduceSize:  2,
		},
		{
			name: "reduce short position 1 (partial)",
			trades: [][2]float64{
				{100, -1},
			},
			reduceSize:     0.5,
			expReducePrice: 100,
			expReduceSize:  0.5,
		},
		{
			name: "reduce short position 1 (full)",
			trades: [][2]float64{
				{100, -1},
			},
			reduceSize:     1,
			expReducePrice: 100,
			expReduceSize:  1,
		},
		{
			name: "reduce short position 2 (partial)",
			trades: [][2]float64{
				{100, -1},
				{101, -1},
			},
			reduceSize:     1.5,
			expReducePrice: 100.66666666666667,
			expReduceSize:  1.5,
		},
		{
			name: "reduce short position 2 (full, not enough)",
			trades: [][2]float64{
				{100, -1},
				{101, -1},
			},
			reduceSize:     3,
			expReducePrice: 100.5,
			expReduceSize:  2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := positionStructure{}
			for _, trade := range tc.trades {
				upd := models.PositionUpdate{
					Price:  decimal.NewFromFloat(trade[0]),
					Amount: decimal.NewFromFloat(trade[1]),
				}
				p.Update(upd)
			}

			price, size := p.getMinReducePrice(tc.reduceSize)
			if price != tc.expReducePrice {
				t.Errorf("expected reduce price %v, got %v", tc.expReducePrice, price)
			}

			if size != tc.expReduceSize {
				t.Errorf("expected reduce size %v, got %v", tc.expReduceSize, size)
			}
		})
	}
}

func TestPositionAddFuzz(t *testing.T) {
	const numTrades = 10000
	var (
		totalSize float64
		sizePrice float64
		// Comparison tolerance for floating point numbers.
		sizeTolerance = 0.0001
	)

	eq := func(a decimal.Decimal, b float64) bool {
		return a.Sub(decimal.NewFromFloat(b)).Abs().InexactFloat64() < sizeTolerance
	}

	p := positionStructure{}
	for i := 0; i < numTrades; i++ {
		size := float64(rand.Intn(1000000)-500000.0) / 1000.0
		price := float64(rand.Intn(100000)) / 1000.0

		totalSize += size
		sizePrice += size * price

		upd := models.PositionUpdate{
			Price:  decimal.NewFromFloat(price),
			Amount: decimal.NewFromFloat(size),
		}
		p.Update(upd)

		if !eq(p.totalSize, totalSize) {
			t.Errorf("%05d: expected totalSize %v, got %v", i, totalSize, p.totalSize)
		}

		reducePrice := 0.0
		if !p.long {
			reducePrice = math.MaxFloat64
		}
		if eq(decimal.NewFromFloat(totalSize), p.getReduceSize(reducePrice)) {
			t.Errorf("%05d: expected total internal size %v, got %v", i, totalSize, p.getReduceSize(reducePrice))
		}
	}
}
