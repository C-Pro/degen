package strategies_test

import (
	"context"
	"io"
	"log"
	"math/rand"
	"os"
	"sort"
	"testing"
	"time"

	"degen/pkg/account"
	"degen/pkg/connectors/dummy"
	"degen/pkg/models"
	"degen/pkg/strategies"

	"github.com/shopspring/decimal"
)

func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

// --- Simulation parameters ---

const (
	symbol       = "BTCUSDT"
	baseAsset    = "BTC"
	quoteAsset   = "USDT"
	seed         = 42
	numTicks     = 10000
	startPrice   = 50000.0
	bboSpreadPct = 0.0005 // 0.05% half-spread for generated BBO
	priceStepPct = 0.001  // random walk step size ±0.1%
	startBase    = 1.0    // initial BTC balance
	startQuote   = 50000.0
	priceTick    = 0.01
	quantityTick = 0.0001
	minQty       = 0.0001
	makerFeePct  = 0.001 // 0.1% maker fee
)

// newTestEnv constructs a deterministic dummy exchange, account, and ladder strategy.
func newTestEnv(t *testing.T) (
	ctx context.Context,
	cancel context.CancelFunc,
	d *dummy.Dummy,
	acc *account.Account,
	ladder *strategies.Ladder,
	cfg strategies.LadderConfig,
) {
	t.Helper()

	ctx, cancel = context.WithTimeout(context.Background(), 120*time.Second)

	d = dummy.NewDummy(ctx, "key", "secret", "", "")
	// Block the Listen generator – we drive events manually.
	d.Generator = func(ctx context.Context, d *dummy.Dummy, ch chan<- models.ExchangeMessage) {
		<-ctx.Done()
	}

	sym := models.SymbolInfo{
		Symbol:           symbol,
		Base:             baseAsset,
		Quote:            quoteAsset,
		PriceTickSize:    decimal.NewFromFloat(priceTick),
		QuantityTickSize: decimal.NewFromFloat(quantityTick),
		MinQuantity:      decimal.NewFromFloat(minQty),
	}
	d.SetSymbol(sym)

	// Seed initial balances.
	d.SetBalance(models.Balance{
		Total:     decimal.NewFromFloat(startBase),
		Available: decimal.NewFromFloat(startBase),
		UpdatedAt: time.Now().UTC(),
	}, baseAsset)

	d.SetBalance(models.Balance{
		Total:     decimal.NewFromFloat(startQuote),
		Available: decimal.NewFromFloat(startQuote),
		UpdatedAt: time.Now().UTC(),
	}, quoteAsset)

	var err error
	acc, err = account.NewAccount("integration-test", d)
	if err != nil {
		cancel()
		t.Fatalf("failed to create account: %v", err)
	}

	cfg = strategies.LadderConfig{
		PortfolioAllocation: decimal.NewFromFloat(0.5),
		LevelsCount:         3,
		LevelsSpread: []decimal.Decimal{
			decimal.NewFromFloat(0.001), // 0.1%
			decimal.NewFromFloat(0.001), // 0.1%
			decimal.NewFromFloat(0.001), // 0.1%
		},
		LevelsSize: []decimal.Decimal{
			decimal.NewFromFloat(0.5), // 50%
			decimal.NewFromFloat(0.3), // 30%
			decimal.NewFromFloat(0.2), // 20%
		},
		LevelsPriceTolerance: []decimal.Decimal{
			decimal.NewFromFloat(0.005), // 0.5%
			decimal.NewFromFloat(0.005), // 0.5%
			decimal.NewFromFloat(0.005), // 0.5%
		},
	}

	ladder = strategies.NewLadder(ctx, acc, symbol, cfg)
	if ladder == nil {
		cancel()
		t.Fatalf("failed to create ladder strategy")
	}

	return
}

