package pintupro

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestParamsToString(t *testing.T) {
	cases := []struct {
		name   string
		params any
		want   string
	}{
		{
			name:   "empty",
			params: nil,
			want:   "",
		},
		{
			name: "emptyString",
			params: struct {
				S string `json:"s"`
			}{S: ""},
			want: "s",
		},
		{
			name: "simple struct",
			params: struct {
				A int    `json:"batur"`
				B string `json:"agung"`
				C string `json:"-"`
			}{
				A: 1717,
				B: "tinggi",
			},
			want: "agungtinggibatur1717",
		},
		{
			name: "arrays",
			params: struct {
				List1 []int    `json:"theList"`
				List2 []string `json:"anotherOne"`
				Empty []string `json:"empty,omitempty"`
			}{
				List1: []int{3, 4, 5},
				List2: []string{"c", "b", "a"},
			},
			want: "anotherOnecbatheList345",
		},
		{
			name: "omitempty",
			params: struct {
				A int     `json:"test,omitempty"`
				B float64 `json:"best,omitempty"`
				X int     `json:"x"`
				Y float64 `json:"y"`
			}{
				A: 0,
				B: 0,
				X: 0,
				Y: 0,
			},
			want: "x0y0",
		},
		{
			name: "nested",
			params: struct {
				Obj struct {
					A int    `json:"a"`
					B string `json:"b"`
				} `json:"obj"`
				Obj2 []struct{ C string } `json:"obj2"`
			}{
				Obj: struct {
					A int    `json:"a"`
					B string `json:"b"`
				}{
					A: 1,
					B: "2",
				},
				Obj2: []struct{ C string }{
					{C: "test"},
					{C: "best"},
				},
			},
			want: "obja1b2obj2CtestCbest",
		},
		{
			name: "example from docs",
			params: struct {
				Price       string `json:"price"`
				Side        string `json:"side"`
				Size        string `json:"size"`
				Symbol      string `json:"symbol"`
				TimeInForce string `json:"time_in_force"`
				OrderType   string `json:"type"`
			}{
				Price:       "1015979000",
				Side:        "BUY",
				Size:        "0.001",
				Symbol:      "BTC-IDR",
				TimeInForce: "GTC",
				OrderType:   "LIMIT",
			},
			want: "price1015979000sideBUYsize0.001symbolBTC-IDRtime_in_forceGTCtypeLIMIT",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := paramsToString(tt.params)
			if got != tt.want {
				t.Errorf("paramsToString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWrapAndSign(t *testing.T) {
	cases := []struct {
		name      string
		method    string
		key       string
		secret    string
		requestID string
		params    any
		ts        time.Time
		expected  Envelope
	}{
		{
			name:      "place order 1",
			method:    "private/place-order",
			key:       "abc0",
			secret:    "abc",
			requestID: "9f53b794-ae60-4155-b7df-b65c691535f4",
			params: map[string]interface{}{
				"symbol":          "BTC-IDR",
				"side":            "SELL",
				"type":            "LIMIT",
				"price":           "1003716000",
				"size":            "0.01",
				"client_order_id": "426560af-25ce-4b58-a400-30cd2f0a841c",
				"time_in_force":   "GTC",
				"exec_inst":       "POST_ONLY",
			},
			ts: time.Unix(0, 1719733351080*int64(time.Millisecond)),
			expected: Envelope{
				RequestID: "9f53b794-ae60-4155-b7df-b65c691535f4",
				Timestamp: 1719733351080,
				Method:    "private/place-order",
				Params: map[string]interface{}{
					"symbol":          "BTC-IDR",
					"side":            "SELL",
					"type":            "LIMIT",
					"price":           "1003716000",
					"size":            "0.01",
					"client_order_id": "426560af-25ce-4b58-a400-30cd2f0a841c",
					"time_in_force":   "GTC",
					"exec_inst":       "POST_ONLY",
				},
				Signature: "bed43b83fc8663f49b8d2a33224d87fe6dbacd4b664def8512ffe84683b1dcd7",
				APIKey:    "abc0",
			},
		},
		{
			name:      "simple",
			method:    "private/whatever",
			key:       "key",
			secret:    "secret",
			requestID: "req",
			params: struct {
				Price string `json:"price"`
				Size  string `json:"size"`
			}{
				Price: "100",
				Size:  "10",
			},
			ts: time.Unix(1715512719, 0),
			expected: Envelope{
				RequestID: "req",
				Timestamp: 1715512719000,
				Method:    "private/whatever",
				Params: struct {
					Price string `json:"price"`
					Size  string `json:"size"`
				}{
					Price: "100",
					Size:  "10",
				},
				Signature: "dfccf903072ab73d5092bdfb86155f22f85d68e58b73aa6c4c9556edf7e9c901",
				APIKey:    "key",
			},
		},
		{
			name:      "example from docs (place-order)",
			method:    "private/place-order",
			key:       "abc0",
			secret:    "abc",
			requestID: "fc9f3e2e-6791-49ac-af23-715fccac13dd",
			params: struct {
				Price       string `json:"price"`
				Side        string `json:"side"`
				Size        string `json:"size"`
				Symbol      string `json:"symbol"`
				TimeInForce string `json:"time_in_force"`
				OrderType   string `json:"type"`
			}{
				Price:       "1015979000",
				Side:        "BUY",
				Size:        "0.001",
				Symbol:      "BTC-IDR",
				TimeInForce: "GTC",
				OrderType:   "LIMIT",
			},
			ts: time.Unix(0, 1719295943513*int64(time.Millisecond)),
			expected: Envelope{
				RequestID: "fc9f3e2e-6791-49ac-af23-715fccac13dd",
				Timestamp: 1719295943513,
				Method:    "private/place-order",
				Params: struct {
					Price       string `json:"price"`
					Side        string `json:"side"`
					Size        string `json:"size"`
					Symbol      string `json:"symbol"`
					TimeInForce string `json:"time_in_force"`
					OrderType   string `json:"type"`
				}{
					Price:       "1015979000",
					Side:        "BUY",
					Size:        "0.001",
					Symbol:      "BTC-IDR",
					TimeInForce: "GTC",
					OrderType:   "LIMIT",
				},
				Signature: "2094afd679b8a8afe1a74350aa5a3f05329309ce48ac9a4f05e28750be2c4ed4",
				APIKey:    "abc0",
			},
		},
		{
			name:      "example from docs (ws auth)",
			method:    "public/auth",
			key:       "abc0",
			secret:    "abc",
			requestID: "837873eb-0d68-457b-860f-a853046455cf",
			params:    nil,
			ts:        time.Unix(0, 1719306245083*int64(time.Millisecond)),
			expected: Envelope{
				RequestID: "837873eb-0d68-457b-860f-a853046455cf",
				Timestamp: 1719306245083,
				Method:    "public/auth",
				Params:    nil,
				Signature: "6d13d543b959454eb6b05ac5d3722aad3d9cd7a887d5083fb1818d18e34cff36",
				APIKey:    "abc0",
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapAndSign(tt.method, tt.key, tt.secret, tt.requestID, tt.params, tt.ts)
			if diff := cmp.Diff(got, &tt.expected); diff != "" {
				t.Errorf("WrapAndSign() mismatch (-got +want):\n%s", diff)
			}
		})
	}
}
