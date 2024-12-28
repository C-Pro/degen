package account

import (
	"math"
	"time"

	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

// entry represents a single price entry in the position structure.
// float64 recision should be enough for single entry and we can
// have a lot of entries in large and old positions, so avoiding
// decimal.Decimal makes sense.
type entry struct {
	prev float64
	next float64
	size float64
}

func (e entry) Size() decimal.Decimal {
	return decimal.NewFromFloat(e.size)
}

// positionStructure stores position in a double linked list of
// entries. Each entry corresponds to specific price.
// This allows to understand exact price structure of the position,
// instead of treating it is as a big blob with average price.
type positionStructure struct {
	long        bool
	sizes       map[float64]entry
	head        float64
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

func (p *positionStructure) less(a float64, b float64) bool {
	if p.long {
		return a < b
	}

	return a > b
}

func (p *positionStructure) Update(upd models.PositionUpdate) {
	if upd.Amount.IsZero() {
		return
	}

	size := upd.Amount.InexactFloat64()
	price := upd.Price.InexactFloat64()

	if p.sizes == nil {
		p.sizes = make(map[float64]entry)
		p.long = size > 0
	} else {
		// If position side is opposite to the incoming trade, position should be reduced.
		if p.long != (size > 0) {
			p.reduce(upd)
			return
		}
	}

	// If entry with the same price exists, update it.
	if e, ok := p.sizes[price]; ok {
		e.size += size
		p.sizes[price] = e
	} else {
		// Find where to insert a new entry.
		switch {
		// Case 0: first entry.
		case p.head == 0:
			p.head = price
			p.sizes[price] = entry{
				size: size,
			}
		// Case 1: price is better than head, insert at the top.
		case p.less(price, p.head):
			p.sizes[price] = entry{
				next: p.head,
				size: size,
			}
			p.head = price
		// Default case: find first entry that is better than the incoming price.
		default:
			prev := float64(0)
			curr := p.head
			for !p.less(price, curr) {
				prev = curr
				curr = p.sizes[curr].next
				if curr == 0 {
					break
				}
			}
			// Insert new entry between prev and curr.
			p.sizes[price] = entry{
				prev: prev,
				next: curr,
				size: size,
			}
			// Update prev entry's next.
			prevEntry := p.sizes[prev]
			prevEntry.next = price
			p.sizes[prev] = prevEntry
		}
	}

	p.updatedAt = upd.Timestamp
	p.totalSize = p.totalSize.Add(upd.Amount)

	// p.avgPrice = (p.avgPrice*(p.totalSize-size) + price*size) / p.totalSize
	p.avgPrice = (p.avgPrice.Mul(p.totalSize.Sub(upd.Amount)).Add(upd.Price.Mul(upd.Amount))).Div(p.totalSize)
}

// reduce removes size from the position.
// It removes up to size liquidity from entries, starting from the head.
// If size is more than the total size it will create a position of
// the opposite side with the remaining size.
func (p *positionStructure) reduce(upd models.PositionUpdate) {
	if p.sizes == nil {
		panic("reducing empty position")
	}

	size := upd.Amount.InexactFloat64()

	// Size here will have the opposite sign to the size of the position.
	var next float64
	for curr := p.head; curr != 0 && size != 0; curr = next {
		e := p.sizes[curr]
		next = e.next
		// If the entry is smaller than the size, remove it.
		if math.Abs(e.size) <= math.Abs(size) {
			p.realizedPnL = p.realizedPnL.Add(upd.Price.Mul(e.Size())).Sub(decimal.NewFromFloat(curr).Mul(e.Size()))
			size += e.size // decreasing absolute value of size.
			delete(p.sizes, curr)
			switch {
			case p.totalSize.Sub(e.Size()).IsZero():
				p.avgPrice = decimal.Zero
			default:
				// p.avgPrice = (p.avgPrice*p.totalSize - curr*e.size) / (p.totalSize - e.size)
				p.avgPrice = p.avgPrice.Mul(p.totalSize).
					Sub(decimal.NewFromFloat(curr).Mul(e.Size())).
					Div(p.totalSize.Sub(e.Size()))
			}
			p.totalSize = p.totalSize.Sub(e.Size())
			if e.prev == 0 {
				p.head = e.next
			} else {
				prevEntry := p.sizes[e.prev]
				prevEntry.next = e.next
				p.sizes[e.prev] = prevEntry
			}
		} else { // If the entry is larger than the size, reduce it.
			p.realizedPnL = p.realizedPnL.Add(
				decimal.NewFromFloat(curr).Mul(decimal.NewFromFloat(size)).
					Sub(upd.Price.Mul(decimal.NewFromFloat(size))))
			// p.avgPrice = (p.avgPrice*p.totalSize + curr*size) / (p.totalSize + size)
			p.avgPrice = p.avgPrice.Mul(p.totalSize).
				Add(decimal.NewFromFloat(curr).Mul(decimal.NewFromFloat(size))).
				Div(p.totalSize.Add(decimal.NewFromFloat(size)))
			// p.totalSize += size
			p.totalSize = p.totalSize.Add(decimal.NewFromFloat(size))
			e.size += size
			p.sizes[curr] = e
			size = 0
			break
		}
	}

	// If there is still size left, open a new position in
	// the opposite direction.
	if size != 0 {
		p.long = !p.long
		upd.Amount = decimal.NewFromFloat(size)
		p.Update(upd)
	}
}

// getReduceSize returns the size that position can be reduced by
// given the expected execution price.
func (p *positionStructure) getReduceSize(price float64) float64 {
	if p.sizes == nil {
		return 0
	}

	size := 0.0
	for curr := p.head; curr != 0; curr = p.sizes[curr].next {
		if p.less(curr, price) {
			size += p.sizes[curr].size
			continue
		}
		break
	}

	return size
}

// getMinReducePrice returns the minimum price at which the position
// can be reduced by up to the given size.
func (p *positionStructure) getMinReducePrice(reqSize float64) (price, size float64) {
	if p.sizes == nil {
		return 0, 0
	}

	sizePrice := 0.0
	for curr := p.head; curr != 0 && reqSize != 0; curr = p.sizes[curr].next {
		reduceSize := math.Min(math.Abs(reqSize), math.Abs(p.sizes[curr].size))
		if p.long {
			reduceSize = -reduceSize
		}
		sizePrice += math.Abs(curr * reduceSize)
		reqSize -= math.Abs(reduceSize)
		size += math.Abs(reduceSize)
	}

	if size == 0 {
		return 0, 0
	}

	return sizePrice / size, size
}
