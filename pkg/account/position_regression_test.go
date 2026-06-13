package account

import (
	"testing"
	"time"

	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

func pupd(amount, price float64) models.PositionUpdate {
	return models.PositionUpdate{
		Amount:    decimal.NewFromFloat(amount),
		Price:     decimal.NewFromFloat(price),
		Timestamp: time.Unix(0, 0),
	}
}

// Regression for C1: a middle insert (non-monotonic trade prices) followed by a
// reduce must not orphan the head or lose live size. The linked-list view
// (getReduceSize) must agree with the decimal totalSize.
func TestPosition_MiddleInsertThenReduce(t *testing.T) {
	p := &positionStructure{}
	p.Update(pupd(1, 100))
	p.Update(pupd(1, 102))
	p.Update(pupd(1, 101)) // inserted between 100 and 102
	p.Update(pupd(-2.5, 105))

	if !p.totalSize.Equal(decimal.NewFromFloat(0.5)) {
		t.Fatalf("totalSize = %s, want 0.5", p.totalSize)
	}
	// All 0.5 remaining was bought at 102 (the two cheapest lots were sold).
	// getReduceSize(price) sums lots cheaper than price; with a large price it
	// must see the whole remaining size.
	if got := p.getReduceSize(1e9); got != 0.5 {
		t.Errorf("getReduceSize(1e9) = %v, want 0.5 (remaining size must be visible)", got)
	}
	if len(p.levels) != 1 || p.levels[0].price != 102 {
		t.Errorf("levels = %+v, want a single level at 102", p.levels)
	}
}

// Regression for C2: closing-and-flipping a position must leave a clean
// structure (no stale zero-size levels, exactly the flipped remainder).
func TestPosition_FlipLeavesCleanStructure(t *testing.T) {
	p := &positionStructure{}
	p.Update(pupd(1, 100))
	p.Update(pupd(1, 101))  // long 2
	p.Update(pupd(-3, 105)) // sell 3 -> close long 2, flip to short 1 @105

	if p.long {
		t.Errorf("expected short after flip")
	}
	if !p.totalSize.Equal(decimal.NewFromFloat(-1)) {
		t.Errorf("totalSize = %s, want -1", p.totalSize)
	}
	if len(p.levels) != 1 {
		t.Fatalf("levels = %+v, want exactly 1 level after flip", p.levels)
	}
	if p.levels[0].price != 105 || p.levels[0].size != -1 {
		t.Errorf("level = %+v, want {price:105 size:-1}", p.levels[0])
	}
	for _, l := range p.levels {
		if l.size == 0 {
			t.Errorf("stale zero-size level present: %+v", l)
		}
	}
}

// Regression for M12: a price of 0 must not collide with any "empty" sentinel;
// it is tracked like any other level.
func TestPosition_ZeroPriceLevel(t *testing.T) {
	p := &positionStructure{}
	p.Update(pupd(1, 0)) // zero-price lot first
	p.Update(pupd(1, 100))

	if !p.totalSize.Equal(decimal.NewFromFloat(2)) {
		t.Errorf("totalSize = %s, want 2", p.totalSize)
	}
	// Both lots are cheaper than 1e9, so both must be visible.
	if got := p.getReduceSize(1e9); got != 2 {
		t.Errorf("getReduceSize(1e9) = %v, want 2 (zero-price lot must be visible)", got)
	}
	if len(p.levels) != 2 {
		t.Errorf("levels = %+v, want 2 levels", p.levels)
	}
}

// Regression for H7: minReducePrice is the lowest entry price (the conservative
// reduce-only bound) and, crucially, is no longer polluted by stale zero-size
// entries — which the old linked-list implementation left behind on flips and
// reduces (see C1/C2). Here we drive a flip and assert the bound reflects only
// the live remainder, not a ghost of the closed position.
func TestPosition_MinReducePriceIgnoresStale(t *testing.T) {
	long := &positionStructure{}
	long.Update(pupd(1, 100))
	long.Update(pupd(1, 110))
	if got := long.minReducePrice(); !got.Equal(decimal.NewFromFloat(100)) {
		t.Errorf("long minReducePrice = %s, want 100 (lowest entry)", got)
	}

	// Close the long and flip to a short at 130. The old structure would have
	// left a zero-size ghost at 100/110, making minReducePrice report 100.
	long.Update(pupd(-3, 130))
	if got := long.minReducePrice(); !got.Equal(decimal.NewFromFloat(130)) {
		t.Errorf("after flip minReducePrice = %s, want 130 (only the live short remains)", got)
	}
}
