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
				Empty []string
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
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := paramsToString(tt.params)
			if got != tt.want {
				t.Errorf("paramsToString() = %s, want %s", got, tt.want)
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