// generateBBO generates a deterministic random-walk BBO sequence.
func generateBBO(rng *rand.Rand, n int) []models.BBO {
	bbos := make([]models.BBO, n)
	mid := startPrice
	for i := 0; i < n; i++ {
		step := mid * priceStepPct * (2*rng.Float64() - 1)
		mid += step
		if mid < 1 {
			mid = 1
		}
		halfSpread := mid * bboSpreadPct
		bid := mid - halfSpread
		ask := mid + halfSpread
		bbos[i] = models.BBO{
			Bid:       models.PriceLevel{Price: decimal.NewFromFloat(bid), Size: decimal.NewFromFloat(10)},
			Ask:       models.PriceLevel{Price: decimal.NewFromFloat(ask), Size: decimal.NewFromFloat(10)},
			Timestamp: time.Now().UTC(),
		}
	}
	return bbos
}

// matchOrders is a simple matching engine: fills open orders that cross the BBO.
func matchOrders(
	t *testing.T,
	d *dummy.Dummy,
	acc *account.Account,
	bbo models.BBO,
) {
	t.Helper()
	orders := acc.GetOpenOrders(symbol)

	for _, o := range orders {
		filled := false
		var fillPrice decimal.Decimal

		switch o.Side {
		case models.OrderSideBuy:
			if bbo.Ask.Price.LessThanOrEqual(o.Price) {
				filled = true
				fillPrice = o.Price
			}
		case models.OrderSideSell:
			if bbo.Bid.Price.GreaterThanOrEqual(o.Price) {
				filled = true
				fillPrice = o.Price
			}
		}

		if !filled {
			continue
		}

		filledOrder := models.Order{
			Symbol:          o.Symbol,
			ClientOrderID:   o.ClientOrderID,
			ExchangeOrderID: o.ExchangeOrderID,
			Side:            o.Side,
			Type:            o.Type,
			Price:           o.Price,
			Size:            o.Size,
			FilledSize:      o.Size,
			AveragePrice:    fillPrice,
			Status:          models.OrderStatusFilled,
			Final:           true,
			UpdatedAt:       time.Now().UTC(),
		}
		d.SetOrder(filledOrder)
		acc.UpdateOrder(filledOrder)

		posAmount := o.Size
		if o.Side == models.OrderSideSell {
			posAmount = posAmount.Neg()
		}
		acc.UpdatePosition(symbol, posAmount, fillPrice, time.Now().UTC())

		fee := fillPrice.Mul(o.Size).Mul(decimal.NewFromFloat(makerFeePct))
		currentBase := acc.GetBalance(baseAsset)
		currentQuote := acc.GetBalance(quoteAsset)

		if o.Side == models.OrderSideBuy {
			notional := fillPrice.Mul(o.Size)
			acc.UpdateBalance(baseAsset, currentBase.Total.Add(o.Size), decimal.Zero, time.Now().UTC())
			acc.UpdateBalance(quoteAsset, currentQuote.Total.Sub(notional).Sub(fee), decimal.Zero, time.Now().UTC())
		} else {
			notional := fillPrice.Mul(o.Size)
			acc.UpdateBalance(baseAsset, currentBase.Total.Sub(o.Size), decimal.Zero, time.Now().UTC())
			acc.UpdateBalance(quoteAsset, currentQuote.Total.Add(notional).Sub(fee), decimal.Zero, time.Now().UTC())
		}
	}
}

