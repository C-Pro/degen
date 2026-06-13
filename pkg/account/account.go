package account

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/c-pro/geche"
	"github.com/shopspring/decimal"

	"degen/pkg/metrics"
	"degen/pkg/models"
)

type exchange interface {
	Name() string
	GetSymbols(ctx context.Context) (map[string]models.SymbolInfo, error)
	GetAccountInfo(ctx context.Context) (*models.AccountInfo, error)
	GetOrderDetails(ctx context.Context, order models.Order) (*models.Order, error)
	PlaceOrder(ctx context.Context, order models.Order) (*models.Order, error)
	CancelOrder(ctx context.Context, order models.Order) (*models.Order, error)
	CancelAllOrders(ctx context.Context, symbol string) error
	GetOpenOrders(ctx context.Context, symbol string) ([]models.Order, error)
	Listen(ctx context.Context, ch chan<- models.ExchangeMessage)
	SubscribeBookTickers(ctx context.Context, symbols []string) error
	SubscribeBookAggTrades(ctx context.Context, symbols []string) error
	SubscribeUserOrders(ctx context.Context) error
	SubscribeUserBalance(ctx context.Context) error
	SubscribeUserTrades(ctx context.Context) error
	RequestReconnect(reason string)
}

type Account struct {
	exchange
	id        string
	balances  map[string]models.Balance
	positions map[string]*positionStructure
	orders    *geche.KVCache[string, models.Order]
	interest  map[string]*openInterest
	ctx       context.Context
	cancel    context.CancelFunc
	updCh     chan models.ExchangeMessage

	mux    sync.RWMutex
	stopWg sync.WaitGroup
}

func NewAccount(id string, api exchange) (*Account, error) {
	a := &Account{
		id:        id,
		exchange:  api,
		balances:  make(map[string]models.Balance),
		positions: make(map[string]*positionStructure),
		orders:    geche.NewKVCache[string, models.Order](),
		interest:  make(map[string]*openInterest),
		updCh:     make(chan models.ExchangeMessage, 100),
	}

	info, err := a.GetAccountInfo(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get initial account info: %w", err)
	}

	a.balances = info.Balances
	for sym, pos := range info.Positions {
		a.positions[sym] = &positionStructure{}
		a.positions[sym].Update(models.PositionUpdate{
			Amount:    pos.Amount,
			Price:     pos.AveragePrice,
			Timestamp: pos.UpdatedAt,
		})
	}

	orders, err := api.GetOpenOrders(context.Background(), "")
	if err != nil {
		return nil, fmt.Errorf("failed to get open orders: %w", err)
	}
	for _, order := range orders {
		if err := a.Update(models.ExchangeMessage{
			Exchange: api.Name(),
			MsgType:  models.MsgTypeOrderStatus,
			Symbol:   order.Symbol,
			Payload:  order,
		},
		); err != nil {
			return nil, fmt.Errorf("failed to update order: %w", err)
		}
	}

	return a, nil
}

func (a *Account) Start(ctx context.Context) error {
	a.ctx, a.cancel = context.WithCancel(ctx)
	ch := make(chan models.ExchangeMessage, 100)
	go func() {
		a.Listen(a.ctx, ch)
		close(ch)
	}()

	a.stopWg.Add(1)
	go func() {
		defer a.stopWg.Done()
		a.updateLoop(a.ctx, ch)
	}()

	return nil
}

func (a *Account) Stop() {
	a.cancel()
	a.stopWg.Wait()
	close(a.updCh)
}

// Updates returns a channel of exchange messages for the strategy to consume.
//
// Delivery is best-effort: account state is always applied (via Update) before
// a message is forwarded here, and forwarding uses a non-blocking send. If the
// consumer is slower than the inbound message rate the oldest queued forwards
// are dropped rather than stalling state application and back-pressuring the
// websocket reader. Consumers must therefore treat this as a "latest view"
// stream (the ladder strategy only acts on the most recent BBO) and rely on
// the Get* accessors for authoritative state.
func (a *Account) Updates() <-chan models.ExchangeMessage {
	return a.updCh
}

func (a *Account) SubscribeSymbols(symbols []string) error {
	if err := a.SubscribeBookAggTrades(a.ctx, symbols); err != nil {
		return fmt.Errorf("failed to subscribe %v", err)
	}
	if err := a.SubscribeBookTickers(a.ctx, symbols); err != nil {
		return fmt.Errorf("failed to subscribe %v", err)
	}
	return nil
}

