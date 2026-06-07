package strategies

import (
	"context"
	"errors"
	"log"
	"strings"

	"degen/pkg/account"
	"degen/pkg/models"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type LadderConfig struct {
	// Max portion of portfolio to be allocated.
	PortfolioAllocation decimal.Decimal
	// Number of levels on each side of the midprice.
	LevelsCount int
	// List of spreads between previous level and the current.
	// For level 0 it is a spread from BBO.
	LevelsSpread []decimal.Decimal
	// Relative size of the level. Sum(LevelsSize)==1.
	LevelsSize []decimal.Decimal
	// How much the spread can move before order is canceled and placed with a new price.
	LevelsPriceTolerance []decimal.Decimal
}

func (l *LadderConfig) Validate() error {
	if l.PortfolioAllocation.IsNegative() {
		return errors.New("portfolio allocation must be non-negative")
	}
	if l.LevelsCount <= 0 {
		return errors.New("levels count must be positive")
	}
	if len(l.LevelsSpread) != l.LevelsCount {
		return errors.New("levels spread must have the same length as levels count")
	}
	if len(l.LevelsSize) != l.LevelsCount {
		return errors.New("levels size must have the same length as levels count")
	}
	if len(l.LevelsPriceTolerance) != l.LevelsCount {
		return errors.New("levels price tolerance must have the same length as levels count")
	}
	if l.PortfolioAllocation.GreaterThan(decimal.NewFromInt(1)) {
		return errors.New("portfolio allocation must be less than or equal to 1")
	}

	return nil
}

type DesiredOrders struct {
	Bids [][2]decimal.Decimal // [price, size]
	Asks [][2]decimal.Decimal // [price, size]
}

func (l *LadderConfig) IdealAllocation(
	midprice decimal.Decimal,
	baseTotal decimal.Decimal,
	quoteTotal decimal.Decimal,
	bidPenalty decimal.Decimal,
	askPenalty decimal.Decimal,
) DesiredOrders {
	orders := DesiredOrders{
		Bids: make([][2]decimal.Decimal, 0, l.LevelsCount),
		Asks: make([][2]decimal.Decimal, 0, l.LevelsCount),
	}
	if midprice.IsZero() || l.PortfolioAllocation.IsZero() {
		return orders
	}

	// Bids go from midprice down.
	price := midprice
	for i := 0; i < l.LevelsCount; i++ {
		price = price.Sub(price.Mul(l.LevelsSpread[i].Add(bidPenalty)))
		size := quoteTotal.Mul(l.PortfolioAllocation).Mul(l.LevelsSize[i]).Div(price)
		if size.IsPositive() {
			orders.Bids = append(orders.Bids, [2]decimal.Decimal{price, size})
		}
	}

	price = midprice
	// Asks go from midprice up.
	for i := 0; i < l.LevelsCount; i++ {
		price = price.Add(price.Mul(l.LevelsSpread[i].Add(askPenalty)))
		size := baseTotal.Mul(l.PortfolioAllocation).Mul(l.LevelsSize[i])
		if size.IsPositive() {
			orders.Asks = append(orders.Asks, [2]decimal.Decimal{price, size})
		}
	}

	return orders
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
) (bids, asks []models.Order) {
	if bbo.Bid.Price.IsZero() && bbo.Ask.Price.IsZero() {
		return nil, nil
	}

	baseBalance := m.acc.GetBalance(m.symbol.Base)
	quoteBalance := m.acc.GetBalance(m.symbol.Quote)

	bidPenalty := m.bidSpreadPenalty()
	askPenalty := m.askSpreadPenalty()

	ideal := m.cfg.IdealAllocation(bbo.Midprice(), baseBalance.Total, quoteBalance.Total, bidPenalty, askPenalty)

	pos := m.acc.GetPosition(m.symbol.Symbol)
	minReducePrice := m.acc.GetPositionMinReducePrice(m.symbol.Symbol)
	switch pos.Amount.Sign() {
	case 1:
		// We are long. Don't want to close at lower price.
		// Adjust asks to be above min reduce price.
		if ideal.Asks[0][0].LessThan(minReducePrice) {
			diff := minReducePrice.Sub(ideal.Asks[0][0])
			for i := range ideal.Asks {
				ideal.Asks[i][0] = ideal.Asks[i][0].Add(diff)
			}
		}
	case -1:
		// We are short. Don't want to close at larger price.
		// Adjust bids to be below min reduce price.
		if ideal.Bids[0][0].GreaterThan(minReducePrice) {
			diff := ideal.Bids[0][0].Sub(minReducePrice)
			for i := range ideal.Bids {
				ideal.Bids[i][0] = ideal.Bids[i][0].Sub(diff)
			}
		}
	}

	for _, b := range ideal.Bids {
		bids = append(bids, models.Order{
			Symbol:        m.symbol.Symbol,
			Side:          models.OrderSideBuy,
			Type:          models.OrderTypeLimit,
			PostOnly:      true,
			Price:         roundDown(b[0], m.symbol.PriceTickSize),
			Size:          m.quantizeOrderSize(b[1]),
			ClientOrderID: uuid.NewString(),
		})
	}

	for _, a := range ideal.Asks {
		asks = append(asks, models.Order{
			Symbol:        m.symbol.Symbol,
			Side:          models.OrderSideSell,
			Type:          models.OrderTypeLimit,
			PostOnly:      true,
			Price:         roundUp(a[0], m.symbol.PriceTickSize),
			Size:          m.quantizeOrderSize(a[1]),
			ClientOrderID: uuid.NewString(),
		})
	}

	return bids, asks
}

func (m *Ladder) See(e models.ExchangeMessage) {
	if e.Symbol != m.symbol.Symbol {
		return
	}

	switch e.MsgType {
	case models.MsgTypeBBO:
		bbo := e.Payload.(models.BBO)
		if bbo.Bid.Price.IsZero() && bbo.Ask.Price.IsZero() {
			log.Println("no BBO")
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

		desiredBids, desiredAsks := m.GetDesiredOrders(bbo)

		// Match open bids to desired bids 1-to-1 (priority to best match)
		matchedBids := make(map[string]bool)
		dbToBid := make([]*models.Order, len(desiredBids))
		for i, db := range desiredBids {
			tol := m.cfg.LevelsPriceTolerance[i]
			var bestMatch *models.Order
			var bestDiff decimal.Decimal
			for j := range bids {
				bid := &bids[j]
				if matchedBids[bid.ClientOrderID] {
					continue
				}
				diff := bid.Price.Sub(db.Price).Abs()
				if diff.LessThan(db.Price.Mul(tol)) {
					if bestMatch == nil || diff.LessThan(bestDiff) {
						bestMatch = bid
						bestDiff = diff
					}
				}
			}
			if bestMatch != nil {
				matchedBids[bestMatch.ClientOrderID] = true
				dbToBid[i] = bestMatch
			}
		}

		// Cancel unmatched bids
		for _, bid := range bids {
			if !matchedBids[bid.ClientOrderID] {
				_, err := m.acc.CancelOrder(context.Background(), bid)
				if err != nil && !errors.Is(err, models.ErrOrderNotFound) && !strings.Contains(strings.ToUpper(err.Error()), "ORDER_NOT_FOUND") {
					log.Printf("failed to cancel bid: %v\n", err)
				}
			}
		}

		// Place new bids where matching failed
		for i, db := range desiredBids {
			if dbToBid[i] == nil && db.Price.IsPositive() {
				log.Printf("placing order %s", db)
				_, err := m.acc.PlaceOrder(context.Background(), db)
				if err != nil {
					log.Printf("failed to place bid: %v\n", err)
				}
			}
		}

		// Match open asks to desired asks 1-to-1 (priority to best match)
		matchedAsks := make(map[string]bool)
		daToAsk := make([]*models.Order, len(desiredAsks))
		for i, da := range desiredAsks {
			tol := m.cfg.LevelsPriceTolerance[i]
			var bestMatch *models.Order
			var bestDiff decimal.Decimal
			for j := range asks {
				ask := &asks[j]
				if matchedAsks[ask.ClientOrderID] {
					continue
				}
				diff := ask.Price.Sub(da.Price).Abs()
				if diff.LessThan(da.Price.Mul(tol)) {
					if bestMatch == nil || diff.LessThan(bestDiff) {
						bestMatch = ask
						bestDiff = diff
					}
				}
			}
			if bestMatch != nil {
				matchedAsks[bestMatch.ClientOrderID] = true
				daToAsk[i] = bestMatch
			}
		}

		// Cancel unmatched asks
		for _, ask := range asks {
			if !matchedAsks[ask.ClientOrderID] {
				_, err := m.acc.CancelOrder(context.Background(), ask)
				if err != nil && !errors.Is(err, models.ErrOrderNotFound) && !strings.Contains(strings.ToUpper(err.Error()), "ORDER_NOT_FOUND") {
					log.Printf("failed to cancel ask: %v\n", err)
				}
			}
		}

		// Place new asks where matching failed
		for i, da := range desiredAsks {
			if daToAsk[i] == nil && da.Price.IsPositive() {
				log.Printf("placing order %s", da)
				_, err := m.acc.PlaceOrder(context.Background(), da)
				if err != nil {
					log.Printf("failed to place ask: %v\n", err)
				}
			}
		}
	}
}
