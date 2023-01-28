package binance

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"
)

// signRequests implements this stuff:
// https://binance-docs.github.io/apidocs/spot/en/#signed-trade-user_data-and-margin-endpoint-security
func signRequest(secret, queryString, body string, ts time.Time) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(queryString))
	h.Write([]byte(body))
	if len(queryString)+len(body) > 0 {
		h.Write([]byte("&"))
	}
	h.Write([]byte("timestamp="))
	h.Write([]byte(strconv.FormatInt(ts.UnixMilli(), 10)))

	return hex.EncodeToString(h.Sum(nil))
}
