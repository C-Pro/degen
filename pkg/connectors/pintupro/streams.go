package pintupro

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"degen/pkg/models"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func (p *PintuPro) wsReconnectLoop(ctx context.Context, wsBaseURL string) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.reconnectCh:
			if err := p.ws.Connect(ctx, wsBaseURL); err != nil {
				log.Printf("pintupro websocket connect error: %v", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
					continue
				}
			}
			if p.key != "" {
				if err := p.auth(ctx); err != nil {
					log.Printf("pintupro auth write error: %v", err)
					select {
					case <-ctx.Done():
						return
					case <-time.After(time.Second):
						continue
					}
				}
			}
			// Connected. Subscribe to streams.
			connectedAt = time.Now()

			var toSubscribe []string
			p.mux.RLock()
			if len(p.subscribedStreams) > 0 {
				toSubscribe = p.subscribedStreams
				p.subscribedStreams = p.subscribedStreams[:0]
			}
			p.mux.RUnlock()

			if len(toSubscribe) > 0 {
				log.Printf("pintupro: subscribing: %q", strings.Join(toSubscribe, ","))
				if err := p.subscribeStreams(ctx, toSubscribe); err != nil {
					log.Printf("pintupro websocket subscribe error: %v", err)
					select {
					case <-ctx.Done():
						return
					case <-time.After(time.Second):
						continue
					}
				}
			}
		}
	}
}

func (p *PintuPro) auth(ctx context.Context) error {
	req := WrapAndSign("public/auth", p.key, p.secret, uuid.NewString(), nil, time.Now())
	b, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}
	return p.ws.Write(ctx, b)
}

func (p *PintuPro) SubscribeBookTickers(ctx context.Context, symbols []string) error {
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
	p.subscribedStreams = append(p.subscribedStreams, streams...)

	return nil
}

func (p *PintuPro) SubscribeBookAggTrades(ctx context.Context, symbols []string) error {
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

	p.subscribedStreams = append(p.subscribedStreams, streams...)

	return nil
}

func (p *PintuPro) Listen(ctx context.Context, ch chan<- models.ExchangeMessage) {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			stream := p.subscribedStreams[rand.Intn(len(p.subscribedStreams))]
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
