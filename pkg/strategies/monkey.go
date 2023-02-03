package strategies

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"degen/pkg/models"
)

const patience = 1

type Monkey struct {
	ch               chan models.OrderSide
	cntUp, cntDown   int
	prevBid, prevAsk *models.PriceLevel
	numEvents        uint32
}

func NewMonkey(ctx context.Context) *Monkey {
	m := &Monkey{
		ch: make(chan models.OrderSide),
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
				if m.cntUp > 1 {
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
		// buy
		tick := getPayload(e)
		if m.prevBid != nil {
			if m.prevBid.Price.LessThan(tick.Price) {
				m.cntDown++
				log.Printf("vvv %d\n", m.cntDown)
				if m.cntDown > 1 {
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
