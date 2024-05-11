package pintupro

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"time"

	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

func (d *Dummy) SubscribeBookTickers(ctx context.Context, symbols []string) error {
	if len(symbols) == 0 {
		return nil
	}
	if symbols[0] == "error" {
		return errors.New("SubscribeBookTickers error")
	}

	streams := make([]string, len(symbols))
	for i, s := range symbols {
		streams[i] = strings.ToLower(s) + "@bookTicker"
	}
	d.subscribedStreams = append(d.subscribedStreams, streams...)

	return nil
}

func (d *Dummy) SubscribeBookAggTrades(ctx context.Context, symbols []string) error {
	if len(symbols) == 0 {
		return nil
	}
	if symbols[0] == "error" {
		return errors.New("SubscribeBookAggTrades error")
	}
	streams := make([]string, len(symbols))
	for i, s := range symbols {
		streams[i] = strings.ToLower(s) + "@aggTrade"
	}

	d.subscribedStreams = append(d.subscribedStreams, streams...)

	return nil
}

func (d *Dummy) Listen(ctx context.Context, ch chan<- models.ExchangeMessage) {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			stream := d.subscribedStreams[rand.Intn(len(d.subscribedStreams))]
			parts := strings.Split(stream, "@")
			switch parts[1] {
			case "bookTicker":
				ch <- models.ExchangeMessage{
					Exchange:  Name,
					Symbol:    parts[0],
					Timestamp: time.Now().UTC(),
					MsgType:   models.MsgTypeBBO,
					Payload: models.BBO{
						Bid: models.PriceLevel{
							Price: decimal.NewFromFloat(rand.Float64() * 100),
							Size:  decimal.NewFromFloat(rand.Float64() * 100),
						},
						Ask: models.PriceLevel{
							Price: decimal.NewFromFloat(rand.Float64() * 100),
							Size:  decimal.NewFromFloat(rand.Float64() * 100),
						},
						Timestamp: time.Now().UTC(),
					},
				}
			case "aggTrade":
				side := models.OrderSideSell
				if rand.Float64() < 0.5 {
					side = models.OrderSideBuy
				}
				ch <- models.ExchangeMessage{
					Exchange:  Name,
					Symbol:    parts[0],
					Timestamp: time.Now().UTC(),
					MsgType:   models.MsgTypeTrade,
					Payload: models.Trade{
						Price:     decimal.NewFromFloat(rand.Float64() * 100),
						Size:      decimal.NewFromFloat(rand.Float64() * 100),
						Timestamp: time.Now().UTC(),
						Side:      side,
					},
				}
			}

			switch rand.Intn(3) {
			case 0:
				ch <- models.ExchangeMessage{
					Exchange:  Name,
					Symbol:    "BTCUSDT",
					Timestamp: time.Now().UTC(),
					MsgType:   models.MsgTypeOrderStatus,
					Payload: models.Order{
						ClientOrderID:   "client-order-id",
						ExchangeOrderID: "order-id",
						UpdatedAt:       time.Now().UTC(),
						Status:          models.OrderStatusPlaced,
						Side:            "BUY",
						Symbol:          "BTCUSDT",
						FilledSize:      decimal.Zero,
						AveragePrice:    decimal.Zero,
					},
				}
			case 1:
				ch <- models.ExchangeMessage{
					Exchange:  Name,
					Timestamp: time.Now().UTC(),
					MsgType:   models.MsgTypeBalanceUpdate,
					Payload: models.BalanceUpdate{
						Asset:   "BTC",
						Balance: decimal.NewFromInt(1),
					},
				}

			case 2:
				ch <- models.ExchangeMessage{
					Exchange:  Name,
					Timestamp: time.Now().UTC(),
					MsgType:   models.MsgTypePositionUpdate,
					Payload: models.PositionUpdate{
						Symbol:     "BTCUSDT",
						Amount:     decimal.NewFromInt(1),
						EntryPrice: decimal.NewFromInt(100),
					},
				}
			}

		}
	}
}
