package pintupro

import (
	"context"
	"sync"
	"time"

	"degen/pkg/connectors"
)

const Name = "pintupro"

type PintuPro struct {
	API                  *API
	ws                   *connectors.WS
	subscribedStreams    []string
	subscriptionRequests map[uint64][]string
	lastReceived         int64
	idleTimeout          time.Duration
	key, secret          string

	reconnectCh chan any

	mux sync.RWMutex
}

func NewPintuPro(
	ctx context.Context,
	key, secret, apiBaseURL, wsBaseURL string,
) *PintuPro {
	p := &PintuPro{
		API:                  NewAPI(key, secret, apiBaseURL),
		ws:                   &connectors.WS{},
		reconnectCh:          make(chan any),
		subscriptionRequests: make(map[uint64][]string),
		idleTimeout:          5 * time.Second,
		key:                  key,
		secret:               secret,
	}

	go p.wsReconnectLoop(ctx, wsBaseURL)

	return p
}

func (d *PintuPro) Name() string {
	return Name
}
