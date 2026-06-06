package strategies

import (
	"context"
	"fmt"
	"testing"
	"time"

	"degen/pkg/account"
	"degen/pkg/connectors/dummy"
	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

func TestLadder_IdealAllocation(t *testing.T) {
	cfg := LadderConfig{
		PortfolioAllocation: decimal.NewFromFloat(0.5), // 50%
		LevelsCount:         3,
		LevelsSpread: []decimal.Decimal{
			decimal.NewFromFloat(0.01), // 1%
			decimal.NewFromFloat(0.02), // 2%
			decimal.NewFromFloat(0.03), // 3%
		},
		LevelsSize: []decimal.Decimal{
			decimal.NewFromFloat(0.2), // 20%
			decimal.NewFromFloat(0.3), // 30%
			decimal.NewFromFloat(0.5), // 50%
		},
		LevelsPriceTolerance: []decimal.Decimal{
			decimal.NewFromFloat(0.005),
			decimal.NewFromFloat(0.005),
			decimal.NewFromFloat(0.005),
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("config validation failed: %v", err)
	}

	// 1. Without penalty
	ideal := cfg.IdealAllocation(
		decimal.NewFromFloat(100.0),
		decimal.NewFromFloat(10.0),   // baseTotal
		decimal.NewFromFloat(1000.0), // quoteTotal
		decimal.Zero,
		decimal.Zero,
	)

	// Bids (going down, relative/geometric compounding):
	// L0: price = 100 - (100 * 0.01) = 99.0
	//     size = 1000 * 0.5 * 0.2 / 99 = 100 / 99 = 1.010101...
	// L1: price = 99 - (99 * 0.02) = 97.02
	//     size = 1000 * 0.5 * 0.3 / 97.02 = 150 / 97.02 = 1.546072...
	// L2: price = 97.02 - (97.02 * 0.03) = 94.1094
	//     size = 1000 * 0.5 * 0.5 / 94.1094 = 250 / 94.1094 = 2.656482...
	if len(ideal.Bids) != 3 {
		t.Errorf("expected 3 bids, got %d", len(ideal.Bids))
	} else {
		expectedBids := []struct {
			price decimal.Decimal
			size  decimal.Decimal
		}{
			{decimal.NewFromFloat(99.0), decimal.NewFromFloat(100.0).Div(decimal.NewFromFloat(99.0))},
			{decimal.NewFromFloat(97.02), decimal.NewFromFloat(150.0).Div(decimal.NewFromFloat(97.02))},
			{decimal.NewFromFloat(94.1094), decimal.NewFromFloat(250.0).Div(decimal.NewFromFloat(94.1094))},
		}
		for i, eb := range expectedBids {
			if !ideal.Bids[i][0].Equal(eb.price) {
				t.Errorf("bid %d price: expected %s, got %s", i, eb.price, ideal.Bids[i][0])
			}
			if !ideal.Bids[i][1].Equal(eb.size) {
				t.Errorf("bid %d size: expected %s, got %s", i, eb.size, ideal.Bids[i][1])
			}
		}
	}

	// Asks (going up, relative/geometric compounding):
	// L0: price = 100 + (100 * 0.01) = 101.0
	//     size = 10 * 0.5 * 0.2 = 1.0
	// L1: price = 101 + (101 * 0.02) = 103.02
	//     size = 10 * 0.5 * 0.3 = 1.5
	// L2: price = 103.02 + (103.02 * 0.03) = 106.1106
	//     size = 10 * 0.5 * 0.5 = 2.5
	if len(ideal.Asks) != 3 {
		t.Errorf("expected 3 asks, got %d", len(ideal.Asks))
	} else {
		expectedAsks := []struct {
			price decimal.Decimal
			size  decimal.Decimal
		}{
			{decimal.NewFromFloat(101.0), decimal.NewFromFloat(1.0)},
			{decimal.NewFromFloat(103.02), decimal.NewFromFloat(1.5)},
			{decimal.NewFromFloat(106.1106), decimal.NewFromFloat(2.5)},
		}
		for i, ea := range expectedAsks {
			if !ideal.Asks[i][0].Equal(ea.price) {
				t.Errorf("ask %d price: expected %s, got %s", i, ea.price, ideal.Asks[i][0])
			}
			if !ideal.Asks[i][1].Equal(ea.size) {
				t.Errorf("ask %d size: expected %s, got %s", i, ea.size, ideal.Asks[i][1])
			}
		}
	}

	// 2. With penalties
	idealWithPenalty := cfg.IdealAllocation(
		decimal.NewFromFloat(100.0),
		decimal.NewFromFloat(10.0),
		decimal.NewFromFloat(1000.0),
		decimal.NewFromFloat(0.005), // bidPenalty 0.5%
		decimal.NewFromFloat(0.002), // askPenalty 0.2%
	)

	// Bids with penalty:
	// L0: price = 100 - (100 * (0.01 + 0.005)) = 98.5
	if len(idealWithPenalty.Bids) > 0 {
		expectedL0Price := decimal.NewFromFloat(98.5)
		if !idealWithPenalty.Bids[0][0].Equal(expectedL0Price) {
			t.Errorf("bid with penalty price: expected %s, got %s", expectedL0Price, idealWithPenalty.Bids[0][0])
		}
	}

	// Asks with penalty:
	// L0: price = 100 + (100 * (0.01 + 0.002)) = 101.2
	if len(idealWithPenalty.Asks) > 0 {
		expectedL0Price := decimal.NewFromFloat(101.2)
		if !idealWithPenalty.Asks[0][0].Equal(expectedL0Price) {
			t.Errorf("ask with penalty price: expected %s, got %s", expectedL0Price, idealWithPenalty.Asks[0][0])
		}
	}
}

func TestLadder_ImbalancePenalties(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	d := dummy.NewDummy(ctx, "key", "secret", "", "")
	d.Generator = func(ctx context.Context, d *dummy.Dummy, ch chan<- models.ExchangeMessage) {
		<-ctx.Done()
	}

	sym := models.SymbolInfo{
		Symbol:           "BTCUSDT",
		Base:             "BTC",
		Quote:            "USDT",
		PriceTickSize:    decimal.NewFromFloat(0.01),
		QuantityTickSize: decimal.NewFromFloat(0.0001),
		MinQuantity:      decimal.NewFromFloat(0.0001),
	}
	d.SetSymbol(sym)

	acc, err := account.NewAccount("test-acc", d)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	cfg := LadderConfig{
		PortfolioAllocation: decimal.NewFromFloat(0.5),
		LevelsCount:         1,
		LevelsSpread:        []decimal.Decimal{decimal.NewFromFloat(0.01)},
		LevelsSize:          []decimal.Decimal{decimal.NewFromFloat(1.0)},
		LevelsPriceTolerance: []decimal.Decimal{decimal.NewFromFloat(0.005)},
	}

	ladder := NewLadder(ctx, acc, "BTCUSDT", cfg)
	if ladder == nil {
		t.Fatalf("failed to create ladder strategy")
	}

	// Initially, no open orders, imbalance is 0
	if !ladder.bidSpreadPenalty().IsZero() {
		t.Errorf("expected 0 bid penalty, got %s", ladder.bidSpreadPenalty())
	}
	if !ladder.askSpreadPenalty().IsZero() {
		t.Errorf("expected 0 ask penalty, got %s", ladder.askSpreadPenalty())
	}

	// 1. Bid-heavy imbalance (Total bid size = 10, Total ask size = 5)
	acc.UpdateOrder(models.Order{
		Symbol:        "BTCUSDT",
		ClientOrderID: "bid-o",
		Side:          models.OrderSideBuy,
		Size:          decimal.NewFromFloat(10.0),
		Price:         decimal.NewFromFloat(90.0),
		Status:        models.OrderStatusPlaced,
	})
	acc.UpdateOrder(models.Order{
		Symbol:        "BTCUSDT",
		ClientOrderID: "ask-o",
		Side:          models.OrderSideSell,
		Size:          decimal.NewFromFloat(5.0),
		Price:         decimal.NewFromFloat(110.0),
		Status:        models.OrderStatusPlaced,
	})

	// Imbalance = (10 - 5) / 10 = 0.5
	// Penalty = 0.5 * 0.05 = 0.025
	expectedBidPenalty := decimal.NewFromFloat(0.025)
	if !ladder.bidSpreadPenalty().Equal(expectedBidPenalty) {
		t.Errorf("expected bid penalty %s, got %s", expectedBidPenalty, ladder.bidSpreadPenalty())
	}
	if !ladder.askSpreadPenalty().IsZero() {
		t.Errorf("expected ask penalty to be zero, got %s", ladder.askSpreadPenalty())
	}
}

func TestLadder_GetDesiredOrders_WithPosition(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	d := dummy.NewDummy(ctx, "key", "secret", "", "")
	d.Generator = func(ctx context.Context, d *dummy.Dummy, ch chan<- models.ExchangeMessage) {
		<-ctx.Done()
	}

	sym := models.SymbolInfo{
		Symbol:           "BTCUSDT",
		Base:             "BTC",
		Quote:            "USDT",
		PriceTickSize:    decimal.NewFromFloat(0.01),
		QuantityTickSize: decimal.NewFromFloat(0.0001),
		MinQuantity:      decimal.NewFromFloat(0.0001),
	}
	d.SetSymbol(sym)

	acc, err := account.NewAccount("test-acc", d)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	cfg := LadderConfig{
		PortfolioAllocation: decimal.NewFromFloat(0.5),
		LevelsCount:         1,
		LevelsSpread:        []decimal.Decimal{decimal.NewFromFloat(0.01)},
		LevelsSize:          []decimal.Decimal{decimal.NewFromFloat(1.0)},
		LevelsPriceTolerance: []decimal.Decimal{decimal.NewFromFloat(0.005)},
	}

	ladder := NewLadder(ctx, acc, "BTCUSDT", cfg)
	if ladder == nil {
		t.Fatalf("failed to create ladder strategy")
	}

	// Set balances
	acc.UpdateBalance("BTC", decimal.NewFromFloat(1.0), decimal.NewFromFloat(1.0), time.Now().UTC())
	acc.UpdateBalance("USDT", decimal.NewFromFloat(100.0), decimal.NewFromFloat(100.0), time.Now().UTC())

	// Set long position with breakeven at 99.0
	acc.UpdatePosition("BTCUSDT", decimal.NewFromFloat(1.0), decimal.NewFromFloat(99.0), time.Now().UTC())

	// BBO: Bid 95.0, Ask 96.0, Mid 95.5
	bbo := models.BBO{
		Bid:       models.PriceLevel{Price: decimal.NewFromFloat(95.0), Size: decimal.NewFromFloat(1.0)},
		Ask:       models.PriceLevel{Price: decimal.NewFromFloat(96.0), Size: decimal.NewFromFloat(1.0)},
		Timestamp: time.Now().UTC(),
	}

	// Ideal ask at 95.5 + 1% = 96.455.
	// Since we are long and minReducePrice is 99.0, and 96.455 < 99.0,
	// ask must be adjusted upward by diff: 99.0 - 96.455 = 2.545.
	// Adjusted ask price: 96.455 + 2.545 = 99.0.
	_, asks := ladder.GetDesiredOrders(bbo)
	if len(asks) != 1 {
		t.Fatalf("expected 1 ask order, got %d", len(asks))
	}

	expectedAskPrice := decimal.NewFromFloat(99.0)
	if !asks[0].Price.Equal(expectedAskPrice) {
		t.Errorf("expected adjusted ask price %s, got %s", expectedAskPrice, asks[0].Price)
	}
}

func TestLadder_See_BBO(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	d := dummy.NewDummy(ctx, "key", "secret", "", "")
	d.Generator = func(ctx context.Context, d *dummy.Dummy, ch chan<- models.ExchangeMessage) {
		<-ctx.Done()
	}

	sym := models.SymbolInfo{
		Symbol:           "BTCUSDT",
		Base:             "BTC",
		Quote:            "USDT",
		PriceTickSize:    decimal.NewFromFloat(0.01),
		QuantityTickSize: decimal.NewFromFloat(0.0001),
		MinQuantity:      decimal.NewFromFloat(0.0001),
	}
	d.SetSymbol(sym)

	acc, err := account.NewAccount("test-acc", d)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	cfg := LadderConfig{
		PortfolioAllocation: decimal.NewFromFloat(0.5),
		LevelsCount:         1,
		LevelsSpread:        []decimal.Decimal{decimal.NewFromFloat(0.01)},
		LevelsSize:          []decimal.Decimal{decimal.NewFromFloat(1.0)},
		LevelsPriceTolerance: []decimal.Decimal{decimal.NewFromFloat(0.05)},
	}

	ladder := NewLadder(ctx, acc, "BTCUSDT", cfg)
	if ladder == nil {
		t.Fatalf("failed to create ladder strategy")
	}

	// Set balances to allow non-zero orders
	acc.UpdateBalance("BTC", decimal.NewFromFloat(1.0), decimal.NewFromFloat(1.0), time.Now().UTC())
	acc.UpdateBalance("USDT", decimal.NewFromFloat(100.0), decimal.NewFromFloat(100.0), time.Now().UTC())

	// BBO message: Bid 90, Ask 100
	bbo := models.BBO{
		Bid:       models.PriceLevel{Price: decimal.NewFromFloat(90.0), Size: decimal.NewFromFloat(1.0)},
		Ask:       models.PriceLevel{Price: decimal.NewFromFloat(100.0), Size: decimal.NewFromFloat(1.0)},
		Timestamp: time.Now().UTC(),
	}

	// Trigger BBO processing
	ladder.See(models.ExchangeMessage{
		Symbol:   "BTCUSDT",
		MsgType:  models.MsgTypeBBO,
		Payload:  bbo,
		Exchange: dummy.Name,
	})

	// Wait for async order placements to be processed by account/dummy exchange
	time.Sleep(100 * time.Millisecond)

	// Validate orders placed in the exchange
	openOrders, err := d.GetOpenOrders(ctx, "BTCUSDT")
	if err != nil {
		t.Fatalf("failed to get open orders from dummy exchange: %v", err)
	}

	// Expected placed orders:
	// Midprice = 95
	// Bid = 95 - 1% = 94.05
	// Ask = 95 + 1% = 95.95
	if len(openOrders) != 2 {
		t.Errorf("expected 2 orders on exchange, got %d", len(openOrders))
	} else {
		var hasBid, hasAsk bool
		for _, o := range openOrders {
			if o.Side == models.OrderSideBuy {
				hasBid = true
				if !o.Price.Equal(decimal.NewFromFloat(94.05)) {
					t.Errorf("expected bid price 94.05, got %s", o.Price)
				}
			} else if o.Side == models.OrderSideSell {
				hasAsk = true
				if !o.Price.Equal(decimal.NewFromFloat(95.95)) {
					t.Errorf("expected ask price 95.95, got %s", o.Price)
				}
			}
		}
		if !hasBid {
			t.Errorf("expected bid order to be placed")
		}
		if !hasAsk {
			t.Errorf("expected ask order to be placed")
		}
	}
}

func TestLadder_See_OrderLimitSafety(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	d := dummy.NewDummy(ctx, "key", "secret", "", "")
	d.Generator = func(ctx context.Context, d *dummy.Dummy, ch chan<- models.ExchangeMessage) {
		<-ctx.Done()
	}

	sym := models.SymbolInfo{
		Symbol:           "BTCUSDT",
		Base:             "BTC",
		Quote:            "USDT",
		PriceTickSize:    decimal.NewFromFloat(0.01),
		QuantityTickSize: decimal.NewFromFloat(0.0001),
		MinQuantity:      decimal.NewFromFloat(0.0001),
	}
	d.SetSymbol(sym)

	acc, err := account.NewAccount("test-acc", d)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	cfg := LadderConfig{
		PortfolioAllocation: decimal.NewFromFloat(0.5),
		LevelsCount:         1,
		LevelsSpread:        []decimal.Decimal{decimal.NewFromFloat(0.01)},
		LevelsSize:          []decimal.Decimal{decimal.NewFromFloat(1.0)},
		LevelsPriceTolerance: []decimal.Decimal{decimal.NewFromFloat(0.005)},
	}

	ladder := NewLadder(ctx, acc, "BTCUSDT", cfg)
	if ladder == nil {
		t.Fatalf("failed to create ladder strategy")
	}

	// Pre-populate 25 unique open orders in the exchange and account cache to exceed the limit of 20
	for i := 0; i < 25; i++ {
		o := models.Order{
			Symbol:          "BTCUSDT",
			ExchangeOrderID: fmt.Sprintf("order-%d", i),
			ClientOrderID:   fmt.Sprintf("client-order-%d", i),
			Side:            models.OrderSideBuy,
			Price:           decimal.NewFromFloat(80.0),
			Size:            decimal.NewFromFloat(1.0),
		}
		d.SetOrder(o)
		acc.UpdateOrder(o)
	}

	// Populate balances to allow non-zero order sizing
	acc.UpdateBalance("BTC", decimal.NewFromFloat(1.0), decimal.NewFromFloat(1.0), time.Now().UTC())
	acc.UpdateBalance("USDT", decimal.NewFromFloat(100.0), decimal.NewFromFloat(100.0), time.Now().UTC())

	bbo := models.BBO{
		Bid:       models.PriceLevel{Price: decimal.NewFromFloat(90.0), Size: decimal.NewFromFloat(1.0)},
		Ask:       models.PriceLevel{Price: decimal.NewFromFloat(100.0), Size: decimal.NewFromFloat(1.0)},
		Timestamp: time.Now().UTC(),
	}

	// Trigger BBO message. It should cancel the excessive orders.
	ladder.See(models.ExchangeMessage{
		Symbol:   "BTCUSDT",
		MsgType:  models.MsgTypeBBO,
		Payload:  bbo,
		Exchange: dummy.Name,
	})

	time.Sleep(100 * time.Millisecond)

	openOrders, err := d.GetOpenOrders(ctx, "BTCUSDT")
	if err != nil {
		t.Fatalf("failed to query open orders: %v", err)
	}

	// Since they were cleared, it should have replaced them with only the 2 new desired orders.
	if len(openOrders) > 2 {
		t.Errorf("expected all excessive orders to be cancelled and replaced, but got %d open orders", len(openOrders))
	}
}