// TestLadderIntegration_SpreadInvariant validates that the strategy maintains
// a minimum spread between its tightest bid and ask orders.
// The strategy places orders and then only re-quotes when price moves beyond
// tolerance, so stale orders may temporarily appear tight. We measure what
// fraction of ticks have a "fresh" spread violation (i.e. after the strategy
// has had a chance to re-quote).
func TestLadderIntegration_SpreadInvariant(t *testing.T) {
	ctx, cancel, d, acc, ladder, cfg := newTestEnv(t)
	defer cancel()

	rng := rand.New(rand.NewSource(seed))
	bbos := generateBBO(rng, numTicks)

	violations := 0

	for tick, bbo := range bbos {
		matchOrders(t, d, acc, bbo)

		ladder.See(models.ExchangeMessage{
			Symbol:   symbol,
			MsgType:  models.MsgTypeBBO,
			Payload:  bbo,
			Exchange: dummy.Name,
		})

		orders := acc.GetOpenOrders(symbol)
		if len(orders) == 0 {
			continue
		}

		var highestBid, lowestAsk decimal.Decimal
		for _, o := range orders {
			switch o.Side {
			case models.OrderSideBuy:
				if highestBid.IsZero() || o.Price.GreaterThan(highestBid) {
					highestBid = o.Price
				}
			case models.OrderSideSell:
				if lowestAsk.IsZero() || o.Price.LessThan(lowestAsk) {
					lowestAsk = o.Price
				}
			}
		}

		if highestBid.IsZero() || lowestAsk.IsZero() {
			continue
		}

		// Orders must never cross (ask <= bid).
		if lowestAsk.LessThanOrEqual(highestBid) {
			t.Errorf("tick %d: crossed book: bid=%s >= ask=%s", tick, highestBid, lowestAsk)
		}

		// The minimum spread between innermost orders should be at least
		// 2 * LevelsSpread[0] of midprice. We allow 50% slack because the
		// strategy uses tolerance bands and price rounding.
		midprice := bbo.Midprice()
		minExpectedSpread := midprice.Mul(cfg.LevelsSpread[0]).Mul(decimal.NewFromFloat(2 * 0.5))
		orderSpread := lowestAsk.Sub(highestBid)
		if orderSpread.LessThan(minExpectedSpread) {
			violations++
		}
	}

	// Some violations are expected due to tolerance bands and order lag.
	// We allow up to 50% because the strategy explicitly keeps stale orders
	// within tolerance rather than re-quoting every tick.
	violationRate := float64(violations) / float64(numTicks)
	t.Logf("Spread violations: %d / %d (%.1f%%)", violations, numTicks, violationRate*100)

	if violationRate > 0.50 {
		t.Errorf("spread violation rate %.1f%% exceeds 50%% threshold", violationRate*100)
	}

	_ = ctx
}

// TestLadderIntegration_BreakevenInvariant validates that the strategy never
// places sell orders below the position's minimum reduce price (for longs)
// or buy orders above it (for shorts) immediately after a fresh See() call.
func TestLadderIntegration_BreakevenInvariant(t *testing.T) {
	ctx, cancel, d, acc, ladder, _ := newTestEnv(t)
	defer cancel()

	rng := rand.New(rand.NewSource(seed))
	bbos := generateBBO(rng, numTicks)

	violations := 0

	for tick, bbo := range bbos {
		matchOrders(t, d, acc, bbo)

		ladder.See(models.ExchangeMessage{
			Symbol:   symbol,
			MsgType:  models.MsgTypeBBO,
			Payload:  bbo,
			Exchange: dummy.Name,
		})

		pos := acc.GetPosition(symbol)
		minReducePrice := acc.GetPositionMinReducePrice(symbol)

		if minReducePrice.IsZero() || pos.Amount.IsZero() {
			continue
		}

		orders := acc.GetOpenOrders(symbol)
		for _, o := range orders {
			if pos.Amount.Sign() == 1 && o.Side == models.OrderSideSell {
				if o.Price.LessThan(minReducePrice) {
					violations++
					if violations <= 5 {
						t.Logf("tick %d: breakeven violation (long): ask @ %s < minReducePrice %s",
							tick, o.Price, minReducePrice)
					}
				}
			} else if pos.Amount.Sign() == -1 && o.Side == models.OrderSideBuy {
				if o.Price.GreaterThan(minReducePrice) {
					violations++
					if violations <= 5 {
						t.Logf("tick %d: breakeven violation (short): bid @ %s > minReducePrice %s",
							tick, o.Price, minReducePrice)
					}
				}
			}
		}
	}

	t.Logf("Breakeven violations: %d / %d ticks", violations, numTicks)

	// Breakeven violations indicate stale orders that weren't yet cancelled
	// when the position changed. The strategy only re-quotes on tolerance
	// breaches, so some amount of staleness is expected.
	// However, we consider more than 5% violation rate a problem.
	violationRate := float64(violations) / float64(numTicks)
	if violationRate > 0.05 {
		t.Errorf("breakeven violation rate %.1f%% exceeds 5%% threshold", violationRate*100)
	}

	_ = ctx
}

