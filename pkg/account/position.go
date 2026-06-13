package account

import (
	"math"
	"sort"
	"time"

	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

// level represents the size of the position opened at a specific price.
// float64 precision should be enough for a single level and we can have
// a lot of levels in large and old positions, so avoiding decimal.Decimal
// here makes sense. Aggregate values (totalSize, avgPrice, realizedPnL) are
// kept in decimal.Decimal to avoid precision drift.
type level struct {
	price float64
	size  float64
}

// positionStructure stores the position as price levels kept sorted by price
// (ascending). This allows understanding the exact price structure of the
// position instead of treating it as a single blob with an average price.
//
// The "head" of the structure (the level reduced first) depends on direction:
// for a long position it is the lowest price, for a short position the highest
// price. Direction is encoded by less() and the traversal helpers below.
//
// Earlier versions used a hand-rolled doubly linked list keyed by float64
// price with 0 as the "no node" sentinel. That had three defects: middle
// inserts did not maintain back-pointers (orphaning the head and silently
// losing size), closing-and-flipping left dangling pointers and stale
// zero-size nodes, and a legitimate price of 0 collided with the sentinel.
// The sorted slice below avoids all three.
type positionStructure struct {
	long        bool
	levels      []level
	totalSize   decimal.Decimal
	avgPrice    decimal.Decimal
	updatedAt   time.Time
	realizedPnL decimal.Decimal
}

func (p *positionStructure) Position() models.Position {
	return models.Position{
		Amount:       p.totalSize,
		AveragePrice: p.avgPrice,
		UpdatedAt:    p.updatedAt,
		RealizedPnL:  p.realizedPnL,
	}
}

// less reports whether price a should be reduced before price b.
func (p *positionStructure) less(a float64, b float64) bool {
	if p.long {
		return a < b
	}

	return a > b
}

// headIndex returns the index into the sorted levels slice of the level that
// should be reduced first (lowest price for long, highest for short).
func (p *positionStructure) headIndex() int {
	if p.long {
		return 0
	}

	return len(p.levels) - 1
}

func (p *positionStructure) Update(upd models.PositionUpdate) {
	if upd.Amount.IsZero() {
		return
	}

	size := upd.Amount.InexactFloat64()
	price := upd.Price.InexactFloat64()

	switch {
	case len(p.levels) == 0:
		// First level (re)opens the position; derive the side from its sign.
		p.long = size > 0
	case p.long != (size > 0):
		// Incoming trade is opposite to the position side: reduce (and possibly flip).
		p.reduce(upd)
		return
	}

	// Same side as the position: merge into an existing level or insert a new one.
	if i, ok := p.find(price); ok {
		p.levels[i].size += size
	} else {
		p.insert(level{price: price, size: size})
	}

	p.updatedAt = upd.Timestamp
	p.totalSize = p.totalSize.Add(upd.Amount)

	// p.avgPrice = (p.avgPrice*(p.totalSize-size) + price*size) / p.totalSize
	p.avgPrice = (p.avgPrice.Mul(p.totalSize.Sub(upd.Amount)).Add(upd.Price.Mul(upd.Amount))).Div(p.totalSize)
}

// find returns the index of the level at the given price, if present.
func (p *positionStructure) find(price float64) (int, bool) {
	i := sort.Search(len(p.levels), func(i int) bool { return p.levels[i].price >= price })
	if i < len(p.levels) && p.levels[i].price == price {
		return i, true
	}

	return 0, false
}

// insert adds a new level keeping the slice sorted by price ascending.
func (p *positionStructure) insert(l level) {
	i := sort.Search(len(p.levels), func(i int) bool { return p.levels[i].price >= l.price })
	p.levels = append(p.levels, level{})
	copy(p.levels[i+1:], p.levels[i:])
	p.levels[i] = l
}

// removeHead removes the level at the given head index.
func (p *positionStructure) removeHead(idx int) {
	p.levels = append(p.levels[:idx], p.levels[idx+1:]...)
}

// reduce removes size from the position, starting from the head (the level we
// would close first). If the incoming size exceeds the whole position it closes
// it and opens a new position of the opposite side with the remaining size.
func (p *positionStructure) reduce(upd models.PositionUpdate) {
	// size here has the opposite sign to the size of the position.
	size := upd.Amount.InexactFloat64()

	for len(p.levels) > 0 && size != 0 {
		idx := p.headIndex()
		e := p.levels[idx]
		curr := e.price
		eSize := decimal.NewFromFloat(e.size)

		if math.Abs(e.size) <= math.Abs(size) {
			// The level is fully consumed.
			p.realizedPnL = p.realizedPnL.Add(upd.Price.Mul(eSize)).Sub(decimal.NewFromFloat(curr).Mul(eSize))
			size += e.size // decreasing absolute value of size.

			switch {
			case p.totalSize.Sub(eSize).IsZero():
				p.avgPrice = decimal.Zero
			default:
				// p.avgPrice = (p.avgPrice*p.totalSize - curr*e.size) / (p.totalSize - e.size)
				p.avgPrice = p.avgPrice.Mul(p.totalSize).
					Sub(decimal.NewFromFloat(curr).Mul(eSize)).
					Div(p.totalSize.Sub(eSize))
			}
			p.totalSize = p.totalSize.Sub(eSize)
			p.removeHead(idx)
		} else {
			// The level is larger than the remaining size: reduce it in place.
			sz := decimal.NewFromFloat(size)
			p.realizedPnL = p.realizedPnL.Add(
				decimal.NewFromFloat(curr).Mul(sz).
					Sub(upd.Price.Mul(sz)))
			// p.avgPrice = (p.avgPrice*p.totalSize + curr*size) / (p.totalSize + size)
			p.avgPrice = p.avgPrice.Mul(p.totalSize).
				Add(decimal.NewFromFloat(curr).Mul(sz)).
				Div(p.totalSize.Add(sz))
			p.totalSize = p.totalSize.Add(sz)
			e.size += size
			p.levels[idx] = e
			size = 0
			break
		}
	}

	p.updatedAt = upd.Timestamp

	// If there is still size left, the position was fully closed: open a new
	// position in the opposite direction with a clean structure. Update() will
	// re-derive the side from the leftover sign because levels is now empty.
	if size != 0 {
		p.levels = nil
		upd.Amount = decimal.NewFromFloat(size)
		p.Update(upd)
	}
}

// getReduceSize returns the size that the position can be reduced by given the
// expected execution price (sum of levels strictly "better" than price).
func (p *positionStructure) getReduceSize(price float64) float64 {
	size := 0.0
	for _, l := range p.levels {
		if p.less(l.price, price) {
			size += l.size
		}
	}

	return size
}

// getMinReducePrice returns the size-weighted average price at which the
// position can be reduced by up to the given size, walking from the head.
func (p *positionStructure) getMinReducePrice(reqSize float64) (price, size float64) {
	sizePrice := 0.0
	n := len(p.levels)
	for k := 0; k < n && reqSize != 0; k++ {
		idx := k
		if !p.long {
			idx = n - 1 - k
		}
		l := p.levels[idx]
		reduceSize := math.Min(math.Abs(reqSize), math.Abs(l.size))
		sizePrice += math.Abs(l.price * reduceSize)
		reqSize -= math.Abs(reduceSize)
		size += math.Abs(reduceSize)
	}

	if size == 0 {
		return 0, 0
	}

	return sizePrice / size, size
}

// minReducePrice returns the lowest entry price of the position (ignoring any
// zero-size levels). The strategy uses it as a reduce-only bound: a floor for
// asks when long and a ceiling for bids when short. For a short, the lowest
// entry is the conservative ceiling (buying back below the cheapest sale
// guarantees a profit on every lot). Returns zero for an empty position.
//
// This deliberately matches the historical behaviour (min over all entries),
// but operates on the corrected level structure so it can no longer be skewed
// by the stale zero-size entries the previous linked-list implementation left
// behind.
func (p *positionStructure) minReducePrice() decimal.Decimal {
	found := false
	min := 0.0
	for _, l := range p.levels {
		if l.size == 0 {
			continue
		}
		if !found || l.price < min {
			min = l.price
			found = true
		}
	}

	if !found {
		return decimal.Zero
	}

	return decimal.NewFromFloat(min)
}