func (a *Account) updateLoop(ctx context.Context, ch chan models.ExchangeMessage) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			if (msg.MsgType != models.MsgTypeBBO) && (msg.MsgType != models.MsgTypeBalanceUpdate) {
				var mt string
				switch msg.MsgType {
				case models.MsgTypeOrderStatus:
					mt = "order"
				case models.MsgTypePositionUpdate:
					mt = "position"
				}
				log.Printf("Received %s message: %v\n", mt, msg)
			}
			if err := a.Update(msg); err != nil {
				// A single bad/unknown message must not permanently disable the
				// account update pipeline; log and keep processing. [M1]
				log.Printf("failed to apply update (type %d): %v\n", msg.MsgType, err)
				continue
			}

			// Forward to the strategy without blocking: if the consumer is slow,
			// drop the queued forward rather than stalling state application and
			// back-pressuring the websocket reader (which would also starve the
			// connector's idle watchdog). [H4]
			select {
			case a.updCh <- msg:
			default:
			}
		}
	}
}

func (a *Account) UpdateBalance(
	asset string,
	balance decimal.Decimal,
	locked decimal.Decimal,
	updatedAt time.Time,
) {
	a.mux.Lock()
	defer a.mux.Unlock()

	metrics.RecordAssetBalance(a.Name(), asset, balance.InexactFloat64())

	a.balances[asset] = models.Balance{
		Total:     balance,
		Available: balance.Sub(locked),
		UpdatedAt: updatedAt,
	}
}

func (a *Account) GetOrder(symbol, clientOrderID string) *models.Order {
	o, err := a.orders.Get(fmt.Sprintf("%s:%s", symbol, clientOrderID))
	if err != nil {
		return nil
	}

	return &o
}

func (a *Account) GetOpenOrders(symbol string) []models.Order {
	prefix := ""
	if symbol != "" {
		prefix = symbol + ":"
	}

	orders, _ := a.orders.ListByPrefix(prefix)

	return orders
}

func (a *Account) GetTotalBidSize(symbol string) decimal.Decimal {
	a.mux.RLock()
	defer a.mux.RUnlock()

	oi, ok := a.interest[symbol]
	if !ok {
		return decimal.Zero
	}

	return oi.totalBidSize
}

func (a *Account) GetTotalAskSize(symbol string) decimal.Decimal {
	a.mux.RLock()
	defer a.mux.RUnlock()

	oi, ok := a.interest[symbol]
	if !ok {
		return decimal.Zero
	}

	return oi.totalAskSize
}

// OpenInterestSnapshot is a consistent point-in-time view of the resting orders
// for a symbol.
type OpenInterestSnapshot struct {
	TotalBidSize decimal.Decimal
	TotalAskSize decimal.Decimal
	AvgBidPrice  decimal.Decimal
	AvgAskPrice  decimal.Decimal
}

// GetOpenInterest returns all four open-interest aggregates for a symbol under a
// single lock acquisition, so callers see a coherent snapshot (and avoid
// repeated locking). Avg prices are zero when the corresponding side is empty.
func (a *Account) GetOpenInterest(symbol string) OpenInterestSnapshot {
	a.mux.RLock()
	defer a.mux.RUnlock()

	oi, ok := a.interest[symbol]
	if !ok {
		return OpenInterestSnapshot{}
	}

	return OpenInterestSnapshot{
		TotalBidSize: oi.totalBidSize,
		TotalAskSize: oi.totalAskSize,
		AvgBidPrice:  oi.GetAvgBidPrice(),
		AvgAskPrice:  oi.GetAvgAskPrice(),
	}
}

func (a *Account) UpdatePosition(
	symbol string,
	amount decimal.Decimal,
	price decimal.Decimal,
	updatedAt time.Time,
) {
	log.Printf("Updating position %s: %s %s\n", symbol, amount, price)
	a.mux.Lock()
	defer a.mux.Unlock()

	pos, ok := a.positions[symbol]
	if !ok {
		pos = &positionStructure{}
		a.positions[symbol] = pos
	}

	pos.Update(models.PositionUpdate{
		Amount:    amount,
		Price:     price,
		Timestamp: updatedAt,
	})

	metrics.RecordPosition(a.Name(), symbol, pos.Position())
}

func orderKey(order models.Order) string {
	return fmt.Sprintf("%s:%s", order.Symbol, order.ClientOrderID)
}

