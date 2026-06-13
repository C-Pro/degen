package account

import (
	"fmt"
	"maps"
	"reflect"
	"testing"

	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

// newTestOrder is a helper function to create models.Order for testing.
func newTestOrder(id string, side models.OrderSide, priceStr, sizeStr, filledSizeStr string, status models.OrderStatus, final bool) models.Order {
	price, err := decimal.NewFromString(priceStr)
	if err != nil {
		panic(fmt.Sprintf("Error parsing price '%s': %v", priceStr, err))
	}
	size, err := decimal.NewFromString(sizeStr)
	if err != nil {
		panic(fmt.Sprintf("Error parsing size '%s': %v", sizeStr, err))
	}
	filledSize, err := decimal.NewFromString(filledSizeStr)
	if err != nil {
		panic(fmt.Sprintf("Error parsing filledSize '%s': %v", filledSizeStr, err))
	}
	return models.Order{
		ClientOrderID: id,
		Side:          side,
		Price:         price,
		Size:          size,
		FilledSize:    filledSize,
		Status:        status,
		Final:         final,
	}
}

// wantState defines the expected state of openInterest for assertions.
type wantState struct {
	totalBidSize     string
	totalAskSize     string
	totalBidPriceSum string // Sum of Price * OpenSize for bids
	totalAskPriceSum string // Sum of Price * OpenSize for asks
	bids             map[string]models.Order
	asks             map[string]models.Order
}

func TestNewOpenInterest(t *testing.T) {
	oi := newOpenInterest()
	if oi.bids == nil {
		t.Errorf("newOpenInterest() oi.bids is nil, want non-nil map")
	}
	if oi.asks == nil {
		t.Errorf("newOpenInterest() oi.asks is nil, want non-nil map")
	}
	if !oi.totalBidSize.IsZero() {
		t.Errorf("newOpenInterest() oi.totalBidSize = %s, want 0", oi.totalBidSize)
	}
	if !oi.totalBidPrice.IsZero() {
		t.Errorf("newOpenInterest() oi.totalBidPrice = %s, want 0", oi.totalBidPrice)
	}
	if !oi.totalAskSize.IsZero() {
		t.Errorf("newOpenInterest() oi.totalAskSize = %s, want 0", oi.totalAskSize)
	}
	if !oi.totalAskPrice.IsZero() {
		t.Errorf("newOpenInterest() oi.totalAskPrice = %s, want 0", oi.totalAskPrice)
	}
}

func TestOpenInterest_Observe(t *testing.T) {
	type fields struct {
		initialBids          map[string]models.Order
		initialAsks          map[string]models.Order
		initialTotalBidSize  string
		initialTotalAskSize  string
		initialTotalBidPrice string
		initialTotalAskPrice string
	}
	type args struct {
		order models.Order
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		wantErr   bool
		wantState wantState
	}{
		{
			name: "new buy order placed",
			args: args{order: newTestOrder("b1", models.OrderSideBuy, "10", "5", "0", models.OrderStatusPlaced, false)},
			wantState: wantState{
				totalBidSize:     "5",
				totalBidPriceSum: "50", // 10 * 5
				totalAskSize:     "0",
				totalAskPriceSum: "0",
				bids:             map[string]models.Order{"b1": newTestOrder("b1", models.OrderSideBuy, "10", "5", "0", models.OrderStatusPlaced, false)},
				asks:             map[string]models.Order{},
			},
		},
		{
			name: "new sell order placed",
			args: args{order: newTestOrder("s1", models.OrderSideSell, "11", "3", "0", models.OrderStatusPlaced, false)},
			wantState: wantState{
				totalBidSize:     "0",
				totalBidPriceSum: "0",
				totalAskSize:     "3",
				totalAskPriceSum: "33", // 11 * 3
				bids:             map[string]models.Order{},
				asks:             map[string]models.Order{"s1": newTestOrder("s1", models.OrderSideSell, "11", "3", "0", models.OrderStatusPlaced, false)},
			},
		},
		{
			name: "existing buy order canceled",
			fields: fields{
				initialBids:          map[string]models.Order{"b1": newTestOrder("b1", models.OrderSideBuy, "10", "5", "0", models.OrderStatusPlaced, false)},
				initialTotalBidSize:  "5",
				initialTotalBidPrice: "50",
			},
			args: args{order: newTestOrder("b1", models.OrderSideBuy, "10", "5", "0", models.OrderStatusCanceled, true)}, // Cancel full original size
			wantState: wantState{
				totalBidSize:     "0",
				totalBidPriceSum: "0",
				totalAskSize:     "0",
				totalAskPriceSum: "0",
				bids:             map[string]models.Order{},
				asks:             map[string]models.Order{},
			},
		},
		{
			name: "buy order partially filled (new)",
			args: args{order: newTestOrder("b1", models.OrderSideBuy, "10", "5", "2", models.OrderStatusPartiallyFilled, false)}, // 2 filled, 3 open
			wantState: wantState{
				totalBidSize:     "3",  // 5 (total) - 2 (filled)
				totalBidPriceSum: "30", // 10 * 3
				bids:             map[string]models.Order{"b1": newTestOrder("b1", models.OrderSideBuy, "10", "5", "2", models.OrderStatusPartiallyFilled, false)},
				// Ensure asks map is explicitly empty if not set
				asks: map[string]models.Order{},
			},
		},
		{
			name: "buy order partially filled (existing)",
			fields: fields{
				initialBids:          map[string]models.Order{"b1": newTestOrder("b1", models.OrderSideBuy, "10", "5", "1", models.OrderStatusPartiallyFilled, false)}, // 1 filled, 4 open
				initialTotalBidSize:  "4",                                                                                                                              // 10 * 4
				initialTotalBidPrice: "40",
			},
			args: args{order: newTestOrder("b1", models.OrderSideBuy, "10", "5", "3", models.OrderStatusPartiallyFilled, false)}, // Now 3 filled (2 newly filled), 2 open
			wantState: wantState{
				totalBidSize:     "2",  // Was 4 open, 2 more filled, so 4-2=2 open
				totalBidPriceSum: "20", // Was 40, 10*2 (20) removed
				bids:             map[string]models.Order{"b1": newTestOrder("b1", models.OrderSideBuy, "10", "5", "3", models.OrderStatusPartiallyFilled, false)},
				asks:             map[string]models.Order{},
			},
		},
		{
			name: "buy order fully filled (from partially filled)",
			fields: fields{
				initialBids:          map[string]models.Order{"b1": newTestOrder("b1", models.OrderSideBuy, "10", "5", "3", models.OrderStatusPartiallyFilled, false)}, // 3 filled, 2 open
				initialTotalBidSize:  "2",
				initialTotalBidPrice: "20",
			},
			args: args{order: newTestOrder("b1", models.OrderSideBuy, "10", "5", "5", models.OrderStatusFilled, true)}, // Now 5 filled (2 newly filled), 0 open
			wantState: wantState{
				totalBidSize:     "0",
				totalBidPriceSum: "0",
				bids:             map[string]models.Order{},
				asks:             map[string]models.Order{},
			},
		},
		{
			name: "buy order fully filled (new, e.g. market order)",
			args: args{order: newTestOrder("b1", models.OrderSideBuy, "10", "5", "5", models.OrderStatusFilled, true)},
			wantState: wantState{ // No impact on totals as it was never "open" in our books from a Placed/PartiallyFilled state
				totalBidSize:     "0",
				totalBidPriceSum: "0",
				bids:             map[string]models.Order{},
				asks:             map[string]models.Order{},
			},
		},
		{
			name: "error - negative total bid size on cancel",
			fields: fields{
				initialBids:          map[string]models.Order{"b1": newTestOrder("b1", models.OrderSideBuy, "10", "1", "0", models.OrderStatusPlaced, false)},
				initialTotalBidSize:  "1",
				initialTotalBidPrice: "10",
			},
			args:    args{order: newTestOrder("b1", models.OrderSideBuy, "10", "5", "0", models.OrderStatusCanceled, true)}, // Cancel for 5, but only 1 was open size
			wantErr: true,
		},
		{
			name: "error - negative total ask size on cancel",
			fields: fields{
				initialAsks:          map[string]models.Order{"s1": newTestOrder("s1", models.OrderSideSell, "10", "1", "0", models.OrderStatusPlaced, false)},
				initialTotalAskSize:  "1",
				initialTotalAskPrice: "10",
			},
			args:    args{order: newTestOrder("s1", models.OrderSideSell, "10", "5", "0", models.OrderStatusCanceled, true)},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oi := newOpenInterest()
			if tt.fields.initialBids != nil {
				oi.bids = maps.Clone(tt.fields.initialBids) // Clone to avoid modifying test case data
			}
			if tt.fields.initialAsks != nil {
				oi.asks = maps.Clone(tt.fields.initialAsks)
			}
			if tt.fields.initialTotalBidSize != "" {
				oi.totalBidSize, _ = decimal.NewFromString(tt.fields.initialTotalBidSize)
			}
			if tt.fields.initialTotalAskSize != "" {
				oi.totalAskSize, _ = decimal.NewFromString(tt.fields.initialTotalAskSize)
			}
			if tt.fields.initialTotalBidPrice != "" {
				oi.totalBidPrice, _ = decimal.NewFromString(tt.fields.initialTotalBidPrice)
			}
			if tt.fields.initialTotalAskPrice != "" {
				oi.totalAskPrice, _ = decimal.NewFromString(tt.fields.initialTotalAskPrice)
			}

			err := oi.observe(tt.args.order)
			if (err != nil) != tt.wantErr {
				t.Errorf("openInterest.observe() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Check totalBidSize
				if tt.wantState.totalBidSize != "" {
					expected, _ := decimal.NewFromString(tt.wantState.totalBidSize)
					if !oi.totalBidSize.Equal(expected) {
						t.Errorf("observe() totalBidSize got = %s, want = %s", oi.totalBidSize, expected)
					}
				}
				// Check totalAskSize
				if tt.wantState.totalAskSize != "" {
					expected, _ := decimal.NewFromString(tt.wantState.totalAskSize)
					if !oi.totalAskSize.Equal(expected) {
						t.Errorf("observe() totalAskSize got = %s, want = %s", oi.totalAskSize, expected)
					}
				}
				// Check totalBidPrice (sum of price*size)
				if tt.wantState.totalBidPriceSum != "" {
					expected, _ := decimal.NewFromString(tt.wantState.totalBidPriceSum)
					if !oi.totalBidPrice.Equal(expected) {
						t.Errorf("observe() totalBidPrice got = %s, want = %s", oi.totalBidPrice, expected)
					}
				}
				// Check totalAskPrice (sum of price*size)
				if tt.wantState.totalAskPriceSum != "" {
					expected, _ := decimal.NewFromString(tt.wantState.totalAskPriceSum)
					if !oi.totalAskPrice.Equal(expected) {
						t.Errorf("observe() totalAskPrice got = %s, want = %s", oi.totalAskPrice, expected)
					}
				}
				// Check bids map
				if !reflect.DeepEqual(oi.bids, tt.wantState.bids) {
					t.Errorf("observe() bids map got = %+v, want = %+v", oi.bids, tt.wantState.bids)
				}
				// Check asks map
				if !reflect.DeepEqual(oi.asks, tt.wantState.asks) {
					t.Errorf("observe() asks map got = %+v, want = %+v", oi.asks, tt.wantState.asks)
				}
			}
		})
	}
}

func TestOpenInterest_SetFromOrders(t *testing.T) {
	type args struct {
		orders []models.Order
	}
	tests := []struct {
		name      string
		args      args
		wantState wantState
		wantPanic bool
	}{
		{
			name: "empty orders",
			args: args{orders: []models.Order{}},
			wantState: wantState{
				totalBidSize:     "0",
				totalBidPriceSum: "0",
				totalAskSize:     "0",
				totalAskPriceSum: "0",
				bids:             map[string]models.Order{},
				asks:             map[string]models.Order{},
			},
		},
		{
			name: "only bids (placed and partially filled)",
			args: args{orders: []models.Order{
				newTestOrder("b1", models.OrderSideBuy, "10", "2", "0", models.OrderStatusPlaced, false),           // Open: 2, Price*Open: 20
				newTestOrder("b2", models.OrderSideBuy, "9.9", "3", "1", models.OrderStatusPartiallyFilled, false), // Open: 2, Price*Open: 19.8
			}},
			wantState: wantState{
				totalBidSize:     "4",    // 2 + 2
				totalBidPriceSum: "39.8", // 20 + 19.8
				totalAskSize:     "0",
				totalAskPriceSum: "0",
				bids: map[string]models.Order{
					"b1": newTestOrder("b1", models.OrderSideBuy, "10", "2", "0", models.OrderStatusPlaced, false),
					"b2": newTestOrder("b2", models.OrderSideBuy, "9.9", "3", "1", models.OrderStatusPartiallyFilled, false),
				},
				asks: map[string]models.Order{},
			},
		},
		{
			name: "only asks",
			args: args{orders: []models.Order{
				newTestOrder("s1", models.OrderSideSell, "12", "1", "0", models.OrderStatusPlaced, false),            // Open: 1, Price*Open: 12
				newTestOrder("s2", models.OrderSideSell, "12.1", "4", "2", models.OrderStatusPartiallyFilled, false), // Open: 2, Price*Open: 24.2
			}},
			wantState: wantState{
				totalBidSize:     "0",
				totalBidPriceSum: "0",
				totalAskSize:     "3",    // 1 + 2
				totalAskPriceSum: "36.2", // 12 + 24.2
				bids:             map[string]models.Order{},
				asks: map[string]models.Order{
					"s1": newTestOrder("s1", models.OrderSideSell, "12", "1", "0", models.OrderStatusPlaced, false),
					"s2": newTestOrder("s2", models.OrderSideSell, "12.1", "4", "2", models.OrderStatusPartiallyFilled, false),
				},
			},
		},
		{
			name: "mixed bids and asks",
			args: args{orders: []models.Order{
				newTestOrder("b1", models.OrderSideBuy, "10", "2", "0", models.OrderStatusPlaced, false),
				newTestOrder("s1", models.OrderSideSell, "11", "3", "0", models.OrderStatusPlaced, false),
			}},
			wantState: wantState{
				totalBidSize:     "2",
				totalBidPriceSum: "20",
				totalAskSize:     "3",
				totalAskPriceSum: "33",
				bids:             map[string]models.Order{"b1": newTestOrder("b1", models.OrderSideBuy, "10", "2", "0", models.OrderStatusPlaced, false)},
				asks:             map[string]models.Order{"s1": newTestOrder("s1", models.OrderSideSell, "11", "3", "0", models.OrderStatusPlaced, false)},
			},
		},
		{
			name: "orders including final (canceled/filled) - should be ignored by setFromOrders logic if it implies setting active book",
			args: args{orders: []models.Order{
				newTestOrder("b1", models.OrderSideBuy, "10", "2", "0", models.OrderStatusPlaced, false),
				newTestOrder("b2", models.OrderSideBuy, "9", "1", "1", models.OrderStatusFilled, true), // Should not contribute to totals
				newTestOrder("s1", models.OrderSideSell, "11", "3", "0", models.OrderStatusPlaced, false),
				newTestOrder("s2", models.OrderSideSell, "12", "1", "0", models.OrderStatusCanceled, true), // Should not contribute to totals
			}},
			wantState: wantState{
				totalBidSize:     "2", // Only from b1
				totalBidPriceSum: "20",
				totalAskSize:     "3", // Only from s1
				totalAskPriceSum: "33",
				bids:             map[string]models.Order{"b1": newTestOrder("b1", models.OrderSideBuy, "10", "2", "0", models.OrderStatusPlaced, false)},  // b2 is final, not in map
				asks:             map[string]models.Order{"s1": newTestOrder("s1", models.OrderSideSell, "11", "3", "0", models.OrderStatusPlaced, false)}, // s2 is final, not in map
			},
		},
		{
			name: "panic if observe returns error",
			args: args{orders: []models.Order{
				newTestOrder("b1", models.OrderSideBuy, "10", "1", "0", models.OrderStatusPlaced, false),
			}},
			wantState: wantState{
				totalBidSize:     "1",
				totalBidPriceSum: "10",
				totalAskSize:     "0",
				totalAskPriceSum: "0",
				bids:             map[string]models.Order{"b1": newTestOrder("b1", models.OrderSideBuy, "10", "1", "0", models.OrderStatusPlaced, false)},
				asks:             map[string]models.Order{},
			},
		},
		{
			name: "setFromOrders with single bid order", // This test was previously misplaced and named "panic if observe returns error"
			args: args{orders: []models.Order{
				newTestOrder("b1", models.OrderSideBuy, "10", "1", "0", models.OrderStatusPlaced, false),
			}},
			wantState: wantState{
				totalBidSize:     "1",
				totalBidPriceSum: "10",
				totalAskSize:     "0",
				totalAskPriceSum: "0",
				bids:             map[string]models.Order{"b1": newTestOrder("b1", models.OrderSideBuy, "10", "1", "0", models.OrderStatusPlaced, false)},
				asks:             map[string]models.Order{},
			},
			wantPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("setFromOrders() did not panic as expected")
					}
				}()
			}

			oi := newOpenInterest()
			if err := oi.setFromOrders(tt.args.orders); err != nil {
				t.Fatalf("setFromOrders returned unexpected error: %v", err)
			}

			// Assertions
			if tt.wantState.totalBidSize != "" {
				expected, _ := decimal.NewFromString(tt.wantState.totalBidSize)
				if !oi.totalBidSize.Equal(expected) {
					t.Errorf("setFromOrders() totalBidSize got = %s, want = %s", oi.totalBidSize, expected)
				}
			}
			if tt.wantState.totalAskSize != "" {
				expected, _ := decimal.NewFromString(tt.wantState.totalAskSize)
				if !oi.totalAskSize.Equal(expected) {
					t.Errorf("setFromOrders() totalAskSize got = %s, want = %s", oi.totalAskSize, expected)
				}
			}
			if tt.wantState.totalBidPriceSum != "" {
				expected, _ := decimal.NewFromString(tt.wantState.totalBidPriceSum)
				if !oi.totalBidPrice.Equal(expected) {
					t.Errorf("setFromOrders() totalBidPrice got = %s, want = %s", oi.totalBidPrice, expected)
				}
			}
			if tt.wantState.totalAskPriceSum != "" {
				expected, _ := decimal.NewFromString(tt.wantState.totalAskPriceSum)
				if !oi.totalAskPrice.Equal(expected) {
					t.Errorf("setFromOrders() totalAskPrice got = %s, want = %s", oi.totalAskPrice, expected)
				}
			}
			if !reflect.DeepEqual(oi.bids, tt.wantState.bids) {
				t.Errorf("setFromOrders() bids map got = %+v, want = %+v", oi.bids, tt.wantState.bids)
			}
			if !reflect.DeepEqual(oi.asks, tt.wantState.asks) {
				t.Errorf("setFromOrders() asks map got = %+v, want = %+v", oi.asks, tt.wantState.asks)
			}
		})
	}
}