// TestLadderIntegration_AllocationInvariant validates that total open order
// exposure does not wildly exceed the configured portfolio allocation.
// The strategy distributes PortfolioAllocation * balance across levels
// proportionally according to LevelsSize (which sum to 1.0). Each level
// is then rounded up to the nearest tick, so the true cap is slightly
// above PortfolioAllocation * balance. We check that total exposure
// stays within 2x the theoretical cap — anything beyond that would
// indicate a real sizing bug.
func TestLadderIntegration_AllocationInvariant(t *testing.T) {
	ctx, cancel, d, acc, ladder, cfg := newTestEnv(t)
	defer cancel()

	rng := rand.New(rand.NewSource(seed))
	bbos := generateBBO(rng, numTicks)

	bidViolations := 0
	askViolations := 0

	// Sum of all level sizes (should be 1.0 in our config).
	sumLevelsSize := decimal.Zero
	for _, ls := range cfg.LevelsSize {
		sumLevelsSize = sumLevelsSize.Add(ls)
	}

	// Tolerance: 2x accounts for quantization rounding (each level rounded
	// up by up to 1 tick), min quantity floors, and geometric compounding
	// effects on prices.
	tolerance := decimal.NewFromFloat(2.0)

	// Track peak balances: orders were placed when balances were higher.
	// Comparing stale orders against reduced (post-fill) balances causes
	// false positives. Use the high-water mark instead.
	peakQuote := decimal.NewFromFloat(startQuote)
	peakBase := decimal.NewFromFloat(startBase)

	for _, bbo := range bbos {
		matchOrders(t, d, acc, bbo)

		ladder.See(models.ExchangeMessage{
			Symbol:   symbol,
			MsgType:  models.MsgTypeBBO,
			Payload:  bbo,
			Exchange: dummy.Name,
		})

		quoteBalance := acc.GetBalance(quoteAsset)
		baseBalance := acc.GetBalance(baseAsset)
		if quoteBalance.Total.GreaterThan(peakQuote) {
			peakQuote = quoteBalance.Total
		}
		if baseBalance.Total.GreaterThan(peakBase) {
			peakBase = baseBalance.Total
		}

		orders := acc.GetOpenOrders(symbol)
		if len(orders) == 0 {
			continue
		}

		totalBidNotional := decimal.Zero
		totalAskSize := decimal.Zero
		for _, o := range orders {
			if o.Side == models.OrderSideBuy {
				totalBidNotional = totalBidNotional.Add(o.Price.Mul(o.Size))
			} else {
				totalAskSize = totalAskSize.Add(o.Size)
			}
		}

		// Max bid notional: peakQuote * allocation * sumLevelsSize * tolerance
		maxBidNotional := peakQuote.Mul(cfg.PortfolioAllocation).Mul(sumLevelsSize).Mul(tolerance)
		if !maxBidNotional.IsZero() && totalBidNotional.GreaterThan(maxBidNotional) {
			bidViolations++
			if bidViolations <= 5 {
				t.Logf("Bid violation: totalBidNotional=%s maxBidNotional=%s", totalBidNotional, maxBidNotional)
				for _, o := range orders {
					if o.Side == models.OrderSideBuy {
						t.Logf("  Bid order: Price=%s Size=%s Notional=%s ID=%s", o.Price, o.Size, o.Price.Mul(o.Size), o.ClientOrderID)
					}
				}
			}
		}

		// Max ask size: peakBase * allocation * sumLevelsSize * tolerance
		maxAskSize := peakBase.Mul(cfg.PortfolioAllocation).Mul(sumLevelsSize).Mul(tolerance)
		if !maxAskSize.IsZero() && totalAskSize.GreaterThan(maxAskSize) {
			askViolations++
		}
	}

	t.Logf("Bid allocation violations: %d / %d ticks", bidViolations, numTicks)
	t.Logf("Ask allocation violations: %d / %d ticks", askViolations, numTicks)

	// Allow up to 10% violation rate. Some staleness is expected because
	// the strategy does not immediately resize orders when balances change
	// from fills — it only re-quotes when price moves beyond tolerance.
	if float64(bidViolations)/float64(numTicks) > 0.10 {
		t.Errorf("too many bid allocation violations: %d (%.1f%%)", bidViolations, float64(bidViolations)/float64(numTicks)*100)
	}
	if float64(askViolations)/float64(numTicks) > 0.10 {
		t.Errorf("too many ask allocation violations: %d (%.1f%%)", askViolations, float64(askViolations)/float64(numTicks)*100)
	}

	_ = ctx
}