func (a *Account) UpdateOrder(order models.Order) {
	key := orderKey(order)
	existing, err := a.orders.Get(key)
	if err == nil && existing.Status == models.OrderStatusNew {
		log.Printf("Order time to book: %s\n", order.CreatedAt.Sub(existing.PlacedAt))
		log.Printf("Order e2e time: %s\n", order.UpdatedAt.Sub(existing.PlacedAt))
		metrics.RecordPlaceOrderDuration(
			a.Name(),
			existing.PlacedAt,
		)
	}
	// Always observe, including terminal (Final) orders: a placed order added
	// its open size to the per-side totals, so the matching fill/cancel must be
	// observed to subtract it before the order is dropped. observe() removes the
	// order from the per-side maps when Final is set. [H5]
	a.observeOrder(order)

	if order.Final {
		// nolint:errcheck
		a.orders.Del(key)
		log.Printf("order %s () deleted", key)
		return
	}

	a.orders.Set(key, order)
}

// observeOrder applies an order update to the per-symbol open interest while
// holding the account lock, so that mutation is serialized with the readers
// (GetTotalBidSize/GetTotalAskSize/GetOpenInterest) and with concurrent
// UpdateOrder calls from both the update loop and the strategy goroutine. [H2]
//
// If observe reports an inconsistency (e.g. totals would go negative on an
// out-of-order update) it resyncs the open interest from a fresh REST snapshot,
// and on failure requests a websocket reconnect rather than blocking on an
// unread error channel. [H3]
func (a *Account) observeOrder(order models.Order) {
	a.mux.Lock()
	oi, ok := a.interest[order.Symbol]
	if !ok {
		oi = newOpenInterest()
		a.interest[order.Symbol] = oi
	}
	err := oi.observe(order)
	a.mux.Unlock()

	if err == nil {
		return
	}

	log.Printf("failed to observe order: %v\n", err)
	orders, gerr := a.exchange.GetOpenOrders(context.Background(), order.Symbol)
	if gerr != nil {
		log.Printf("failed to resync open orders for %s: %v\n", order.Symbol, gerr)
		a.RequestReconnect("open interest resync failed")
		return
	}

	a.mux.Lock()
	// Re-fetch the live open interest rather than reusing the pointer captured
	// before the (unlocked) REST call: a concurrent CancelAllOrders/purge may
	// have replaced or removed it. If it was purged we honor that (a cancel-all
	// happened, so the stale snapshot must not resurrect orders).
	if cur, ok := a.interest[order.Symbol]; ok {
		if rerr := cur.setFromOrders(orders); rerr != nil {
			log.Printf("failed to rebuild open interest for %s: %v\n", order.Symbol, rerr)
			a.RequestReconnect("open interest rebuild failed")
		}
	}
	a.mux.Unlock()
}

func (a *Account) GetBalance(asset string) models.Balance {
	a.mux.RLock()
	defer a.mux.RUnlock()

	return a.balances[asset]
}

func (a *Account) GetPosition(symbol string) models.Position {
	a.mux.RLock()
	defer a.mux.RUnlock()

	pos, ok := a.positions[symbol]
	if !ok {
		return models.Position{}
	}

	return pos.Position()
}

// GetPositionMinReducePrice returns the boundary entry price for reduce-only
// quoting on the given symbol, accounting for position direction (lowest entry
// for long, highest for short). Returns zero when there is no position.
func (a *Account) GetPositionMinReducePrice(symbol string) decimal.Decimal {
	a.mux.RLock()
	defer a.mux.RUnlock()

	pos, ok := a.positions[symbol]
	if !ok {
		return decimal.Zero
	}

	if pos.totalSize.IsZero() {
		return decimal.Zero
	}

	return pos.minReducePrice()
}

func (a *Account) GetPositionReduceSize(symbol string, price decimal.Decimal) decimal.Decimal {
	a.mux.RLock()
	defer a.mux.RUnlock()

	pos, ok := a.positions[symbol]
	if !ok {
		return decimal.Zero
	}

	if pos.totalSize.IsZero() {
		return decimal.Zero
	}

	return decimal.NewFromFloat(pos.getReduceSize(price.InexactFloat64()))
}

