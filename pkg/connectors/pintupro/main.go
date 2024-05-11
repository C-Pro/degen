package pintupro

import (
	"context"
)

const Name = "dummy"

type Dummy struct {
	API               *API
	subscribedStreams []string
}

func NewDummy(
	ctx context.Context,
	key, secret, apiBaseURL, wsBaseURL string,
) *Dummy {
	b := &Dummy{
		API: NewAPI(key, secret, apiBaseURL),
	}

	return b
}

func (d *Dummy) Name() string {
	return Name
}
