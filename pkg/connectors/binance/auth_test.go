package binance

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSign(t *testing.T) {
	cases := []struct {
		name      string
		secret    string
		query     string
		body      string
		ts        string
		signature string
	}{
		{
			"Example 1: As a request body",
			"NhqPtmdSJYdKjVHjA7PZj4Mge3R5YNiP1e3UZjInClVN65XAbvqqM6A7H5fATj0j",
			"",
			"symbol=LTCBTC&side=BUY&type=LIMIT&timeInForce=GTC&quantity=1&price=0.1&recvWindow=5000",
			"1499827319559",
			"c8db56825ae71d6d79447849e617115f4a920fa2acdcab2b053c4b2838bd6b71",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
		ms, err := strconv.ParseInt(c.ts, 10, 64)
		if err != nil {
			t.Fatalf("bad timestamp: %v", c.ts)
		}
		ts := time.Unix(0, ms * 1000 * 1000)
		signature := signRequest(c.secret, c.query, c.body, ts)
		if !strings.EqualFold(signature, c.signature) {
			t.Errorf("expected %q signature, got %q", c.signature, signature)
		}
	})

	}
}
