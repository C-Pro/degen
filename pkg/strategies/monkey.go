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
	slippage = 0.01
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
	switch e.MsgType {
	case models.MsgTypeTopAsk:
		// sell
		tick := getPayload(e)
		if m.prevAsk != nil {
			if m.prevAsk.Price.GreaterThan(tick.Price) {
				m.cntUp++
				log.Printf("^^^ %d\n", m.cntUp)
				if m.cntUp > patience {
					if atomic.AddUint32(&m.numEvents, 1) < 4 {
						pos := m.acc.GetPosition(symbol)
						adjustedTick := tick.Price.Add(tick.Price.Mul(decimal.NewFromFloat(slippage)))
						if !pos.Amount.IsNegative() ||
							(pos.Amount.IsNegative() &&
								pos.EntryPrice.GreaterThan(adjustedTick)) {
							m.ch <- models.OrderSideBuy
						}
					}
					m.cntUp = 0
				}
			} else if m.prevAsk.Price.LessThan(tick.Price) {
				m.cntUp = 0
			}
		}

		m.prevAsk = tick
	case models.MsgTypeTopBid:
		// buy
		tick := getPayload(e)
		if m.prevBid != nil {
			if m.prevBid.Price.LessThan(tick.Price) {
				m.cntDown++
				log.Printf("vvv %d\n", m.cntDown)
				if m.cntDown > patience {
					if atomic.AddUint32(&m.numEvents, 1) < 4 {
						pos := m.acc.GetPosition(symbol)
						adjustedTick := tick.Price.Sub(tick.Price.Mul(decimal.NewFromFloat(slippage)))
						if !pos.Amount.IsPositive() ||
							(pos.Amount.IsPositive() &&
								pos.EntryPrice.LessThan(adjustedTick)) {
							m.ch <- models.OrderSideSell
						}
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
