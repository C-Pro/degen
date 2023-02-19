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
	prevBid, prevAsk *models.PriceLevel
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

func getPayload(e models.ExchangeMessage) *models.PriceLevel {
	pl := &models.PriceLevel{}
	*pl = e.Payload.(models.PriceLevel)
	return pl
}

func (m *Monkey) See(e models.ExchangeMessage) {
	pos := m.acc.GetPosition(symbol)

	switch e.MsgType {
	case models.MsgTypeTopAsk:
		tick := getPayload(e)
		// Closing short position if expected PnL > profitMargin.
		profitMargin := tick.Price.Mul(decimal.NewFromFloat(slippage))
		if pos.Amount.IsNegative() &&
			pos.EntryPrice.GreaterThan(tick.Price.Add(profitMargin)) {
			m.ch <- models.OrderSideBuy
			return
		}
		if pos.Amount.IsPositive() {
			// We are already in position.
			return
		}
		if m.prevAsk != nil {
			if m.prevAsk.Price.GreaterThan(tick.Price) {
				m.cntUp++
				log.Printf("^^^ %d\n", m.cntUp)
				if m.cntUp > patience {
					// Opening long position if price is going up.
					if atomic.AddUint32(&m.numEvents, 1) < 4 {
						m.ch <- models.OrderSideBuy
					}
					m.cntUp = 0
				}
			} else if m.prevAsk.Price.LessThan(tick.Price) {
				m.cntUp = 0
			}
		}

		m.prevAsk = tick
	case models.MsgTypeTopBid:
		tick := getPayload(e)
		// Closing long position if expected PnL > profitMargin.
		profitMargin := tick.Price.Mul(decimal.NewFromFloat(slippage))
		if pos.Amount.IsPositive() &&
			pos.EntryPrice.LessThan(tick.Price.Sub(profitMargin)) {
			m.ch <- models.OrderSideSell
			return
		}
		if pos.Amount.IsNegative() {
			// We are already in position.
			return
		}
		if m.prevBid != nil {
			if m.prevBid.Price.LessThan(tick.Price) {
				m.cntDown++
				log.Printf("vvv %d\n", m.cntDown)
				if m.cntDown > patience {
					// Opening short position if price is going up.
					if atomic.AddUint32(&m.numEvents, 1) < 4 {
						m.ch <- models.OrderSideSell
					}
					m.cntDown = 0
				}
			} else if m.prevBid.Price.GreaterThan(tick.Price) {
				m.cntDown = 0
			}
		}

		m.prevBid = tick
	}
}

func (m *Monkey) Say() <-chan models.OrderSide {
	return m.ch
}