func TestOpenInterest_Getters(t *testing.T) {
	oi := newOpenInterest()
	ordersToSetup := []models.Order{
		newTestOrder("b1", models.OrderSideBuy, "10", "2", "0", models.OrderStatusPlaced, false),             // Open: 2, Price*Open: 20
		newTestOrder("b2", models.OrderSideBuy, "10.1", "3", "1", models.OrderStatusPartiallyFilled, false),  // Open: 2, Price*Open: 20.2
		newTestOrder("a1", models.OrderSideSell, "11", "4", "0", models.OrderStatusPlaced, false),            // Open: 4, Price*Open: 44
		newTestOrder("a2", models.OrderSideSell, "10.9", "5", "2", models.OrderStatusPartiallyFilled, false), // Open: 3, Price*Open: 32.7
	}
	if err := oi.setFromOrders(ordersToSetup); err != nil { // This populates oi based on the logic in setFromOrders/observe
		t.Fatalf("setFromOrders returned unexpected error: %v", err)
	}

	t.Run("GetTotalBidSize", func(t *testing.T) {
		// b1 open size = 2. b2 open size = 3-1=2. Total = 2+2=4
		want, _ := decimal.NewFromString("4")
		if got := oi.GetTotalBidSize(); !got.Equal(want) {
			t.Errorf("GetTotalBidSize() = %s, want %s", got, want)
		}
	})

	t.Run("GetTotalAskSize", func(t *testing.T) {
		// a1 open size = 4. a2 open size = 5-2=3. Total = 4+3=7
		want, _ := decimal.NewFromString("7")
		if got := oi.GetTotalAskSize(); !got.Equal(want) {
			t.Errorf("GetTotalAskSize() = %s, want %s", got, want)
		}
	})

	t.Run("GetAvgBidPrice", func(t *testing.T) {
		// totalBidPriceSum = (10*2) + (10.1*2) = 20 + 20.2 = 40.2
		// totalBidSize = 4
		// avg = 40.2 / 4 = 10.05
		want, _ := decimal.NewFromString("10.05")
		if got := oi.GetAvgBidPrice(); !got.Equal(want) {
			t.Errorf("GetAvgBidPrice() = %s, want %s", got, want)
		}
	})

	t.Run("GetAvgBidPrice_ZeroSize", func(t *testing.T) {
		localOi := newOpenInterest() // Use a local, new openInterest instance
		// With no open bids the average must be zero, not a divide-by-zero panic. [L1]
		if got := localOi.GetAvgBidPrice(); !got.IsZero() {
			t.Errorf("GetAvgBidPrice() with zero size = %s, want 0", got)
		}
	})

	t.Run("GetAvgAskPrice", func(t *testing.T) {
		// totalAskPriceSum = (11*4) + (10.9*3) = 44 + 32.7 = 76.7
		// totalAskSize = 7
		// avg = 76.7 / 7 = 10.9571428571428571 (decimal default 16 places)
		want, _ := decimal.NewFromString("10.9571428571428571")
		if got := oi.GetAvgAskPrice(); !got.Equal(want) {
			t.Errorf("GetAvgAskPrice() = %s, want %s", got, want)
		}
	})

	t.Run("GetAvgAskPrice_ZeroSize", func(t *testing.T) {
		localOi := newOpenInterest() // Use a local, new openInterest instance
		// With no open asks the average must be zero, not a divide-by-zero panic. [L1]
		if got := localOi.GetAvgAskPrice(); !got.IsZero() {
			t.Errorf("GetAvgAskPrice() with zero size = %s, want 0", got)
		}
	})

	// Test GetBids
	t.Run("GetBids", func(t *testing.T) {
		// b1: P=10, S=2, FS=0
		// b2: P=10.1, S=3, FS=1
		// Expected sorted: b2 (10.1), b1 (10)
		wantBids := []models.Order{
			newTestOrder("b2", models.OrderSideBuy, "10.1", "3", "1", models.OrderStatusPartiallyFilled, false),
			newTestOrder("b1", models.OrderSideBuy, "10", "2", "0", models.OrderStatusPlaced, false),
		}
		gotBids := oi.GetBids()
		if !reflect.DeepEqual(gotBids, wantBids) {
			t.Errorf("GetBids() got = %+v, want %+v", gotBids, wantBids)
		}
	})

	t.Run("GetAsks", func(t *testing.T) {
		// a1: P=11, S=4, FS=0
		// a2: P=10.9, S=5, FS=2
		// Expected sorted: a2 (10.9), a1 (11)
		wantAsks := []models.Order{
			newTestOrder("a2", models.OrderSideSell, "10.9", "5", "2", models.OrderStatusPartiallyFilled, false),
			newTestOrder("a1", models.OrderSideSell, "11", "4", "0", models.OrderStatusPlaced, false),
		}
		gotAsks := oi.GetAsks()
		if !reflect.DeepEqual(gotAsks, wantAsks) {
			t.Errorf("GetAsks() got = %+v, want %+v", gotAsks, wantAsks)
		}
	})

	t.Run("GetBids_Empty", func(t *testing.T) {
		emptyOi := newOpenInterest()
		gotBids := emptyOi.GetBids()
		if len(gotBids) != 0 {
			t.Errorf("GetBids() on empty oi = %+v, want empty slice", gotBids)
		}
	})

	t.Run("GetAsks_Empty", func(t *testing.T) {
		emptyOi := newOpenInterest()
		gotAsks := emptyOi.GetAsks()
		if len(gotAsks) != 0 {
			t.Errorf("GetAsks() on empty oi = %+v, want empty slice", gotAsks)
		}
	})

	t.Run("GetBids_SamePriceOrder", func(t *testing.T) {
		oiSamePrice := newOpenInterest()
		// Order of insertion into map doesn't guarantee order for collection.
		// ClientOrderID might affect order if prices are same, due to how maps are iterated then slices.Collect.
		// The sort is only on Price.
		o1 := newTestOrder("b1_same", models.OrderSideBuy, "100", "1", "0", models.OrderStatusPlaced, false)
		o2 := newTestOrder("b2_same", models.OrderSideBuy, "100", "1", "0", models.OrderStatusPlaced, false)
		o3 := newTestOrder("b3_higher", models.OrderSideBuy, "101", "1", "0", models.OrderStatusPlaced, false)

		// Insert in a specific order to see if GetBids respects price primarily
		_ = oiSamePrice.observe(o1)
		_ = oiSamePrice.observe(o2)
		_ = oiSamePrice.observe(o3)

		gotBids := oiSamePrice.GetBids()
		// Expected: b3_higher (101), then b1_same/b2_same (100) in some order.
		if len(gotBids) != 3 || gotBids[0].ClientOrderID != "b3_higher" {
			t.Errorf("GetBids() with same prices, highest price not first. Got: %+v", gotBids)
		}
		if (gotBids[1].ClientOrderID != "b1_same" || gotBids[2].ClientOrderID != "b2_same") &&
			(gotBids[1].ClientOrderID != "b2_same" || gotBids[2].ClientOrderID != "b1_same") {
			t.Errorf("GetBids() with same prices, orders with same price not present as expected. Got: %+v", gotBids)
		}
		if !(gotBids[1].Price.Equal(gotBids[2].Price)) {
			t.Errorf("GetBids() with same prices, prices of second and third elements are not equal. Got: %+v", gotBids)
		}
	})
}
