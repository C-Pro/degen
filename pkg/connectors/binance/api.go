package binance

import (
	"net/http"
	"time"
)

type API struct {
	key     string
	secret  string
	baseURL string

	client http.Client
}

func NewAPI(key, secret, baseURL string) *API {
	return &API{
		key:     key,
		secret:  secret,
		baseURL: baseURL,
		client: http.Client{
			Timeout: time.Second,
		},
	}
}

func (api *API) signRequest(queryString, body string, ts time.Time) string {
	return signRequest(api.secret, queryString, body, ts)
}
