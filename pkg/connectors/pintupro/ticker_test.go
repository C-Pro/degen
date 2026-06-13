package pintupro

import (
	"encoding/json"
	"testing"
)

// TestTickersDataDecode verifies the 24h ticker payload decodes through the
// responseMessage `data` field (which only fills Data pointers implementing
// json.Unmarshaler — hence tickersData's custom UnmarshalJSON). The sample is a
// trimmed real response from public/get-tickers?interval=1d.
func TestTickersDataDecode(t *testing.T) {
	raw := `{"timestamp":1781365539994,"method":"public/get-tickers","code":0,"message":"OK","data":{"interval":"1d","tickers":{` +
		`"BTC-IDR":{"from":1781279195,"to":1781365595,"o":"1146512000","h":"1154999000","l":"1138008000","c":"1154999000","v":"4.6577","v_quote":"5342123004.9","change":"0.0074"},` +
		`"PEPE-IDR":{"o":"0.0505","h":"0.05169","l":"0.04996","c":"0.05169","v":"2999520663.7"}}}}`

	var data tickersData
	resp := responseMessage{Data: &data}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Code != 0 {
		t.Fatalf("code = %d (%s)", resp.Code, resp.Message)
	}
	if data.Interval != "1d" {
		t.Errorf("interval = %q, want 1d", data.Interval)
	}
	if len(data.Tickers) != 2 {
		t.Fatalf("got %d tickers, want 2", len(data.Tickers))
	}

	e, ok := data.Tickers["BTC-IDR"]
	if !ok {
		t.Fatal("BTC-IDR missing")
	}
	if e.Open.String() != "1146512000" || e.High.String() != "1154999000" ||
		e.Low.String() != "1138008000" || e.Close.String() != "1154999000" {
		t.Errorf("BTC-IDR OHLC: o=%s h=%s l=%s c=%s", e.Open, e.High, e.Low, e.Close)
	}
	if e.High.LessThanOrEqual(e.Low) {
		t.Errorf("BTC-IDR high %s !> low %s", e.High, e.Low)
	}

	if p := data.Tickers["PEPE-IDR"]; p.Low.String() != "0.04996" {
		t.Errorf("PEPE-IDR low = %s, want 0.04996", p.Low)
	}
}
