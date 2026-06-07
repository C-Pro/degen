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

func TestAccount_Recovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	d := dummy.NewDummy(ctx, "key", "secret", "", "")
	d.Generator = func(ctx context.Context, d *dummy.Dummy, ch chan<- models.ExchangeMessage) {
		<-ctx.Done()
	}

	// Pre-populate dummy exchange
	symbol := "BTCUSDT"
	pos := models.Position{
		Amount:       decimal.NewFromFloat(1.5),
		AveragePrice: decimal.NewFromFloat(50000),
		UpdatedAt:    time.Now().UTC(),
	}
	d.SetPosition(pos, symbol)

	bal := models.Balance{
		Total:     decimal.NewFromFloat(100000),
		Available: decimal.NewFromFloat(100000),
		UpdatedAt: time.Now().UTC(),
	}
	d.SetBalance(bal, "USDT")

	order := models.Order{
		Symbol:        symbol,
		ClientOrderID: "order1",
		Price:         decimal.NewFromFloat(49000),
		Size:          decimal.NewFromFloat(0.1),
		Side:          models.OrderSideBuy,
		Status:        models.OrderStatusPlaced,
	}
	d.SetOrder(order)

	// Initialize Account
	acc, err := NewAccount("test-acc", d)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	// Verify recovered state
	recoveredPos := acc.GetPosition(symbol)
	if !recoveredPos.Amount.Equal(pos.Amount) {
		t.Errorf("expected position amount %s, got %s", pos.Amount, recoveredPos.Amount)
	}
	if !recoveredPos.AveragePrice.Equal(pos.AveragePrice) {
		t.Errorf("expected average price %s, got %s", pos.AveragePrice, recoveredPos.AveragePrice)
	}

	recoveredBal := acc.GetBalance("USDT")
	if !recoveredBal.Total.Equal(bal.Total) {
		t.Errorf("expected balance total %s, got %s", bal.Total, recoveredBal.Total)
	}

	openOrders := acc.GetOpenOrders(symbol)
	if len(openOrders) != 1 {
		t.Errorf("expected 1 open order, got %d", len(openOrders))
	} else if openOrders[0].ClientOrderID != order.ClientOrderID {
		t.Errorf("expected order ID %s, got %s", order.ClientOrderID, openOrders[0].ClientOrderID)
	}
}

