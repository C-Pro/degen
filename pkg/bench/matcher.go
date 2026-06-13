package bench

import (
	"time"

	"degen/pkg/account"
	"degen/pkg/connectors/dummy"
	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

// matcher is a minimal maker-fill engine. Any resting limit order whose price is
// crossed by the incoming BBO is filled in full at its own (maker) price. It
// mirrors the bookkeeping a live account would receive from the exchange over
// websockets: the terminal order is written back to both the dummy and the
// account, the position is updated, and balances are debited/credited net of a
// maker fee. This reproduces the proven fill model used by the ladder
// integration tests, generalised over symbol, assets and fee.
//
// Note: like that integration harness, locally placed orders are never fed back
// as "placed" websocket confirmations, so the account's open-interest
// aggregates (GetTotalBidSize/AskSize) stay zero and the ladder's bid/ask
// spread-penalty rebalancing is inert here. That feature only skews quoting and
// does not affect realised fills, so headline PnL is unaffected.
type matcher struct {
	d        *dummy.Dummy
	acc      *account.Account
	symbol   string
	base     string
	quote    string
	makerFee decimal.Decimal
	fills    int
}

// match fills every resting order crossed by bbo. It iterates a snapshot of the
// open orders, so orders filled within this call are not revisited.
func (m *matcher) match(bbo models.BBO) {
	for _, o := range m.acc.GetOpenOrders(m.symbol) {
		var fill bool
		switch o.Side {
		case models.OrderSideBuy:
			// A resting bid fills when the market ask drops to or below it.
			fill = !bbo.Ask.Price.IsZero() && bbo.Ask.Price.LessThanOrEqual(o.Price)
		case models.OrderSideSell:
			// A resting ask fills when the market bid rises to or above it.
			fill = !bbo.Bid.Price.IsZero() && bbo.Bid.Price.GreaterThanOrEqual(o.Price)
		}
		if fill {
			m.fillOrder(o)
		}
	}
}

// fillOrder marks a single order fully filled at its limit price and applies the
// resulting position, balance and fee changes to the account.
//
// It models a spot venue: a fill is skipped (the order is left resting, to be
// re-quoted or cancelled by the strategy as its balance changes) when the
// account does not hold enough quote to pay for a buy or enough base to deliver
// a sell. Without this guard, strategies that size every resting level against
// the full balance would over-fill into a net-short / net-borrowed position
// (phantom leverage) and bias the PnL tails. Because balances are updated after
// each fill, several same-tick fills draw down the balance in sequence and the
// guard naturally caps cumulative exposure.
func (m *matcher) fillOrder(o models.Order) {
	notional := o.Price.Mul(o.Size)
	fee := notional.Mul(m.makerFee)
	base := m.acc.GetBalance(m.base)
	quote := m.acc.GetBalance(m.quote)

	switch o.Side {
	case models.OrderSideBuy:
		if quote.Total.LessThan(notional.Add(fee)) {
			return
		}
	case models.OrderSideSell:
		if base.Total.LessThan(o.Size) {
			return
		}
	}

	now := time.Now().UTC()

	filled := o
	filled.FilledSize = o.Size
	filled.AveragePrice = o.Price
	filled.Status = models.OrderStatusFilled
	filled.Final = true
	filled.UpdatedAt = now

	m.d.SetOrder(filled)
	m.acc.UpdateOrder(filled)

	signed := o.Size
	if o.Side == models.OrderSideSell {
		signed = signed.Neg()
	}
	m.acc.UpdatePosition(m.symbol, signed, o.Price, now)

	if o.Side == models.OrderSideBuy {
		m.acc.UpdateBalance(m.base, base.Total.Add(o.Size), decimal.Zero, now)
		m.acc.UpdateBalance(m.quote, quote.Total.Sub(notional).Sub(fee), decimal.Zero, now)
	} else {
		m.acc.UpdateBalance(m.base, base.Total.Sub(o.Size), decimal.Zero, now)
		m.acc.UpdateBalance(m.quote, quote.Total.Add(notional).Sub(fee), decimal.Zero, now)
	}

	m.fills++
}
