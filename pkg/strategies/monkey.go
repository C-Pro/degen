package strategies

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

const (
	patience = 1
	symbol   = "ethusdt"
	slippage = 0.005
)

type Monkey struct {
	ch               chan models.OrderSide
	cntUp, cntDown   int
	prevBid, prevAsk models.PriceLevel
	numEvents        uint32
	acc              *models.Account
}

func NewMonkey(
	ctx context.Context,
	acc *models.Account,
) *Monkey {
	m := &Monkey{
		ch:  make(chan models.OrderSide),
		acc: acc,
	}

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				atomic.StoreUint32(&m.numEvents, 0)
			case <-ctx.Done():
				return
			}
		}
	}()

	return m
}

func (m *Monkey) See(e models.ExchangeMessage) {
	pos := m.acc.GetPosition(symbol)
	switch e.MsgType {
	case models.MsgTypeBBO:
		bbo := e.Payload.(models.BBO)
		if !m.prevAsk.Price.IsZero() {
			if m.prevAsk.Price.GreaterThan(bbo.Ask.Price) {
				m.cntUp++
				log.Printf("^^^ %d\n", m.cntUp)

				// Closing short position if expected PnL > profitMargin.
				profitMargin := bbo.Ask.Price.Mul(decimal.NewFromFloat(slippage))
				if pos.Amount.IsNegative() &&
					pos.EntryPrice.GreaterThan(bbo.Ask.Price.Add(profitMargin)) {
					m.ch <- models.OrderSideBuy
					m.prevAsk = bbo.Ask
					return
				}
				if pos.Amount.IsPositive() {
					// We are already in position.
					m.prevAsk = bbo.Ask
					return
				}

				if m.cntUp > patience {
					// Opening long position if price is going up.
					if atomic.AddUint32(&m.numEvents, 1) < 4 {
						m.ch <- models.OrderSideBuy
					}
					m.cntUp = 0
				}
			} else if m.prevAsk.Price.LessThan(bbo.Ask.Price) {
				m.cntUp = 0
			}
		}

		m.prevAsk = bbo.Ask

		// Closing long position if expected PnL > profitMargin.
		if !m.prevBid.Price.IsZero() {
			if m.prevBid.Price.LessThan(bbo.Bid.Price) {
				m.cntDown++
				log.Printf("vvv %d\n", m.cntDown)

				profitMargin := bbo.Bid.Price.Mul(decimal.NewFromFloat(slippage))
				if pos.Amount.IsPositive() &&
					pos.EntryPrice.LessThan(bbo.Bid.Price.Sub(profitMargin)) {
					m.ch <- models.OrderSideSell
					m.prevBid = bbo.Bid
					return
				}
				if pos.Amount.IsNegative() {
					// We are already in position.
					m.prevBid = bbo.Bid
					return
				}

				if m.cntDown > patience {
					// Opening short position if price is going up.
					if atomic.AddUint32(&m.numEvents, 1) < 4 {
						m.ch <- models.OrderSideSell
					}
					m.cntDown = 0
				}
			} else if m.prevBid.Price.GreaterThan(bbo.Bid.Price) {
				m.cntDown = 0
			}
		}

		m.prevBid = bbo.Bid
	}
}

func (m *Monkey) Say() <-chan models.OrderSide {
	return m.ch
}
