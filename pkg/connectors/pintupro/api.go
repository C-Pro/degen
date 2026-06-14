package pintupro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"degen/pkg/metrics"
	"degen/pkg/models"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const contentType = "application/json"

const (
	// readTimeout bounds idempotent read requests.
	readTimeout = 3 * time.Second
	// orderTimeout bounds order-mutating requests. It is deliberately more
	// generous than reads: a place/cancel that actually executes on the
	// exchange but responds slowly must not be aborted client-side and then
	// assumed not to have happened. [H8]
	orderTimeout = 5 * time.Second
)

// orderMutating reports whether a method places/cancels orders (non-idempotent).
func orderMutating(method string) bool {
	switch method {
	case "private/place-order", "private/cancel-order", "private/cancel-all-orders":
		return true
	}

	return false
}

// checkHTTPStatus turns a non-2xx HTTP response into an error so that gateway
// errors (429/5xx, HTML bodies) are not silently decoded as an empty success. [L6]
func checkHTTPStatus(resp *http.Response) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("unexpected HTTP status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

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
			// Generous backstop only; the real per-call deadline is applied via
			// context in call() so reads and order mutations can differ. [H8]
			Timeout: 15 * time.Second,
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
	ctx context.Context,
	method string,
	params any,
	dest any,
) error {
	defer metrics.RecordRequestDuration(Name, method, time.Now())
	parts := strings.Split(method, "/")
	if len(parts) != 2 {
		return fmt.Errorf("pintupro.call: invalid method %q", method)
	}

	// Apply a per-call deadline (honoring any earlier deadline on ctx) so the
	// context is actually respected and order mutations get a longer budget
	// than reads. [L7][H8]
	timeout := readTimeout
	if orderMutating(method) {
		timeout = orderTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch parts[0] {
	case "private":
		return api.callPrivate(ctx, method, params, dest)
	case "public":
		return api.callPublic(ctx, method, params, dest)
	default:
		return fmt.Errorf("pintupro.call: unknown method prefix %q", method)
	}
}

func (api *API) callPrivate(
	ctx context.Context,
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

	apiURL, err := url.JoinPath(api.baseURL, "v1", method)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", contentType)

	resp, err := api.cl.Do(httpReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if err := checkHTTPStatus(resp); err != nil {
		return err
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return err
	}

	return nil
}

func (api *API) callPublic(
	ctx context.Context,
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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}

	resp, err := api.cl.Do(httpReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if err := checkHTTPStatus(resp); err != nil {
		return err
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return err
	}

	return nil
}

func (api *API) GetAccountInfo(ctx context.Context) (*models.AccountInfo, error) {
	var accountInfo accountInfoResponse
	resp := responseMessage{
		Data: &accountInfo,
	}
	if err := api.call(ctx, "private/get-account-information", nil, &resp); err != nil {
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
		// Do not silently swallow parse errors: log them so a malformed/empty
		// balance string is visible rather than masquerading as zero. [M10]
		balance, err := decimal.NewFromString(rec.Balance)
		if err != nil {
			log.Printf("pintupro.GetAccountInfo: bad balance %q for %s: %v", rec.Balance, asset, err)
		}
		available, err := decimal.NewFromString(rec.Available)
		if err != nil {
			log.Printf("pintupro.GetAccountInfo: bad available %q for %s: %v", rec.Available, asset, err)
		}

		result.Balances[asset] = models.Balance{
			Total:     balance,
			Available: available,
			UpdatedAt: result.UpdatedAt,
		}

		// Treating spot asset balances as long positions.
		bbo, err := api.GetBBO(ctx, asset+"-IDR")
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
	ctx context.Context,
	symbol string,
) (*models.BBO, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("depth", fmt.Sprintf("%d", 1))

	var ob orderBookMsg
	resp := responseMessage{
		Data: &ob,
	}
	if err := api.call(ctx, "public/get-book", params, &resp); err != nil {
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

// Ticker24h is a symbol's daily (1d) OHLC snapshot from public/get-tickers.
type Ticker24h struct {
	Symbol string
	Open   decimal.Decimal
	High   decimal.Decimal
	Low    decimal.Decimal
	Close  decimal.Decimal
	Volume decimal.Decimal
}

// tickerEntry is a single instrument's entry in the get-tickers response.
type tickerEntry struct {
	Open   decimal.Decimal `json:"o"`
	High   decimal.Decimal `json:"h"`
	Low    decimal.Decimal `json:"l"`
	Close  decimal.Decimal `json:"c"`
	Volume decimal.Decimal `json:"v"`
}

// tickersData is the `data` payload of public/get-tickers: an interval label and
// a map of instrument name -> OHLC entry.
type tickersData struct {
	Interval string                 `json:"interval"`
	Tickers  map[string]tickerEntry `json:"tickers"`
}

// UnmarshalJSON is required so tickersData decodes through responseMessage's
// `data` field: that (easyjson) decoder only fills a Data pointer that
// implements easyjson/json Unmarshaler, otherwise it falls back to a generic
// map and leaves this struct empty. The alias breaks the recursion.
func (t *tickersData) UnmarshalJSON(b []byte) error {
	type alias tickersData
	return json.Unmarshal(b, (*alias)(t))
}

// Get24hTicker returns the 24h (1d) OHLC for a single symbol. The endpoint
// returns every instrument keyed by name (the instrument_name parameter is
// ignored by the API), so the requested symbol is selected client-side.
func (api *API) Get24hTicker(ctx context.Context, symbol string) (*Ticker24h, error) {
	params := url.Values{}
	params.Set("interval", "1d")
	params.Set("instrument_name", symbol)

	var data tickersData
	resp := responseMessage{Data: &data}
	if err := api.call(ctx, "public/get-tickers", params, &resp); err != nil {
		return nil, fmt.Errorf("pintupro.Get24hTicker: %w", err)
	}

	if resp.Code != 0 {
		return nil,
			fmt.Errorf("pintupro.Get24hTicker: unexpected code %d %s %s",
				resp.Code, resp.Message, resp.Reason,
			)
	}

	e, ok := data.Tickers[symbol]
	if !ok {
		return nil, fmt.Errorf("pintupro.Get24hTicker: symbol %q not found", symbol)
	}

	return &Ticker24h{
		Symbol: symbol,
		Open:   e.Open,
		High:   e.High,
		Low:    e.Low,
		Close:  e.Close,
		Volume: e.Volume,
	}, nil
}

// Candlestick is one OHLC bar from public/get-candlesticks.
type Candlestick struct {
	From   int64 // unix seconds, bar start
	To     int64 // unix seconds, bar end
	Open   decimal.Decimal
	High   decimal.Decimal
	Low    decimal.Decimal
	Close  decimal.Decimal
	Volume decimal.Decimal
}

type candlestickEntry struct {
	From   int64           `json:"from"`
	To     int64           `json:"to"`
	Open   decimal.Decimal `json:"o"`
	High   decimal.Decimal `json:"h"`
	Low    decimal.Decimal `json:"l"`
	Close  decimal.Decimal `json:"c"`
	Volume decimal.Decimal `json:"v"`
}

type candlesticksData struct {
	Symbol       string             `json:"symbol"`
	Interval     string             `json:"interval"`
	Candlesticks []candlestickEntry `json:"candlesticks"`
}

// UnmarshalJSON lets candlesticksData decode through responseMessage's `data`
// field (which only fills Data pointers implementing json/easyjson Unmarshaler).
func (d *candlesticksData) UnmarshalJSON(b []byte) error {
	type alias candlesticksData
	return json.Unmarshal(b, (*alias)(d))
}

// GetCandlesticks returns the OHLC history for a symbol at the given interval
// (e.g. "1m", "15m", "1h"), sorted oldest-first. The endpoint returns a fixed
// recent window (~100 bars); there is no count parameter.
func (api *API) GetCandlesticks(ctx context.Context, symbol, interval string) ([]Candlestick, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("interval", interval)

	var data candlesticksData
	resp := responseMessage{Data: &data}
	if err := api.call(ctx, "public/get-candlesticks", params, &resp); err != nil {
		return nil, fmt.Errorf("pintupro.GetCandlesticks: %w", err)
	}

	if resp.Code != 0 {
		return nil,
			fmt.Errorf("pintupro.GetCandlesticks: unexpected code %d %s %s",
				resp.Code, resp.Message, resp.Reason,
			)
	}

	out := make([]Candlestick, 0, len(data.Candlesticks))
	for _, c := range data.Candlesticks {
		out = append(out, Candlestick{
			From: c.From, To: c.To,
			Open: c.Open, High: c.High, Low: c.Low, Close: c.Close, Volume: c.Volume,
		})
	}
	// API returns newest-first; replay needs oldest-first.
	sort.Slice(out, func(i, j int) bool { return out[i].From < out[j].From })

	return out, nil
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

func (api *API) GetSymbols(ctx context.Context) (map[string]models.SymbolInfo, error) {
	var data symbolsReferenceResponse
	resp := responseMessage{
		Data: &data,
	}
	if err := api.call(ctx, "public/get-symbols-reference", nil, &resp); err != nil {
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
