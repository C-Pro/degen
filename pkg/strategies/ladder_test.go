package strategies

import (
	"testing"
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
