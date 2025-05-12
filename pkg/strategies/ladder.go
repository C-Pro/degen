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

type LadderConfig struct {
	// Max portion of portfolio to be allocated.
	PortfolioAllocation  decimal.Decimal
	// List of spreads between previous level and the current.
	// For level 0 it is a spread from BBO.
	LevelsSpread         []decimal.Decimal
	// Relative size of the level. Sum(LevelsSize)==1.
	LevelsSize           []decimal.Decimal
	// How much the spread can move before order is canceled and placed with a new price.
	LevelsPriceTolerance []decimal.Decimal
}

// Ladder is a market maker that follows
// the current midprice and places orders with defined spread.
type Ladder struct {
	symbol models.SymbolInfo
	acc    *account.Account

	cfg LadderConfig
}

func NewLadder(
	ctx context.Context,
	acc *account.Account,
	symbol string,
	cfg LadderConfig,
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

	m := &Ladder{
		acc:    acc,
		symbol: s,
		cfg:    cfg,
	}

	log.Printf("Symbol %s", s.Symbol)
	log.Printf("PriceTick: %s", s.PriceTickSize.String())
	log.Printf("SizeTick: %s", s.QuantityTickSize.String())

	if err := acc.CancelAllOrders(ctx, symbol); err != nil {
		log.Printf("failed to cancel all orders: %v\n", err)
		return nil
	}

	return m
}

// cap spread penalty at 5%
const spreadPenaltyClamp = 0.05

// bidSpreadPenalty returns extra spread that should be added to the bid side
// midprice deviation if total bid size is higher than total ask size.
// The goal is to keep bid and ask sizes balanced.
func (m *Ladder) bidSpreadPenalty() decimal.Decimal {
	totalBidSize := m.acc.GetTotalBidSize(m.symbol.Symbol)
	totalAskSize := m.acc.GetTotalAskSize(m.symbol.Symbol)
	if totalBidSize.IsZero() {
		return decimal.Zero
	}

	// Will add 0.05% to the spread for each 1% of bid-heavy imbalance.
	imbalance := totalBidSize.Sub(totalAskSize).Div(totalBidSize)
	if imbalance.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}

	penalty := imbalance.Mul(decimal.NewFromFloat(0.05))
	if penalty.GreaterThan(decimal.NewFromFloat(spreadPenaltyClamp)) {
		return decimal.NewFromFloat(spreadPenaltyClamp)
	}

	return penalty
}

// askSpreadPenalty returns extra spread that should be added to the ask side
// midprice deviation if total ask size is higher than total bid size.
// The goal is to keep bid and ask sizes balanced.
func (m *Ladder) askSpreadPenalty() decimal.Decimal {
	totalBidSize := m.acc.GetTotalBidSize(m.symbol.Symbol)
	totalAskSize := m.acc.GetTotalAskSize(m.symbol.Symbol)
	if totalAskSize.IsZero() {
		return decimal.Zero
	}

	// Will add 0.05% to the spread for each 1% of ask-heavy imbalance.
	imbalance := totalAskSize.Sub(totalBidSize).Div(totalAskSize)
	if imbalance.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero
	}

	penalty := imbalance.Mul(decimal.NewFromFloat(0.05))
	if penalty.GreaterThan(decimal.NewFromFloat(spreadPenaltyClamp)) {
		return decimal.NewFromFloat(spreadPenaltyClamp)
	}

	return penalty
}

func (m *Ladder) calcOrderSize(
	notional decimal.Decimal,
	bbo decimal.Decimal,
) decimal.Decimal {
	return m.quantizeOrderSize(notional.Div(bbo))
}

func (m *Ladder) quantizeOrderSize(
	size decimal.Decimal,
) decimal.Decimal {
	q := roundUp(size, m.symbol.QuantityTickSize)
	if q.LessThan(m.symbol.MinQuantity) {
		return m.symbol.MinQuantity
	}

	return q
}


