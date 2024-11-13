package strategies

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
				p.add(trade[0], trade[1])
			}

			if p.avgPrice != tc.expAvgPrice {
				t.Errorf("expected avgPrice %v, got %v", tc.expAvgPrice, p.avgPrice)
			}

			if p.totalSize != tc.expTotalSize {
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
				p.add(trade[0], trade[1])
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

	eq := func(a, b float64) bool {
		return math.Abs(a-b) < sizeTolerance
	}

	p := positionStructure{}
	for i := 0; i < numTrades; i++ {
		size := float64(rand.Intn(1000000)-500000.0) / 1000.0
		price := float64(rand.Intn(100000)) / 1000.0

		totalSize += size
		sizePrice += size * price

		p.add(price, size)

		if !eq(p.totalSize, totalSize) {
			t.Errorf("%05d: expected totalSize %v, got %v", i, totalSize, p.totalSize)
		}

		reducePrice := 0.0
		if !p.long {
			reducePrice = math.MaxFloat64
		}
		if eq(totalSize, p.getReduceSize(reducePrice)) {
			t.Errorf("%05d: expected total internal size %v, got %v", i, totalSize, p.getReduceSize(reducePrice))
		}
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
				oi.observe(order)
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
			oi := newOpenInterest()

			for _, order := range tc.orders {
				oi.observe(order)
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
