package binance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"degen/pkg/models"

	"github.com/shopspring/decimal"
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
			Timeout: time.Second * 5,
		},
	}
}

func (api *API) signRequest(queryString, body string, ts time.Time) string {
	return signRequest(api.secret, queryString, body, ts)
}

type listenKeyResp struct {
	ListenKey string `json:"listenKey"`
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

	defer func() { _ = resp.Body.Close() }()
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

// easyjson:json
type accountInfoResp struct {
	Assets []struct {
		Asset            string `json:"asset"`
		WalletBalance    string `json:"walletBalance"`
		AvaliableBalance string `json:"availableBalance"`
		UpdatedAt        int64  `json:"updateTime"`
	} `json:"assets"`
	Positions []struct {
		Symbol           string `json:"symbol"`
		PositionSide     string `json:"positionSide"`
		PositionAmt      string `json:"positionAmt"`
		EntryPrice       string `json:"entryPrice"`
		UnrealizedProfit string `json:"unrealizedProfit"`
		UpdatedAt        int64  `json:"updateTime"`
	} `json:"positions"`
}

func (api *API) GetAccountInfo(ctx context.Context) (*models.AccountInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", api.baseURL+"/fapi/v2/account", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("X-MBX-APIKEY", api.key)

	resp, err := api.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("binance.GetAccountInfo failed perform request: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()
	var respData accountInfoResp

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("binance.GetAccountInfo failed to read response: %w", err)
	}

	if err := json.Unmarshal(b, &respData); err != nil {
		return nil, fmt.Errorf("binance.GetAccountInfo failed to unmarshal response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("binance.GetAccountInfo returned code %d: %s", resp.StatusCode, string(b))
	}

	accountInfo := &models.AccountInfo{
		Balances:  make(map[string]models.Balance),
		Positions: make(map[string]models.Position),
		UpdatedAt: time.Now(),
	}

	for _, asset := range respData.Assets {
		accountInfo.Balances[asset.Asset] = models.Balance{
			Total:     decimal.RequireFromString(asset.WalletBalance),
			Available: decimal.RequireFromString(asset.AvaliableBalance),
			UpdatedAt: time.Unix(0, asset.UpdatedAt*int64(time.Millisecond)),
		}
	}

	for _, position := range respData.Positions {
		accountInfo.Positions[position.Symbol] = models.Position{
			Amount:       decimal.RequireFromString(position.PositionAmt),
			AveragePrice: decimal.RequireFromString(position.EntryPrice),
			UpdatedAt:    time.Unix(0, position.UpdatedAt*int64(time.Millisecond)),
		}
	}

	return accountInfo, nil
}