// GetDesiredOrders returns a list of desired bids and asks given the current BBO,
// current positions structure and settings like spead, order size, step between
// price levels and total desired fund allocation.
func (m *Ladder) GetDesiredOrders(
	bbo models.BBO,
	availableBase decimal.Decimal,
	availableQuote decimal.Decimal,
	noLossSellPrice decimal.Decimal,
	noLossBuyPrice decimal.Decimal,
	position models.Position,
	cfg LadderConfig,
	) (bids, asks []models.Order) {
	if bbo.Bid.Price.IsZero() && bbo.Ask.Price.IsZero() {
		return nil, nil
	}

	// midprice := bbo.Bid.Price.Add(bbo.Ask.Price).Div(decimal.NewFromInt(2))
	// if bbo.Bid.Price.IsZero() || bbo.Ask.Price.IsZero() {
	// 	midprice = bbo.Bid.Price.Add(bbo.Ask.Price)
	// }

	// Adjust prices based on position.
	var (
		noLossPrice float64
		availSize   float64
	)
	reqSize := m.calcOrderSize(m.orderNotional, bbo.Ask.Price)
	noLossPrice, availSize = m.ps.getMinReducePrice(reqSize.InexactFloat64())
	size := m.quantizeOrderSize(decimal.NewFromFloat(availSize))
	avgPrice := decimal.NewFromFloat(noLossPrice)
	switch {
	case position.Amount.Sign() == 1:
		// We are long. Don't want to close at lower price.
		desiredAsk = roundUp(avgPrice.Add(avgPrice.Mul(m.spread.Div(two))), m.symbol.PriceTickSize)
	case position.Amount.Sign() == -1:
		// We are short. Don't want to close at larger price.
		desiredBid = roundDown(avgPrice.Sub(avgPrice.Mul(m.spread.Div(two))), m.symbol.PriceTickSize)
	}
}

func (m *Ladder) See(e models.ExchangeMessage) {
	if e.Symbol != m.symbol.Symbol {
		return
	}

	switch e.MsgType {
	case models.MsgTypeOrderStatus:
		order := e.Payload.(models.Order)
		if err := m.oi.observe(order); err != nil {
			log.Printf("failed to observe order: %v\n", err)
		}
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
	if len(orders) > 20 {
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
		// Base prices based on spread from midprice.
		desiredBid := roundDown(midprice.Sub(midprice.Mul(m.spread.Div(two))), m.symbol.PriceTickSize)
		desiredAsk := roundUp(midprice.Add(midprice.Mul(m.spread.Div(two))), m.symbol.PriceTickSize)

		// Adjust prices based on position.
		var (
			noLossPrice float64
			availSize   float64
		)
		reqSize := m.calcOrderSize(m.orderNotional, bbo.Ask.Price)
		noLossPrice, availSize = m.ps.getMinReducePrice(reqSize.InexactFloat64())
		size := m.quantizeOrderSize(decimal.NewFromFloat(availSize))
		avgPrice := decimal.NewFromFloat(noLossPrice)
		switch {
		case position.Amount.Sign() == 1:
			// We are long. Don't want to close at lower price.
			desiredAsk = roundUp(avgPrice.Add(avgPrice.Mul(m.spread.Div(two))), m.symbol.PriceTickSize)
		case position.Amount.Sign() == -1:
			// We are short. Don't want to close at larger price.
			desiredBid = roundDown(avgPrice.Sub(avgPrice.Mul(m.spread.Div(two))), m.symbol.PriceTickSize)
		}

		// TODO: Makes sense to use batch commands.
		// toCancel := make([]models.Order, 0, 2)
		// toPlace := make([]models.Order, 0, 2)

		// See if we already have ok bid.
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

		// If not, place a new one.
		if !hasDesired && desiredBid.IsPositive() {
			order := models.Order{
				Symbol:        m.symbol.Symbol,
				Side:          models.OrderSideBuy,
				Type:          models.OrderTypeLimit,
				PostOnly:      true,
				Price:         desiredBid,
				Size:          size,
				ClientOrderID: uuid.NewString(),
			}

			log.Printf("placing order %s", order)
			_, err := m.acc.PlaceOrder(context.Background(), order)
			if err != nil {
				log.Printf("failed to place bid: %v\n", err)
			}
		}

		// Place ask.
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

		if !hasDesired && desiredAsk.IsPositive() {
			order := models.Order{
				Symbol:        m.symbol.Symbol,
				Side:          models.OrderSideSell,
				Type:          models.OrderTypeLimit,
				PostOnly:      true,
				Price:         desiredAsk,
				Size:          size,
				ClientOrderID: uuid.NewString(),
			}
			log.Printf("placing order %s", order)
			_, err := m.acc.PlaceOrder(context.Background(), order)
			if err != nil {
				log.Printf("failed to place ask: %v\n", err)
			}
		}
	}
}
