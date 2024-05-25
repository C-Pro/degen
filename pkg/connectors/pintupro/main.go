package pintupro

import (
	"context"
	"sync"
	"time"

	"degen/pkg/connectors"
	"degen/pkg/models"
)

const Name = "pintupro"

type PintuPro struct {
	API               *API
	ws                *connectors.WS
	subscribedStreams []string
	// subscriptionRequests map[string]string
	wsHandlers   map[string]wsHandlerFunc
	lastReceived int64
	idleTimeout  time.Duration
	key, secret  string

	reconnectCh chan any

	mux sync.RWMutex
}

type wsHandlerFunc func(msg wsMessage, ch chan<- models.ExchangeMessage) error

func NewPintuPro(
	ctx context.Context,
	key, secret, apiBaseURL, wsBaseURL string,
) *PintuPro {
	p := &PintuPro{
		API:         NewAPI(key, secret, apiBaseURL),
		ws:          &connectors.WS{},
		reconnectCh: make(chan any),
		wsHandlers:  make(map[string]wsHandlerFunc),
		idleTimeout: 5 * time.Second,
		key:         key,
		secret:      secret,
	}

	p.registerWSHandlers()

	go p.wsReconnectLoop(ctx, wsBaseURL)

	return p
}

func (d *PintuPro) Name() string {
	return Name
}
