package strategies

import (
	"context"
	"testing"
	"time"

	"degen/pkg/account"
	"degen/pkg/connectors/dummy"
	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

// Regression for C4: holding a long position while the base balance is zero
// makes IdealAllocation produce an empty ask side. GetDesiredOrders must not
// panic indexing ideal.Asks[0] (which previously crashed the whole process).
func TestLadder_GetDesiredOrders_EmptyAskSideWithLongPosition(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	d := dummy.NewDummy(ctx, "key", "secret", "", "")
	d.Generator = func(ctx context.Context, _ *dummy.Dummy, _ chan<- models.ExchangeMessage) { <-ctx.Done() }
	d.SetSymbol(models.SymbolInfo{
		Symbol: "BTCUSDT", Base: "BTC", Quote: "USDT",
		PriceTickSize: decimal.NewFromFloat(0.01), QuantityTickSize: decimal.NewFromFloat(0.0001),
		MinQuantity: decimal.NewFromFloat(0.0001),
	})

	acc, err := account.NewAccount("test-acc", d)
	if err != nil {
		t.Fatalf("NewAccount: %v", err)
	}

	cfg := LadderConfig{
		PortfolioAllocation:  decimal.NewFromFloat(0.5),
		LevelsCount:          1,
		LevelsSpread:         []decimal.Decimal{decimal.NewFromFloat(0.01)},
		LevelsSize:           []decimal.Decimal{decimal.NewFromFloat(1.0)},
		LevelsPriceTolerance: []decimal.Decimal{decimal.NewFromFloat(0.005)},
	}
	ladder := NewLadder(ctx, acc, "BTCUSDT", cfg)
	if ladder == nil {
		t.Fatalf("NewLadder returned nil")
	}

	// Long position but ZERO base balance -> empty ask side. Quote balance set
	// so the bid side is non-empty.
	acc.UpdateBalance("USDT", decimal.NewFromFloat(1000), decimal.NewFromFloat(1000), time.Now().UTC())
	acc.UpdatePosition("BTCUSDT", decimal.NewFromFloat(1.0), decimal.NewFromFloat(99.0), time.Now().UTC())

	bbo := models.BBO{
		Bid:       models.PriceLevel{Price: decimal.NewFromFloat(95.0), Size: decimal.NewFromFloat(1.0)},
		Ask:       models.PriceLevel{Price: decimal.NewFromFloat(96.0), Size: decimal.NewFromFloat(1.0)},
		Timestamp: time.Now().UTC(),
	}

	// Must not panic.
	bids, asks := ladder.GetDesiredOrders(bbo)
	if len(asks) != 0 {
		t.Errorf("expected 0 asks with zero base balance, got %d", len(asks))
	}
	if len(bids) == 0 {
		t.Errorf("expected bids to still be produced from quote balance")
	}
}
