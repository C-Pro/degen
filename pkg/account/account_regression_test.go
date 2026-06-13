package account

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"degen/pkg/connectors/dummy"
	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

func newTestAccount(t *testing.T) *Account {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	d := dummy.NewDummy(ctx, "key", "secret", "", "")
	d.Generator = func(ctx context.Context, _ *dummy.Dummy, _ chan<- models.ExchangeMessage) {
		<-ctx.Done()
	}
	acc, err := NewAccount("test-acc", d)
	if err != nil {
		t.Fatalf("NewAccount: %v", err)
	}
	return acc
}

// Regression for H5: a placed order adds to open interest; the matching Final
// fill must subtract it again, instead of being deleted before observe (which
// leaked the open size forever).
func TestAccount_OpenInterestNoLeakOnFinalFill(t *testing.T) {
	acc := newTestAccount(t)
	symbol := "BTCUSDT"

	placed := models.Order{
		Symbol:        symbol,
		ClientOrderID: "bid1",
		Price:         decimal.NewFromFloat(40000),
		Size:          decimal.NewFromFloat(1.0),
		Side:          models.OrderSideBuy,
		Status:        models.OrderStatusPlaced,
	}
	acc.UpdateOrder(placed)
	if !acc.GetTotalBidSize(symbol).Equal(decimal.NewFromFloat(1.0)) {
		t.Fatalf("after place: total bid size = %s, want 1.0", acc.GetTotalBidSize(symbol))
	}

	filled := placed
	filled.Status = models.OrderStatusFilled
	filled.FilledSize = placed.Size
	filled.Final = true
	acc.UpdateOrder(filled)

	if !acc.GetTotalBidSize(symbol).IsZero() {
		t.Errorf("after fill: total bid size = %s, want 0 (open size must be subtracted)", acc.GetTotalBidSize(symbol))
	}
	if got := acc.GetOpenOrders(symbol); len(got) != 0 {
		t.Errorf("after fill: open orders = %d, want 0", len(got))
	}
}

func TestAccount_GetOpenInterestSnapshot(t *testing.T) {
	acc := newTestAccount(t)
	symbol := "BTCUSDT"
	acc.UpdateOrder(models.Order{Symbol: symbol, ClientOrderID: "b", Price: decimal.NewFromFloat(100), Size: decimal.NewFromFloat(2), Side: models.OrderSideBuy, Status: models.OrderStatusPlaced})
	acc.UpdateOrder(models.Order{Symbol: symbol, ClientOrderID: "a", Price: decimal.NewFromFloat(110), Size: decimal.NewFromFloat(4), Side: models.OrderSideSell, Status: models.OrderStatusPlaced})

	snap := acc.GetOpenInterest(symbol)
	if !snap.TotalBidSize.Equal(decimal.NewFromFloat(2)) || !snap.TotalAskSize.Equal(decimal.NewFromFloat(4)) {
		t.Errorf("snapshot sizes = %s/%s, want 2/4", snap.TotalBidSize, snap.TotalAskSize)
	}
	if !snap.AvgBidPrice.Equal(decimal.NewFromFloat(100)) || !snap.AvgAskPrice.Equal(decimal.NewFromFloat(110)) {
		t.Errorf("snapshot avg = %s/%s, want 100/110", snap.AvgBidPrice, snap.AvgAskPrice)
	}

	// Empty side must be zero, not a divide-by-zero panic.
	empty := acc.GetOpenInterest("NOSUCH")
	if !empty.AvgBidPrice.IsZero() || !empty.AvgAskPrice.IsZero() {
		t.Errorf("empty snapshot avg = %s/%s, want 0/0", empty.AvgBidPrice, empty.AvgAskPrice)
	}
}

// Regression for M4: CancelAllOrders must purge local tracking immediately so
// the order count and open interest reflect reality without waiting for async
// websocket cancel confirmations.
func TestAccount_CancelAllOrdersPurgesLocalState(t *testing.T) {
	acc := newTestAccount(t)
	symbol := "BTCUSDT"

	acc.UpdateOrder(models.Order{Symbol: symbol, ClientOrderID: "b1", ExchangeOrderID: "x1", Price: decimal.NewFromFloat(100), Size: decimal.NewFromFloat(2), Side: models.OrderSideBuy, Status: models.OrderStatusPlaced})
	acc.UpdateOrder(models.Order{Symbol: symbol, ClientOrderID: "a1", ExchangeOrderID: "x2", Price: decimal.NewFromFloat(110), Size: decimal.NewFromFloat(3), Side: models.OrderSideSell, Status: models.OrderStatusPlaced})
	if len(acc.GetOpenOrders(symbol)) != 2 {
		t.Fatalf("setup: expected 2 open orders")
	}

	if err := acc.CancelAllOrders(context.Background(), symbol); err != nil {
		t.Fatalf("CancelAllOrders: %v", err)
	}

	if got := acc.GetOpenOrders(symbol); len(got) != 0 {
		t.Errorf("after CancelAllOrders: open orders = %d, want 0", len(got))
	}
	if !acc.GetTotalBidSize(symbol).IsZero() || !acc.GetTotalAskSize(symbol).IsZero() {
		t.Errorf("after CancelAllOrders: open interest not cleared (bid=%s ask=%s)",
			acc.GetTotalBidSize(symbol), acc.GetTotalAskSize(symbol))
	}
}

// Regression for H2: open interest is mutated (UpdateOrder) and read
// (GetTotalBidSize/GetOpenInterest) from multiple goroutines concurrently.
// Must be race-free under `go test -race`.
func TestAccount_ConcurrentOpenInterestNoRace(t *testing.T) {
	acc := newTestAccount(t)
	symbol := "BTCUSDT"

	const writers = 4
	const iters = 300
	var writersWg, readersWg sync.WaitGroup

	// Writers: each cycles place -> fill on its own client order id.
	for w := 0; w < writers; w++ {
		writersWg.Add(1)
		go func(w int) {
			defer writersWg.Done()
			id := fmt.Sprintf("o-%d", w)
			for i := 0; i < iters; i++ {
				acc.UpdateOrder(models.Order{
					Symbol: symbol, ClientOrderID: id,
					Price: decimal.NewFromFloat(40000), Size: decimal.NewFromFloat(1),
					Side: models.OrderSideBuy, Status: models.OrderStatusPlaced,
				})
				acc.UpdateOrder(models.Order{
					Symbol: symbol, ClientOrderID: id,
					Price: decimal.NewFromFloat(40000), Size: decimal.NewFromFloat(1),
					FilledSize: decimal.NewFromFloat(1),
					Side:       models.OrderSideBuy, Status: models.OrderStatusFilled, Final: true,
				})
			}
		}(w)
	}

	// Readers run until the writers are done.
	stop := make(chan struct{})
	for r := 0; r < 3; r++ {
		readersWg.Add(1)
		go func() {
			defer readersWg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = acc.GetTotalBidSize(symbol)
					_ = acc.GetOpenInterest(symbol)
				}
			}
		}()
	}

	writersWg.Wait()
	close(stop)
	readersWg.Wait()
}
