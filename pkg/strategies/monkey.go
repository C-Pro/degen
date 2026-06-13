package strategies

import (
	"context"
	"log"
	"strings"

	"degen/pkg/account"
	"degen/pkg/models"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Monkey is a simplest market maker that follows
// the current midprice and places orders with defined spread.
type Monkey struct {
	orderNotional decimal.Decimal
	symbol        models.SymbolInfo
	acc           *account.Account
	spread        decimal.Decimal
	tolerance     decimal.Decimal
}

func NewMonkey(
	ctx context.Context,
	acc *account.Account,
	symbol string,
	orderNotional decimal.Decimal,
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

	log.Printf("Symbol %s", s.Symbol)
	log.Printf("PriceTick: %s", s.PriceTickSize.String())
	log.Printf("SizeTick: %s", s.QuantityTickSize.String())
	if err := acc.CancelAllOrders(ctx, symbol); err != nil {
		log.Printf("failed to cancel all orders: %v\n", err)
		return nil
	}

	m := &Monkey{
		acc:           acc,
		symbol:        s,
		spread:        spread,
		orderNotional: orderNotional,
		tolerance:     spread.Mul(decimal.NewFromFloat(0.3)),
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

func (m *Monkey) calcOrderSize(
	notional decimal.Decimal,
	bbo decimal.Decimal,
) decimal.Decimal {
	size := roundUp(notional.Div(bbo), m.symbol.QuantityTickSize)
	if size.LessThan(m.symbol.MinQuantity) {
		return m.symbol.MinQuantity
	}

	return size
}

func (m *Monkey) withinTolerance(a, b decimal.Decimal) bool {
	return a.Sub(b).Abs().LessThan(a.Mul(m.tolerance))
}

func (m *Monkey) See(e models.ExchangeMessage) {
	if e.Symbol != m.symbol.Symbol {
		return
	}

	orders := m.acc.GetOpenOrders(m.symbol.Symbol)

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
