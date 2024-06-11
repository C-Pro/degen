package pintupro

import (
	"context"
	"fmt"
	"sync"
	"time"

	"degen/pkg/connectors"
	"degen/pkg/models"
)

const Name = "pintupro"

type PintuPro struct {
	API
	ws                *connectors.WS
	subscribedStreams []string
	// subscriptionRequests map[string]string
	wsHandlers   map[string]wsHandlerFunc
	lastReceived int64
	idleTimeout  time.Duration
	key, secret  string

	reconnectCh chan any
	wsReady     chan any

	mux sync.RWMutex
}

type wsHandlerFunc func(msg wsMessage, ch chan<- models.ExchangeMessage) error

func NewPintuPro(
	ctx context.Context,
	key, secret, apiBaseURL, wsBaseURL string,
) (*PintuPro, error) {
	p := &PintuPro{
		API:         *NewAPI(key, secret, apiBaseURL),
		ws:          &connectors.WS{},
		reconnectCh: make(chan any),
		wsHandlers:  make(map[string]wsHandlerFunc),
		idleTimeout: 5 * time.Second,
		key:         key,
		secret:      secret,
		wsReady:     make(chan any),
	}

	p.registerWSHandlers()

	go p.wsReconnectLoop(ctx, wsBaseURL)
	// Wait for ws connection to be established.
	select {
	case <-ctx.Done():
		return nil, nil
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("pintupro websocket connection timeout")
	case <-p.wsReady:
	}

	return p, nil
}

func (d *PintuPro) Name() string {
	return Name
}
