package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"degen/pkg/accounts"
	"degen/pkg/connectors/binance"
	"degen/pkg/models"
	"degen/pkg/strategies"

	"github.com/shopspring/decimal"
)

const (
	theSymbol = "ethusdt"
	theAsset  = "usdt"
)

func main() {
	initialBalance := decimal.Zero
	once := sync.Once{}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	ch := make(chan models.ExchangeMessage, 100)
	go func() {
		<-ctx.Done()
		close(ch)
	}()

	bnc := binance.NewBinance(
		ctx,
		os.Getenv("BINANCE_KEY"),
		os.Getenv("BINANCE_SECRET"),
		"https://testnet.binancefuture.com",
		"wss://stream.binancefuture.com",
	)

	if bnc == nil {
		return
	}

	go bnc.Listen(ctx, ch)

	if err := bnc.SubscribeBookTickers(ctx, []string{theSymbol}); err != nil {
		log.Printf("failed to subscribe: %v\n", err)
		return
	}

	accs := accounts.NewAccounts()
	acc := accs.AddAccount("monkey", binance.Name)

	monkey := strategies.NewMonkey(ctx, acc)
	go func() {
		for {
			select {
			case side := <-monkey.Say():
				log.Printf("MONKEY WANNA %s!\n", strings.ToUpper(string(side)))
				order := models.Order{
					CreatedAt: time.Now().UTC(),
					Symbol:    theSymbol,
					Size:      decimal.NewFromFloat(0.05),
					Side:      side,
					Type:      models.OrderTypeMarket,
				}
				res, err := bnc.API.PlaceOrder(ctx, order)
				if err != nil {
					log.Printf("monkey has failed to place an order: %v", err)
					continue
				}

				if res.Type == models.OrderTypeMarket {
					log.Printf("monkey has placed a %s order to %s %v %s\n",
						res.Type,
						res.Side,
						res.Size,
						res.Symbol,
					)
				} else {
					log.Printf("monkey has placed a %s order to %s %v %s at %v\n",
						res.Type,
						res.Side,
						res.Size,
						res.Symbol,
						res.Price,
					)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		var lastChange time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				b := acc.GetBalance(theAsset)
				if b.UpdatedAt.After(lastChange) {
					pnl := b.Balance.Sub(initialBalance)
					log.Printf("### Current balance is %v; PNL is %v", b.Balance, pnl)
					lastChange = b.UpdatedAt
				}
			}
		}
	}()

	format := "Best %s price: %v, size: %v\n"
	side := ""
	for msg := range ch {

		switch msg.MsgType {
		case models.MsgTypeTopAsk:
			side = "ask"
		case models.MsgTypeTopBid:
			side = "bid"
		case models.MsgTypeOrderStatus:
			upd := msg.Payload.(models.OrderUpdate)
			log.Printf("%s: %s (%v at %v)\n", upd.ExchangeOrderID, upd.Status, upd.FilledSize, upd.AveragePrice)
			continue
		case models.MsgTypeBalanceUpdate:
			upd := msg.Payload.(models.BalanceUpdate)
			// log.Printf("Balance %s = %v\n", upd.Asset, upd.Balance)
			acc.UpdateBalance(upd.Asset, upd.Balance, msg.Timestamp)
			once.Do(func() {
				initialBalance = upd.Balance
			})
			continue
		case models.MsgTypePositionUpdate:
			upd := msg.Payload.(models.PositionUpdate)
			// log.Printf("Position %s = %v\n", upd.Symbol, upd.Amount)
			acc.UpdatePosition(upd.Symbol, upd.Amount, upd.EntryPrice, msg.Timestamp)
			continue
		default:
			continue
		}

		tick := msg.Payload.(models.PriceLevel)
		log.Printf(format, side, tick.Price, tick.Size)
		monkey.See(msg)
	}
}
