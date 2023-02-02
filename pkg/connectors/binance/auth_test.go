package binance

import (
	"strconv"
	"testing"
	"time"
)

func TestSign(t *testing.T) {
	cases := []struct {
		name     string
		secret   string
		query    string
		body     string
		ts       string
		retQuery string
	}{
		{
			"Example 1: As a request body",
			"NhqPtmdSJYdKjVHjA7PZj4Mge3R5YNiP1e3UZjInClVN65XAbvqqM6A7H5fATj0j",
			"",
			"symbol=LTCBTC&side=BUY&type=LIMIT&timeInForce=GTC&quantity=1&price=0.1&recvWindow=5000",
			"1499827319559",
			"timestamp=1499827319559&signature=4ec8fd8ed2512d79b71064a37c946eb99700673bf09e9feafe7cd5f968455b17",
		},
		{
			"Example 2: As a query string",
			"NhqPtmdSJYdKjVHjA7PZj4Mge3R5YNiP1e3UZjInClVN65XAbvqqM6A7H5fATj0j",
			"symbol=LTCBTC&side=BUY&type=LIMIT&timeInForce=GTC&quantity=1&price=0.1&recvWindow=5000",
			"",
			"1499827319559",
			"symbol=LTCBTC&side=BUY&type=LIMIT&timeInForce=GTC&quantity=1&price=0.1&recvWindow=5000&timestamp=1499827319559&signature=c8db56825ae71d6d79447849e617115f4a920fa2acdcab2b053c4b2838bd6b71",
		},
		{
			"Example 3: Mixed query string and request body",
			"NhqPtmdSJYdKjVHjA7PZj4Mge3R5YNiP1e3UZjInClVN65XAbvqqM6A7H5fATj0j",
			"symbol=LTCBTC&side=BUY&type=LIMIT&timeInForce=GTC",
			"quantity=1&price=0.1&recvWindow=5000",
			"1499827319559",
			"symbol=LTCBTC&side=BUY&type=LIMIT&timeInForce=GTC&timestamp=1499827319559&signature=9fe05e0baad80489b9402396276418ebdca1519e3b68c021d18babcbadee7194",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ms, err := strconv.ParseInt(c.ts, 10, 64)
			if err != nil {
				t.Fatalf("bad timestamp: %v", c.ts)
			}
			ts := time.Unix(0, ms*1000*1000)
			query := signRequest(c.secret, c.query, c.body, ts)
			if query != c.retQuery {
				t.Errorf("expected %q query, got %q", c.retQuery, query)
			}
		})
	}
}