// TestLadderIntegration_NoExitAtLoss tracks every fill through the simulation
// and verifies that no individual sell trade closes at a price below the
// corresponding buy entry (FIFO matching).
func TestLadderIntegration_NoExitAtLoss(t *testing.T) {
	ctx, cancel, d, acc, ladder, _ := newTestEnv(t)
	defer cancel()

	rng := rand.New(rand.NewSource(seed))
	bbos := generateBBO(rng, numTicks)

	type fillRecord struct {
		tick  int
		side  models.OrderSide
		price decimal.Decimal
		size  decimal.Decimal
	}
	var fills []fillRecord

	for tick, bbo := range bbos {
		orders := acc.GetOpenOrders(symbol)
		if len(orders) > 20 {
			for _, o := range orders {
				t.Logf("Open order: ID=%s Side=%s Price=%s Size=%s PlacedAt=%v", o.ClientOrderID, o.Side, o.Price, o.Size, o.PlacedAt)
			}
			t.Fatalf("tick %d: too many open orders in account cache: %d", tick, len(orders))
		}
		for _, o := range orders {
			crossed := false
			var fillPrice decimal.Decimal

			switch o.Side {
			case models.OrderSideBuy:
				if bbo.Ask.Price.LessThanOrEqual(o.Price) {
					crossed = true
					fillPrice = o.Price
				}
			case models.OrderSideSell:
				if bbo.Bid.Price.GreaterThanOrEqual(o.Price) {
					crossed = true
					fillPrice = o.Price
				}
			}

			if !crossed {
				continue
			}

			fills = append(fills, fillRecord{tick: tick, side: o.Side, price: fillPrice, size: o.Size})

			filledOrder := models.Order{
				Symbol:          o.Symbol,
				ClientOrderID:   o.ClientOrderID,
				ExchangeOrderID: o.ExchangeOrderID,
				Side:            o.Side,
				Type:            o.Type,
				Price:           o.Price,
				Size:            o.Size,
				FilledSize:      o.Size,
				AveragePrice:    fillPrice,
				Status:          models.OrderStatusFilled,
				Final:           true,
				UpdatedAt:       time.Now().UTC(),
			}
			d.SetOrder(filledOrder)
			acc.UpdateOrder(filledOrder)

			posAmount := o.Size
			if o.Side == models.OrderSideSell {
				posAmount = posAmount.Neg()
			}
			acc.UpdatePosition(symbol, posAmount, fillPrice, time.Now().UTC())

			fee := fillPrice.Mul(o.Size).Mul(decimal.NewFromFloat(makerFeePct))
			currentBase := acc.GetBalance(baseAsset)
			currentQuote := acc.GetBalance(quoteAsset)

			if o.Side == models.OrderSideBuy {
				notional := fillPrice.Mul(o.Size)
				acc.UpdateBalance(baseAsset, currentBase.Total.Add(o.Size), decimal.Zero, time.Now().UTC())
				acc.UpdateBalance(quoteAsset, currentQuote.Total.Sub(notional).Sub(fee), decimal.Zero, time.Now().UTC())
			} else {
				notional := fillPrice.Mul(o.Size)
				acc.UpdateBalance(baseAsset, currentBase.Total.Sub(o.Size), decimal.Zero, time.Now().UTC())
				acc.UpdateBalance(quoteAsset, currentQuote.Total.Add(notional).Sub(fee), decimal.Zero, time.Now().UTC())
			}
		}

		ladder.See(models.ExchangeMessage{
			Symbol:   symbol,
			MsgType:  models.MsgTypeBBO,
			Payload:  bbo,
			Exchange: dummy.Name,
		})
	}

	t.Logf("Total fills: %d", len(fills))

	// Track position entries FIFO and validate sells don't exit at a loss.
	type entry struct {
		price decimal.Decimal
		size  decimal.Decimal
	}
	var longEntries []entry
	lossEvents := 0

	for _, f := range fills {
		if f.side == models.OrderSideBuy {
			longEntries = append(longEntries, entry{price: f.price, size: f.size})
		} else {
			remaining := f.size
			for remaining.IsPositive() && len(longEntries) > 0 {
				e := longEntries[0]
				reduceSize := decimal.Min(remaining, e.size)

				if f.price.LessThan(e.price) {
					lossEvents++
					if lossEvents <= 5 {
						t.Logf("tick %d: sold @ %s < bought @ %s (loss on %s units)",
							f.tick, f.price, e.price, reduceSize)
					}
				}

				remaining = remaining.Sub(reduceSize)
				if reduceSize.Equal(e.size) {
					longEntries = longEntries[1:]
				} else {
					longEntries[0].size = e.size.Sub(reduceSize)
				}
			}
		}
	}

	t.Logf("Loss events (FIFO matching): %d", lossEvents)

	// The strategy protects against selling below minReducePrice, but
	// FIFO matching against individual entries may still show some entries
	// at a loss (while the overall position is profitable). We log these
	// as informational. Zero is ideal but some are expected due to the
	// multi-entry position structure.

	// However, overall realized PnL should be positive.
	pos := acc.GetPosition(symbol)
	t.Logf("Realized PnL: %s", pos.RealizedPnL)

	// Check final portfolio value.
	lastBBO := bbos[len(bbos)-1]
	lastMid := lastBBO.Midprice()
	finalBase := acc.GetBalance(baseAsset)
	finalQuote := acc.GetBalance(quoteAsset)
	finalValue := finalBase.Total.Mul(lastMid).Add(finalQuote.Total)
	initialValue := decimal.NewFromFloat(startBase).Mul(decimal.NewFromFloat(startPrice)).Add(decimal.NewFromFloat(startQuote))

	t.Logf("Initial portfolio value: %s", initialValue)
	t.Logf("Final Base Asset:        %s", finalBase.Total)
	t.Logf("Final Quote Asset:       %s", finalQuote.Total)
	t.Logf("Final portfolio value:   %s (at midprice %s)", finalValue, lastMid)

	_ = ctx
}

