package dummy

import (
	"context"
	"fmt"
	"slices"
	"time"

	"degen/pkg/models"

	"github.com/c-pro/geche"
	"github.com/shopspring/decimal"
)

const (
	Name             = "dummy"
	StreamOrders     = "orders"
	StreamBalances   = "balances"
	StreamPositions  = "positions"
	StreamTickers    = "tickers"
	StreamTrades     = "trades"
	StreamOrderBooks = "orderbooks"
)

type Dummy struct {
	orders *geche.KV[models.Order]
	// Index mapping client order ID to exchange order ID
	ordersByClientID  geche.Geche[string, string]
	balances          *geche.KV[models.Balance]
	positions         *geche.KV[models.Position]
	symbols           geche.Geche[string, models.SymbolInfo]
	subscribedStreams []string
	ch                chan<- models.ExchangeMessage
}

func NewDummy(
	ctx context.Context,
	key, secret, apiBaseURL, wsBaseURL string,
) *Dummy {
	b := &Dummy{
		orders:           geche.NewKV(geche.NewMapCache[string, models.Order]()),
		ordersByClientID: geche.NewMapCache[string, string](),
		balances:         geche.NewKV(geche.NewMapCache[string, models.Balance]()),
		positions:        geche.NewKV(geche.NewMapCache[string, models.Position]()),
		symbols:          geche.NewMapCache[string, models.SymbolInfo](),
	}

	return b
}

func (d *Dummy) Name() string {
	return Name
}

func orderKey(order models.Order) string {
	return fmt.Sprintf("%s-%s", order.Symbol, order.ExchangeOrderID)
}

func (d *Dummy) SetOrder(order models.Order) {
	if order.Final {
		d.orders.Del(orderKey(order))
		if order.ClientOrderID != "" {
			d.ordersByClientID.Del(order.ClientOrderID)
		}
	} else {
		d.orders.Set(orderKey(order), order)
		if order.ClientOrderID != "" {
			d.ordersByClientID.Set(order.ClientOrderID, order.ExchangeOrderID)
		}
	}
	if slices.Contains(d.subscribedStreams, StreamOrders) {
		d.ch <- models.ExchangeMessage{
			MsgType:  models.MsgTypeOrderStatus,
			Exchange: Name,
			Symbol:   order.Symbol,
			Payload:  order,
		}
	}
}

func (d *Dummy) SetBalance(balance models.Balance, asset string) {
	d.balances.Set(asset, balance)
	if balance.Total.IsZero() {
		d.balances.Del(asset)
	}
	if slices.Contains(d.subscribedStreams, StreamBalances) {
		d.ch <- models.ExchangeMessage{
			MsgType:  models.MsgTypeBalanceUpdate,
			Symbol:   asset,
			Exchange: Name,
			Payload:  balance,
		}
	}
}

func (d *Dummy) SetPosition(position models.Position, symbol string) {
	d.positions.Set(symbol, position)
	if position.Amount.IsZero() {
		d.positions.Del(symbol)
	}
	if slices.Contains(d.subscribedStreams, StreamPositions) {
		d.ch <- models.ExchangeMessage{
			MsgType:  models.MsgTypePositionUpdate,
			Symbol:   symbol,
			Exchange: Name,
			Payload:  position,
		}
	}
}

func (d *Dummy) SetSymbol(symbol models.SymbolInfo) {
	d.symbols.Set(symbol.Symbol, symbol)
	if slices.Contains(d.subscribedStreams, StreamTickers) {
		d.ch <- models.ExchangeMessage{
			MsgType:   models.MsgTypeMarketTicker,
			Symbol:    symbol.Symbol,
			Exchange:  Name,
			Payload:   symbol,
			Timestamp: time.Now().UTC(),
		}
	}
}

func (d *Dummy) SetOrderBook(orderBook models.OrderBook) {
	if slices.Contains(d.subscribedStreams, StreamOrderBooks) {
		bbo := models.BBO{
			Bid: models.PriceLevel{
				Price: decimal.NewFromFloat(orderBook.Bids[0][0]),
				Size:  decimal.NewFromFloat(orderBook.Bids[0][1]),
			},
			Ask: models.PriceLevel{
				Price: decimal.NewFromFloat(orderBook.Asks[0][0]),
				Size:  decimal.NewFromFloat(orderBook.Asks[0][1]),
			},
			Timestamp: orderBook.Timestamp,
		}

		d.ch <- models.ExchangeMessage{
			MsgType:   models.MsgTypeBBO,
			Symbol:    orderBook.Symbol,
			Exchange:  Name,
			Payload:   bbo,
			Timestamp: orderBook.Timestamp,
		}
	}
}
