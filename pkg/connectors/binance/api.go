package binance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

type listenKeyResp struct {
	ListenKey string "json:`listenKey`"
}

func (api *API) GetListenKey(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", api.baseURL+"/fapi/v1/listenKey", nil)
	if err != nil {
		return "", err
	}

	req.Header.Add("X-MBX-APIKEY", api.key)

	resp, err := api.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("binance.GetListenKey failed perform request: %w", err)
	}

	defer resp.Body.Close()
	var respData listenKeyResp

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("binance.GetListenKey failed to read response: %w", err)
	}

	if err := json.Unmarshal(b, &respData); err != nil {
		return "", fmt.Errorf("binance.GetListenKey failed to unmarshal response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("binance.GetListenKey returned code %d: %s", resp.StatusCode, string(b))
	}

	if respData.ListenKey == "" {
		return "", errors.New("binance.GetListenKey got empty lisen key")
	}

	return respData.ListenKey, nil
}