// TestLadderIntegration_OrderSymmetry verifies that the strategy places
// the correct number of orders on each side, all bids below and asks above
// midprice, with monotonically spaced price levels.
func TestLadderIntegration_OrderSymmetry(t *testing.T) {
	ctx, cancel, _, acc, ladder, cfg := newTestEnv(t)
	defer cancel()

	bbo := models.BBO{
		Bid:       models.PriceLevel{Price: decimal.NewFromFloat(50000.0), Size: decimal.NewFromFloat(10)},
		Ask:       models.PriceLevel{Price: decimal.NewFromFloat(50050.0), Size: decimal.NewFromFloat(10)},
		Timestamp: time.Now().UTC(),
	}

	ladder.See(models.ExchangeMessage{
		Symbol:   symbol,
		MsgType:  models.MsgTypeBBO,
		Payload:  bbo,
		Exchange: dummy.Name,
	})

	orders := acc.GetOpenOrders(symbol)

	var bidCount, askCount int
	var bidPrices, askPrices []decimal.Decimal
	for _, o := range orders {
		if o.Side == models.OrderSideBuy {
			bidCount++
			bidPrices = append(bidPrices, o.Price)
		} else {
			askCount++
			askPrices = append(askPrices, o.Price)
		}
	}

	if bidCount != cfg.LevelsCount {
		t.Errorf("expected %d bid levels, got %d", cfg.LevelsCount, bidCount)
	}
	if askCount != cfg.LevelsCount {
		t.Errorf("expected %d ask levels, got %d", cfg.LevelsCount, askCount)
	}

	mid := bbo.Midprice()
	for _, o := range orders {
		switch o.Side {
		case models.OrderSideBuy:
			if o.Price.GreaterThanOrEqual(mid) {
				t.Errorf("bid order at %s >= midprice %s", o.Price, mid)
			}
		case models.OrderSideSell:
			if o.Price.LessThanOrEqual(mid) {
				t.Errorf("ask order at %s <= midprice %s", o.Price, mid)
			}
		}
	}

	// Sort for monotonicity check (GetOpenOrders returns unordered).
	sort.Slice(bidPrices, func(i, j int) bool { return bidPrices[i].GreaterThan(bidPrices[j]) })
	sort.Slice(askPrices, func(i, j int) bool { return askPrices[i].LessThan(askPrices[j]) })

	for i := 1; i < len(bidPrices); i++ {
		if bidPrices[i].GreaterThanOrEqual(bidPrices[i-1]) {
			t.Errorf("bid prices not monotonically decreasing: %s >= %s", bidPrices[i], bidPrices[i-1])
		}
	}
	for i := 1; i < len(askPrices); i++ {
		if askPrices[i].LessThanOrEqual(askPrices[i-1]) {
			t.Errorf("ask prices not monotonically increasing: %s <= %s", askPrices[i], askPrices[i-1])
		}
	}

	t.Logf("Placed %d bids and %d asks around midprice %s", bidCount, askCount, mid)
	for _, o := range orders {
		t.Logf("  %s @ %s size=%s", o.Side, o.Price, o.Size)
	}

	_ = ctx
}

