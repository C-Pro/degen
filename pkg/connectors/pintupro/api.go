package pintupro

import (
	"context"
	"time"

	"degen/pkg/models"

	"github.com/shopspring/decimal"
)

type API struct {
	key     string
	secret  string
	baseURL string
}

func NewAPI(key, secret, baseURL string) *API {
	return &API{
		key:     key,
		secret:  secret,
		baseURL: baseURL,
	}
}

func (api *API) GetAccountInfo(_ context.Context) (*models.AccountInfo, error) {
	return &models.AccountInfo{
		Balances: map[string]models.Balance{
			"BTC": {
				Total:     decimal.RequireFromString("0.1"),
				Available: decimal.RequireFromString("0.0"),
				UpdatedAt: time.Now(),
			},
			"USDT": {
				Total:     decimal.RequireFromString("1000.0"),
				Available: decimal.RequireFromString("0.0"),
				UpdatedAt: time.Now(),
			},
		},
		Positions: map[string]models.Position{
			"BTCUSDTPERP": {
				Amount:     decimal.RequireFromString("0.01"),
				EntryPrice: decimal.RequireFromString("50000.0"),
				UpdatedAt:  time.Now(),
			},
		},
	}, nil
}
