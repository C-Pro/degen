package pintupro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"degen/pkg/metrics"
	"degen/pkg/models"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const contentType = "application/json"

type API struct {
	key     string
	secret  string
	baseURL string
	cl      http.Client
}

func NewAPI(key, secret, baseURL string) *API {
	return &API{
		key:     key,
		secret:  secret,
		baseURL: baseURL,
		cl: http.Client{
			Timeout: 1 * time.Second,
		},
	}
}

// easyjson:json
type responseMessage struct {
	RequestID string `json:"request_id"`
	Timestamp int64  `json:"timestamp"`
	Method    string `json:"method"`
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Reason    string `json:"reason"`
	Data      any    `json:"data"`
}

// easyjson:json
type assetBalanceResponse struct {
	Balance   string `json:"balance"`
	Available string `json:"available"`
	Order     string `json:"order"`
}

// easyjson:json
type accountInfoResponse struct {
	Assets map[string]assetBalanceResponse `json:"assets"`
}

func (api *API) call(
	method string,
	params any,
	dest any,
) error {
	defer metrics.RecordRequestDuration(Name, method, time.Now())
	parts := strings.Split(method, "/")
	if len(parts) != 2 {
		return fmt.Errorf("pintupro.call: invalid method %q", method)
	}

	switch parts[0] {
	case "private":
		return api.callPrivate(method, params, dest)
	case "public":
		return api.callPublic(method, params, dest)
	default:
		return fmt.Errorf("pintupro.call: unknown method prefix %q", method)
	}
}

func (api *API) callPrivate(
	method string,
	params any,
	dest any,
) error {
	req := WrapAndSign(
		method,
		api.key,
		api.secret,
		uuid.NewString(),
		params,
		time.Now(),
	)
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}

	// fmt.Println(string(b))

	body := bytes.NewReader(b)
	apiURL, err := url.JoinPath(api.baseURL, "v1", method)
	if err != nil {
		return err
	}

	resp, err := api.cl.Post(apiURL, contentType, body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return err
	}

	return nil
}

func (api *API) callPublic(
	method string,
	paramsAny any,
	dest any,
) error {
	params, _ := paramsAny.(url.Values)

	apiURL, err := url.JoinPath(api.baseURL, "v1", method)
	if err != nil {
		return err
	}
	u, err := url.Parse(apiURL)
	if err != nil {
		return err
	}
	u.RawQuery = params.Encode()
	log.Println(u.String())

	resp, err := api.cl.Get(u.String())
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return err
	}

	return nil
}

func (api *API) GetAccountInfo(_ context.Context) (*models.AccountInfo, error) {
	var accountInfo accountInfoResponse
	resp := responseMessage{
		Data: &accountInfo,
	}
	if err := api.call("private/get-account-information", nil, &resp); err != nil {
		return nil, fmt.Errorf("pintupro.GetAccountInfo: %w", err)
	}

	if resp.Code != 0 {
		return nil,
			fmt.Errorf("pintupro.GetAccountInfo: unexpected code %d %s %s",
				resp.Code, resp.Message, resp.Reason,
			)
	}

	result := models.AccountInfo{
		UpdatedAt: tsToTime(resp.Timestamp),
		Balances:  make(map[string]models.Balance),
		Positions: make(map[string]models.Position),
	}

	for asset, rec := range accountInfo.Assets {
		balance, _ := decimal.NewFromString(rec.Balance)
		available, _ := decimal.NewFromString(rec.Available)

		result.Balances[asset] = models.Balance{
			Total:     balance,
			Available: available,
			UpdatedAt: result.UpdatedAt,
		}

		// Treating spot asset balances as long positions.
		bbo, err := api.GetBBO(context.Background(), asset+"-IDR")
		if err != nil {
			// If we can't get the BBO, just skip this asset.
			continue
		}

		// Assume that the position avg price is the buy price of the BBO.
		if bbo.Bid.Price.IsPositive() {
			result.Positions[asset+"-IDR"] = models.Position{
				Amount:       balance,
				AveragePrice: bbo.Bid.Price,
				UpdatedAt:    result.UpdatedAt,
			}
		}

	}

	return &result, nil
}

// GetBBO returns the order book BBO for the given symbol.
func (api *API) GetBBO(
	_ context.Context,
	symbol string,
) (*models.BBO, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("depth", fmt.Sprintf("%d", 1))

	var ob orderBookMsg
	resp := responseMessage{
		Data: &ob,
	}
	if err := api.call("public/get-book", params, &resp); err != nil {
		return nil, fmt.Errorf("pintupro.GetBBO: %w", err)
	}

	if resp.Code != 0 {
		return nil,
			fmt.Errorf("pintupro.GetBBO: unexpected code %d %s %s",
				resp.Code, resp.Message, resp.Reason,
			)
	}

	// For now I don't care much about depth, so I will just take the first level.
	bbo := models.BBO{
		Timestamp: tsToTime(resp.Timestamp),
		Bid:       models.PriceLevel{},
		Ask:       models.PriceLevel{},
	}

	if len(ob.Bids) > 0 {
		bbo.Bid.Price, _ = decimal.NewFromString(ob.Bids[0][0])
		bbo.Bid.Size, _ = decimal.NewFromString(ob.Bids[0][1])
	}

	if len(ob.Asks) > 0 {
		bbo.Ask.Price, _ = decimal.NewFromString(ob.Asks[0][0])
		bbo.Ask.Size, _ = decimal.NewFromString(ob.Asks[0][1])
	}

	return &bbo, nil
}

// easyjson:json
type symbolsReferenceResponse struct {
	Symbols []struct {
		Symbol           string          `json:"symbol"`
		QuoteAsset       string          `json:"quote_asset"`
		BaseAsset        string          `json:"base_asset"`
		MaxSize          decimal.Decimal `json:"max_size"`
		MinSize          decimal.Decimal `json:"min_size"`
		PriceTickSize    decimal.Decimal `json:"price_tick_size"`
		QuantityTickSize decimal.Decimal `json:"quantity_tick_size"`
	} `json:"symbols"`
}

func (api *API) GetSymbols(_ context.Context) (map[string]models.SymbolInfo, error) {
	var data symbolsReferenceResponse
	resp := responseMessage{
		Data: &data,
	}
	if err := api.call("public/get-symbols-reference", nil, &resp); err != nil {
		return nil, fmt.Errorf("pintupro.GetSymbols: %w", err)
	}

	if resp.Code != 0 {
		return nil,
			fmt.Errorf("pintupro.GetSymbols: unexpected code %d %s %s",
				resp.Code, resp.Message, resp.Reason,
			)
	}

	result := make(map[string]models.SymbolInfo, len(data.Symbols))

	for _, s := range data.Symbols {
		result[s.Symbol] = models.SymbolInfo{
			Symbol:           s.Symbol,
			Base:             s.BaseAsset,
			Quote:            s.QuoteAsset,
			PriceTickSize:    s.PriceTickSize,
			QuantityTickSize: s.QuantityTickSize,
			MinQuantity:      s.MinSize,
			MaxQuantity:      s.MaxSize,
		}
	}

	return result, nil
}
