package binance

import (
	"context"
	"sync"

	"degen/pkg/connectors"
)

const Name = "binance"

type Binance struct {
	ws                   *connectors.WS
	API                  *API
	subscribedStreams    []string
	subscriptionRequests map[uint64][]string

	listenKey   string
	reconnectCh chan any

	mux sync.RWMutex
}

func NewBinance(
	ctx context.Context,
	key, secret, apiBaseURL, wsBaseURL string,
) *Binance {
	b := &Binance{
		API:                  NewAPI(key, secret, apiBaseURL),
		ws:                   &connectors.WS{},
		reconnectCh:          make(chan any),
		subscriptionRequests: make(map[uint64][]string),
	}

	lkOnce := sync.Once{}
	lkReady := make(chan any)
	go b.refreshListenKeyLoop(ctx, &lkOnce, lkReady)
	select {
	case <-lkReady:
	case <-ctx.Done():
		return nil
	}

	wsOnce := sync.Once{}
	wsReady := make(chan any)
	go b.wsReconnectLoop(ctx, wsBaseURL, &wsOnce, wsReady)
	select {
	case <-wsReady:
	case <-ctx.Done():
		return nil
	}

	return b
}