func (a *Account) Update(upd models.ExchangeMessage) error {
	switch upd.MsgType {
	case models.MsgTypeOrderStatus:
		order, ok := upd.Payload.(models.Order)
		if !ok {
			return fmt.Errorf("invalid payload type %T for MsgType %q", upd.Payload, upd.MsgType)
		}
		a.UpdateOrder(order)
	case models.MsgTypeBalanceUpdate:
		bal, ok := upd.Payload.(models.Balance)
		if !ok {
			return fmt.Errorf("invalid payload type %T for MsgType %q", upd.Payload, upd.MsgType)
		}
		a.UpdateBalance(upd.Symbol, bal.Total, bal.Total.Sub(bal.Available), upd.Timestamp)
	case models.MsgTypePositionUpdate:
		pos, ok := upd.Payload.(models.PositionUpdate)
		if !ok {
			return fmt.Errorf("invalid payload type %T for MsgType %q", upd.Payload, upd.MsgType)
		}
		a.UpdatePosition(upd.Symbol, pos.Amount, pos.Price, upd.Timestamp)
	case models.MsgTypeBBO:
		bbo, ok := upd.Payload.(models.BBO)
		if !ok {
			return fmt.Errorf("invalid payload type %T for MsgType %q", upd.Payload, upd.MsgType)
		}
		metrics.RecordBBO(a.Name(), upd.Symbol, bbo)
		a.mux.RLock()
		position, ok := a.positions[upd.Symbol]
		hasPos := ok && !position.totalSize.IsZero()
		var pos models.Position
		if hasPos {
			pos = position.Position()
		}
		a.mux.RUnlock()
		if hasPos {
			price := bbo.Ask.Price
			if pos.Amount.Sign() > 0 {
				price = bbo.Bid.Price
			}
			metrics.RecordUnrealizedPnL(
				a.Name(),
				upd.Symbol,
				pos.UnrealizedPnL(price).InexactFloat64(),
			)
		}
	}

	return nil
}

func (a *Account) PlaceOrder(ctx context.Context, order models.Order) (*models.Order, error) {
	order.PlacedAt = time.Now().UTC()
	order.Status = models.OrderStatusNew
	a.orders.Set(orderKey(order), order)
	o, err := a.exchange.PlaceOrder(ctx, order)
	if err != nil {
		// nolint:errcheck
		a.orders.Del(orderKey(order))
		return nil, err
	}

	if o.Final {
		// nolint:errcheck
		a.orders.Del(orderKey(order))
	}

	return o, nil
}

func (a *Account) CancelOrder(ctx context.Context, order models.Order) (*models.Order, error) {
	o, err := a.exchange.CancelOrder(ctx, order)
	if err == nil && o != nil {
		a.UpdateOrder(*o)
	}
	if time.Since(order.PlacedAt) > time.Second*10 && errors.Is(err, models.ErrOrderNotFound) {
		// nolint:errcheck
		a.orders.Del(orderKey(order))
		order.Status = models.OrderStatusCanceled
		return &order, nil
	}

	return o, err
}

func (a *Account) CancelAllOrders(ctx context.Context, symbol string) error {
	if err := a.exchange.CancelAllOrders(ctx, symbol); err != nil {
		return err
	}

	// Proactively clear local tracking so the order count reflects reality
	// immediately, instead of waiting for asynchronous websocket cancel
	// confirmations (which otherwise lets callers re-trigger cancel-all in a
	// tight loop). Late CANCELED frames for these orders are harmless: observe
	// no longer finds them and skips, and they are already gone from the cache. [M4]
	a.purgeOrders(symbol)
	return nil
}

// purgeOrders removes locally tracked orders and open interest for a symbol
// (all symbols when symbol is "").
func (a *Account) purgeOrders(symbol string) {
	prefix := ""
	if symbol != "" {
		prefix = symbol + ":"
	}
	orders, _ := a.orders.ListByPrefix(prefix)
	for _, o := range orders {
		// nolint:errcheck
		a.orders.Del(orderKey(o))
	}

	a.mux.Lock()
	if symbol == "" {
		a.interest = make(map[string]*openInterest)
	} else {
		delete(a.interest, symbol)
	}
	a.mux.Unlock()
}

func (a *Account) syncOrders(ctx context.Context, symbol string) error {
	orders, _ := a.orders.ListByPrefix(symbol + ":")
	var failed int
	var lastErr error
	for _, o := range orders {
		order, err := a.GetOrderDetails(ctx, o)
		if err != nil {
			// Best-effort: log and keep reconciling the remaining orders rather
			// than aborting the whole pass on the first failure. [L5]
			log.Printf("failed to get order details for %s: %v\n", orderKey(o), err)
			failed++
			lastErr = err
			continue
		}

		a.UpdateOrder(*order)
	}

	if failed > 0 {
		return fmt.Errorf("failed to reconcile %d/%d orders for %s: %w", failed, len(orders), symbol, lastErr)
	}

	return nil
}

func (a *Account) SyncWithExchange(ctx context.Context, symbols []string) error {
	// Wait for some time for ws updates to come in.
	time.Sleep(time.Second)

	var syncErr error
	for _, symbol := range symbols {
		if err := a.syncOrders(ctx, symbol); err != nil {
			log.Printf("partial order sync for %s: %v\n", symbol, err)
			syncErr = err
		}
	}
	// Always request a reconnect so the fresh websocket snapshots repair any
	// drift that REST reconciliation could not. [L5]
	a.RequestReconnect("sync state")
	return syncErr
}
