package strategies

import (
	"context"
	"log"
	"math"
	"strings"

	"degen/pkg/account"
	"degen/pkg/models"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type entry struct {
	prev float64
	next float64
	size float64
}

type positionStructure struct {
	long      bool
	sizes     map[float64]entry
	head      float64
	totalSize float64
	avgPrice  float64
}

func (p *positionStructure) less(a float64, b float64) bool {
	if p.long {
		return a < b
	}

	return a > b
}

func (p *positionStructure) add(price float64, size float64) {
	if size == 0 {
		return
	}

	if p.sizes == nil {
		p.sizes = make(map[float64]entry)
		p.long = size > 0
	} else {
		// If position side is opposite to the incoming trade, position should be reduced.
		if p.long != (size > 0) {
			p.reduce(price, size)
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

	p.totalSize += size
	p.avgPrice = (p.avgPrice*(p.totalSize-size) + price*size) / p.totalSize
}

// reduce removes size from the position.
// It removes up to size liquidity from entries, starting from the head.
// If size is more than the total size it will create a position of
// the opposite side with the remaining size.
func (p *positionStructure) reduce(price float64, size float64) {
	if size == 0 {
		return
	}

	if p.sizes == nil {
		panic("reducing empty position")
	}

	// Size here will have the opposite sign to the size of the position.
	var next float64
	for curr := p.head; curr != 0 && size != 0; curr = next {
		e := p.sizes[curr]
		next = e.next
		// If the entry is smaller than the size, remove it.
		if math.Abs(e.size) <= math.Abs(size) {
			size += e.size // decreasing absolute value of size.
			delete(p.sizes, curr)
			switch {
			case p.totalSize-e.size == 0:
				p.avgPrice = 0
			default:
				p.avgPrice = (p.avgPrice*p.totalSize - curr*e.size) / (p.totalSize - e.size)
			}
			p.totalSize -= e.size
			if e.prev == 0 {
				p.head = e.next
			} else {
				prevEntry := p.sizes[e.prev]
				prevEntry.next = e.next
				p.sizes[e.prev] = prevEntry
			}
		} else { // If the entry is larger than the size, reduce it.
			p.avgPrice = (p.avgPrice*p.totalSize + curr*size) / (p.totalSize + size)
			p.totalSize += size
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
		p.add(price, size)
	}
}

// Ladder is a market maker that follows
// the current midprice and places orders with defined spread.
type Ladder struct {
	orderNotional decimal.Decimal
	symbol        models.SymbolInfo
	acc           *account.Account
	spread        decimal.Decimal
	tolerance     decimal.Decimal
}

func NewLadder(
	ctx context.Context,
	acc *account.Account,
	symbol string,
	orderNotional decimal.Decimal,
	spread decimal.Decimal,
) *Ladder {
	symbols, err := acc.GetSymbols(ctx)
	if err != nil {
		log.Printf("failed to get symbols: %v\n", err)
		return nil
	}
	s, ok := symbols[symbol]
	if !ok {
		log.Printf("symbol %s not found\n", symbol)
		return nil
	}

	log.Printf("Symbol %s", s.Symbol)
	log.Printf("PriceTick: %s", s.PriceTickSize.String())
	log.Printf("SizeTick: %s", s.QuantityTickSize.String())
	if err := acc.CancelAllOrders(ctx, symbol); err != nil {
		log.Printf("failed to cancel all orders: %v\n", err)
		return nil
	}

	m := &Ladder{
		acc:           acc,
		symbol:        s,
		spread:        spread,
		orderNotional: orderNotional,
		tolerance:     spread.Mul(decimal.NewFromFloat(0.3)),
	}

	return m
}

func (m *Ladder) calcOrderSize(
	notional decimal.Decimal,
	bbo decimal.Decimal,
) decimal.Decimal {
	size := roundUp(notional.Div(bbo), m.symbol.QuantityTickSize)
	if size.LessThan(m.symbol.MinQuantity) {
		return m.symbol.MinQuantity
	}

	return size
}

func (m *Ladder) withinTolerance(a, b decimal.Decimal) bool {
	return a.Sub(b).Abs().LessThan(a.Mul(m.tolerance))
}

func (m *Ladder) See(e models.ExchangeMessage) {
	if e.Symbol != m.symbol.Symbol {
		return
	}

	orders := m.acc.GetOrders(m.symbol.Symbol)

	var bids, asks []models.Order
	for _, o := range orders {
		if o.Side == models.OrderSideBuy {
			bids = append(bids, o)
		} else {
			asks = append(asks, o)
		}
	}

	// Hack to prevent too many orders.
	if len(orders) > 4 {
		if err := m.acc.SyncWithExchange(context.Background(), []string{m.symbol.Symbol}); err != nil {
			log.Printf("failed to sync with exchange: %v\n", err)
		}
		if err := m.acc.CancelAllOrders(context.Background(), m.symbol.Symbol); err != nil {
			log.Printf("failed to cancel all orders: %v\n", err)
		} else {
			bids = nil
			asks = nil
		}
	}

	switch e.MsgType {
	case models.MsgTypeBBO:
		bbo := e.Payload.(models.BBO)
		if bbo.Bid.Price.IsZero() && bbo.Ask.Price.IsZero() {
			log.Println("no BBO")
			return
		}

		midprice := bbo.Bid.Price.Add(bbo.Ask.Price).Div(two)
		if bbo.Bid.Price.IsZero() || bbo.Ask.Price.IsZero() {
			midprice = bbo.Bid.Price.Add(bbo.Ask.Price)
		}

		position := m.acc.GetPosition(m.symbol.Symbol)
		desiredBid := roundDown(midprice.Sub(midprice.Mul(m.spread.Div(two))), m.symbol.PriceTickSize)
		desiredAsk := roundUp(midprice.Add(midprice.Mul(m.spread.Div(two))), m.symbol.PriceTickSize)
		switch {
		case position.Amount.Sign() == 1:
			// We are long. Don't want to close at lower price.
			avgPrice := position.AveragePrice
			desiredAsk = roundUp(avgPrice.Add(avgPrice.Mul(m.spread.Div(two))), m.symbol.PriceTickSize)
		case position.Amount.Sign() == -1:
			// We are short. Don't want to close at larger price.
			avgPrice := position.AveragePrice
			desiredBid = roundDown(avgPrice.Sub(avgPrice.Mul(m.spread.Div(two))), m.symbol.PriceTickSize)
		}

		// TODO: Makes sense to use batch commands.
		// toCancel := make([]models.Order, 0, 2)
		// toPlace := make([]models.Order, 0, 2)
		hasDesired := false
		for _, bid := range bids {
			if !m.withinTolerance(bid.Price, desiredBid) {
				_, err := m.acc.CancelOrder(context.Background(), bid)
				if err != nil && !strings.Contains(err.Error(), "ORDER_NOT_FOUND") {
					log.Printf("failed to cancel bid: %v\n", err)
				}
			} else {
				hasDesired = true
			}
		}

		if !hasDesired {
			_, err := m.acc.PlaceOrder(context.Background(), models.Order{
				Symbol:        m.symbol.Symbol,
				Side:          models.OrderSideBuy,
				Type:          models.OrderTypeLimit,
				PostOnly:      true,
				Price:         desiredBid,
				Size:          m.calcOrderSize(m.orderNotional, bbo.Ask.Price),
				ClientOrderID: uuid.NewString(),
			})
			if err != nil {
				log.Printf("failed to place bid: %v\n", err)
			}
		}

		hasDesired = false
		for _, ask := range asks {
			if !m.withinTolerance(ask.Price, desiredAsk) {
				_, err := m.acc.CancelOrder(context.Background(), ask)
				if err != nil && !strings.Contains(err.Error(), "ORDER_NOT_FOUND") {
					log.Printf("failed to cancel ask: %v\n", err)
				}
			} else {
				hasDesired = true
			}
		}

		if !hasDesired {
			_, err := m.acc.PlaceOrder(context.Background(), models.Order{
				Symbol:        m.symbol.Symbol,
				Side:          models.OrderSideSell,
				Type:          models.OrderTypeLimit,
				PostOnly:      true,
				Price:         desiredAsk,
				Size:          m.calcOrderSize(m.orderNotional, bbo.Bid.Price),
				ClientOrderID: uuid.NewString(),
			})
			if err != nil {
				log.Printf("failed to place ask: %v\n", err)
			}
		}
	}
}