func TestAccount_StreamProcessing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	d := dummy.NewDummy(ctx, "key", "secret", "", "")
	d.Generator = func(ctx context.Context, d *dummy.Dummy, ch chan<- models.ExchangeMessage) {
		<-ctx.Done()
	}

	acc, err := NewAccount("test-acc", d)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	// Subscribe to internal dummy streams so it emits messages.
	if err := d.SubscribeUserBalance(ctx); err != nil {
		t.Fatalf("failed to subscribe to user balance: %v", err)
	}
	if err := d.SubscribeUserPositions(ctx); err != nil {
		t.Fatalf("failed to subscribe to user positions: %v", err)
	}
	if err := d.SubscribeUserOrders(ctx); err != nil {
		t.Fatalf("failed to subscribe to user orders: %v", err)
	}

	if err := acc.Start(ctx); err != nil {
		t.Fatalf("failed to start account: %v", err)
	}
	defer acc.Stop()

	// Give it a bit of time to start the Listen goroutine and initialize the channel.
	time.Sleep(100 * time.Millisecond)

	symbol := "BTCUSDT"

	// 1. Test Balance Update
	bal := models.Balance{
		Total:     decimal.NewFromFloat(50000),
		Available: decimal.NewFromFloat(50000),
		UpdatedAt: time.Now().UTC(),
	}
	d.SetBalance(bal, "USDT")

	// Wait for update to be processed and emitted via updCh
	select {
	case msg := <-acc.Updates():
		if msg.MsgType != models.MsgTypeBalanceUpdate {
			t.Errorf("expected balance update msg, got %v", msg.MsgType)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for balance update")
	}

	if !acc.GetBalance("USDT").Total.Equal(bal.Total) {
		t.Errorf("expected balance total %s, got %s", bal.Total, acc.GetBalance("USDT").Total)
	}

	// 2. Test Position Update
	pos := models.Position{
		Amount:       decimal.NewFromFloat(0.5),
		AveragePrice: decimal.NewFromFloat(40000),
		UpdatedAt:    time.Now().UTC(),
	}
	d.SetPosition(pos, symbol)

	select {
	case msg := <-acc.Updates():
		if msg.MsgType != models.MsgTypePositionUpdate {
			t.Errorf("expected position update msg, got %v", msg.MsgType)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for position update")
	}

	if !acc.GetPosition(symbol).Amount.Equal(pos.Amount) {
		t.Errorf("expected position amount %s, got %s", pos.Amount, acc.GetPosition(symbol).Amount)
	}

	// 3. Test Order Update
	order := models.Order{
		Symbol:        symbol,
		ClientOrderID: "order2",
		Price:         decimal.NewFromFloat(41000),
		Size:          decimal.NewFromFloat(0.2),
		Side:          models.OrderSideSell,
		Status:        models.OrderStatusPlaced,
		UpdatedAt:     time.Now().UTC(),
	}
	d.SetOrder(order)

	select {
	case msg := <-acc.Updates():
		if msg.MsgType != models.MsgTypeOrderStatus {
			t.Errorf("expected order status msg, got %v", msg.MsgType)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for order update")
	}

	o := acc.GetOrder(symbol, "order2")
	if o == nil || o.ClientOrderID != "order2" {
		t.Errorf("expected order2 to be found")
	}
}

func TestAccount_BreakEvenLogic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	d := dummy.NewDummy(ctx, "key", "secret", "", "")
	d.Generator = func(ctx context.Context, d *dummy.Dummy, ch chan<- models.ExchangeMessage) {
		<-ctx.Done()
	}

	acc, err := NewAccount("test-acc", d)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	symbol := "BTCUSDT"

	// Create a complex position: 1.0 @ 50000, 0.5 @ 52000
	acc.UpdatePosition(symbol, decimal.NewFromFloat(1.0), decimal.NewFromFloat(50000), time.Now().UTC())
	acc.UpdatePosition(symbol, decimal.NewFromFloat(0.5), decimal.NewFromFloat(52000), time.Now().UTC())

	// Total size: 1.5, Avg Price: (1.0*50000 + 0.5*52000) / 1.5 = 50666.666...

	// MinReducePrice should be 50000 (the entry that makes it break even earliest)
	minPrice := acc.GetPositionMinReducePrice(symbol)
	if !minPrice.Equal(decimal.NewFromFloat(50000)) {
		t.Errorf("expected min reduce price 50000, got %s", minPrice)
	}

	// Reduce size at price 51000.
	// Profitable entries: 1.0 @ 50000.
	// Total size available to reduce at 51000 is 1.0.
	reduceSize := acc.GetPositionReduceSize(symbol, decimal.NewFromFloat(51000))
	if !reduceSize.Equal(decimal.NewFromFloat(1.0)) {
		t.Errorf("expected reduce size 1.0, got %s", reduceSize)
	}
}

func TestAccount_GettersValidation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	d := dummy.NewDummy(ctx, "key", "secret", "", "")
	d.Generator = func(ctx context.Context, d *dummy.Dummy, ch chan<- models.ExchangeMessage) {
		<-ctx.Done()
	}

	acc, err := NewAccount("test-acc", d)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	symbol := "BTCUSDT"

	// 1. Test Open Interest Getters
	order1 := models.Order{
		Symbol:        symbol,
		ClientOrderID: "bid1",
		Price:         decimal.NewFromFloat(40000),
		Size:          decimal.NewFromFloat(1.0),
		Side:          models.OrderSideBuy,
		Status:        models.OrderStatusPlaced,
	}
	order2 := models.Order{
		Symbol:        symbol,
		ClientOrderID: "ask1",
		Price:         decimal.NewFromFloat(60000),
		Size:          decimal.NewFromFloat(2.0),
		Side:          models.OrderSideSell,
		Status:        models.OrderStatusPlaced,
	}
	acc.UpdateOrder(order1)
	acc.UpdateOrder(order2)

	if !acc.GetTotalBidSize(symbol).Equal(decimal.NewFromFloat(1.0)) {
		t.Errorf("expected total bid size 1.0, got %s", acc.GetTotalBidSize(symbol))
	}
	if !acc.GetTotalAskSize(symbol).Equal(decimal.NewFromFloat(2.0)) {
		t.Errorf("expected total ask size 2.0, got %s", acc.GetTotalAskSize(symbol))
	}

	// 2. Test GetOpenOrders
	openOrders := acc.GetOpenOrders(symbol)
	if len(openOrders) != 2 {
		t.Errorf("expected 2 open orders, got %d", len(openOrders))
	}

	// 3. Test GetOrder
	o := acc.GetOrder(symbol, "bid1")
	if o == nil || !o.Size.Equal(decimal.NewFromFloat(1.0)) {
		t.Errorf("failed to get correct order bid1")
	}
}

