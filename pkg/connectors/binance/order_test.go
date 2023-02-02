package binance

import (
	"context"
	"os"
	"testing"
	"time"

	"degen/pkg/models"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestPlaceOrder(t *testing.T) {
	if os.Getenv("BINANCE_KEY") == "" {
		t.Skip("No Binance API key in ENV")
	}
	api := NewAPI(
		os.Getenv("BINANCE_KEY"),
		os.Getenv("BINANCE_SECRET"),
		"https://testnet.binancefuture.com",
	)

	order := models.Order{
		ClientOrderID: uuid.New().String(),
		CreatedAt:     time.Now().UTC(),
		Symbol:        "dogeusdt",
		Size:          decimal.NewFromInt(200),
		Side:          models.OrderSideBuy,
		Type:          models.OrderTypeMarket,
	}

	res, err := api.PlaceOrder(context.Background(), order)
	if err != nil {
		t.Fatalf("PlaceOrder returned error: %v", err)
	}

	if res.Status != models.OrderStatusPlaced {
		t.Errorf("Expected status %s but got %s", models.OrderStatusPlaced, res.Status)
	}

	latency := res.UpdatedAt.Sub(order.CreatedAt)
	if latency > 500*time.Millisecond {
		t.Errorf("Latency too big: %v", latency)
	}
}

func TestPlaceCancelOrder(t *testing.T) {
	if os.Getenv("BINANCE_KEY") == "" {
		t.Skip("No Binance API key in ENV")
	}
	api := NewAPI(
		os.Getenv("BINANCE_KEY"),
		os.Getenv("BINANCE_SECRET"),
		"https://testnet.binancefuture.com",
	)

	order := models.Order{
		ClientOrderID: uuid.New().String(),
		CreatedAt:     time.Now().UTC(),
		Symbol:        "dogeusdt",
		Size:          decimal.NewFromInt(2000),
		Price:         decimal.NewFromFloat(0.01),
		Side:          models.OrderSideBuy,
		Type:          models.OrderTypeLimit,
		TimeInForce:   models.TimeInForceGTC,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	res, err := api.PlaceOrder(ctx, order)
	if err != nil {
		t.Fatalf("PlaceOrder returned error: %v", err)
	}

	if res.Status != models.OrderStatusPlaced {
		t.Errorf("Expected status %s but got %s", models.OrderStatusPlaced, res.Status)
	}

	latency := res.UpdatedAt.Sub(order.CreatedAt)
	if latency > 500*time.Millisecond {
		t.Errorf("Latency too big: %v", latency)
	}

	res2, err := api.CancelOrder(ctx, *res)
	if err != nil {
		t.Fatalf("CancelOrder returned error: %v", err)
	}

	if res2.Status != models.OrderStatusCanceled {
		t.Errorf("Expected status %s but got %s", models.OrderStatusCanceled, res2.Status)
	}
}
