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
	if len(queryString) > 0 {
		queryString = queryString + "&timestamp=" + strconv.FormatInt(ts.UnixMilli(), 10)
	} else {
		queryString = "timestamp=" + strconv.FormatInt(ts.UnixMilli(), 10)
	}
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(queryString))
	h.Write([]byte(body))

	return queryString + "&signature=" + hex.EncodeToString(h.Sum(nil))
}
