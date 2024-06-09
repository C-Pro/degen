package strategies

import (
	"context"
	"log"

	"degen/pkg/models"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Monkey is a simplest market maker that follows
// the current midprice and places orders with definded spread.
type Monkey struct {
	orderSize        decimal.Decimal
	symbol           models.SymbolInfo
	prevBid, prevAsk models.PriceLevel
	acc              *models.Account
	pnl              decimal.Decimal
	spread           decimal.Decimal
}

func NewMonkey(
	ctx context.Context,
	acc *models.Account,
	symbol string,
	orderSize decimal.Decimal,
	spread decimal.Decimal,
) *Monkey {
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

	m := &Monkey{
		acc:       acc,
		symbol:    s,
		orderSize: orderSize,
	}

	return m
}

var two = decimal.NewFromInt(2)

func roundUp(v, tick decimal.Decimal) decimal.Decimal {
	return v.Div(tick).Ceil().Mul(tick)
}

func roundDown(v, tick decimal.Decimal) decimal.Decimal {
	return v.Div(tick).Floor().Mul(tick)
}

func (m *Monkey) See(e models.ExchangeMessage) {
	if e.Symbol != m.symbol.Symbol {
		return
	}

	orders := m.acc.GetOrders(m.symbol.Symbol)
	if len(orders) != 0 && len(orders) != 2 {
		log.Printf("unexpected number of orders: %d\n", len(orders))
		return
	}

	var bid, ask *models.Order
	for _, o := range orders {
		if o.Side == models.OrderSideBuy {
			bid = &o
		} else {
			ask = &o
		}
	}

	switch e.MsgType {
	case models.MsgTypeBBO:
		bbo := e.Payload.(models.BBO)
		if bbo.Bid.Price.IsZero() || bbo.Ask.Price.IsZero() {
			log.Println("bbo contains zeros")
			return
		}
		midprice := bbo.Bid.Price.Add(bbo.Ask.Price).Div(two)
		desiredBid := roundDown(midprice.Sub(m.spread.Div(two)), m.symbol.PriceTickSize)
		desiredAsk := roundUp(midprice.Add(m.spread.Div(two)), m.symbol.PriceTickSize)

		// TODO: Makes sense to use batch commands.
		// toCancel := make([]models.Order, 0, 2)
		// toPlace := make([]models.Order, 0, 2)
		if bid != nil && !bid.Price.Equal(desiredBid) {
			_, err := m.acc.CancelOrder(context.Background(), *bid)
			if err != nil {
				log.Printf("failed to cancel bid: %v\n", err)
			}
		}

		if !(bid != nil && bid.Price.Equal(desiredBid)) {
			_, err := m.acc.PlaceOrder(context.Background(), models.Order{
				Symbol:        m.symbol.Symbol,
				Side:          models.OrderSideBuy,
				Type:          models.OrderTypeLimit,
				PostOnly:      true,
				Price:         desiredBid,
				Size:          m.orderSize,
				ClientOrderID: uuid.NewString(),
			})
			if err != nil {
				log.Printf("failed to place bid: %v\n", err)
			}
		}

		if ask != nil && !ask.Price.Equal(desiredAsk) {
			_, err := m.acc.CancelOrder(context.Background(), *ask)
			if err != nil {
				log.Printf("failed to cancel ask: %v\n", err)
			}
		}

		if !(ask != nil && ask.Price.Equal(desiredAsk)) {
			_, err := m.acc.PlaceOrder(context.Background(), models.Order{
				Symbol:        m.symbol.Symbol,
				Side:          models.OrderSideSell,
				Type:          models.OrderTypeLimit,
				PostOnly:      true,
				Price:         desiredAsk,
				Size:          m.orderSize,
				ClientOrderID: uuid.NewString(),
			})
			if err != nil {
				log.Printf("failed to place ask: %v\n", err)
			}
		}
	}
}