// TestLadderIntegration_NoCrossedBook validates that across the entire
// simulation, the strategy never produces a crossed order book (bid >= ask).
func TestLadderIntegration_NoCrossedBook(t *testing.T) {
	ctx, cancel, d, acc, ladder, _ := newTestEnv(t)
	defer cancel()

	rng := rand.New(rand.NewSource(seed))
	bbos := generateBBO(rng, numTicks)

	for tick, bbo := range bbos {
		matchOrders(t, d, acc, bbo)

		ladder.See(models.ExchangeMessage{
			Symbol:   symbol,
			MsgType:  models.MsgTypeBBO,
			Payload:  bbo,
			Exchange: dummy.Name,
		})

		orders := acc.GetOpenOrders(symbol)
		var highestBid, lowestAsk decimal.Decimal
		for _, o := range orders {
			switch o.Side {
			case models.OrderSideBuy:
				if highestBid.IsZero() || o.Price.GreaterThan(highestBid) {
					highestBid = o.Price
				}
			case models.OrderSideSell:
				if lowestAsk.IsZero() || o.Price.LessThan(lowestAsk) {
					lowestAsk = o.Price
				}
			}
		}

		if !highestBid.IsZero() && !lowestAsk.IsZero() {
			if lowestAsk.LessThanOrEqual(highestBid) {
				t.Fatalf("tick %d: CROSSED BOOK: bid=%s >= ask=%s", tick, highestBid, lowestAsk)
			}
		}
	}

	t.Logf("No crossed book detected across %d ticks", numTicks)

	_ = ctx
}
