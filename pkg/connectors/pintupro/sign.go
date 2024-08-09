package pintupro

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// easyjson:json
// Envelope is a wrapper for private (signed API) requests.
type Envelope struct {
	RequestID string `json:"request_id"`
	Timestamp int64  `json:"timestamp"`
	Method    string `json:"method"`
	Params    any    `json:"params"`
	Signature string `json:"signature"`
	APIKey    string `json:"api_key"`
}

// WrapAndSign wraps the request into an Envelope and signs it
// according to the request signature computation rules:
// https://docs.pintupro.com/#api-signature-computation
func WrapAndSign(
	method, key, secret, requestID string,
	params any,
	ts time.Time,
) *Envelope {
	paramString := paramsToString(params)
	sig := signature(requestID, method, key, paramString, secret, ts)

	return &Envelope{
		RequestID: requestID,
		Timestamp: ts.UnixMilli(),
		Method:    method,
		Params:    params,
		Signature: sig,
		APIKey:    key,
	}
}

func paramsToString(params any) string {
	v := reflect.ValueOf(params)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if s, ok := params.(fmt.Stringer); ok {
		return s.String()
	}

	var b strings.Builder
	switch v.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			b.WriteString(paramsToString(v.Index(i).Interface()))
		}
	case reflect.Map:
		keys := v.MapKeys()
		sort.Slice(keys, func(i, j int) bool {
			return keys[i].String() < keys[j].String()
		})
		for _, key := range keys {
			b.WriteString(key.String())
			b.WriteString(paramsToString(v.MapIndex(key).Interface()))
		}
	case reflect.Struct:
		// For structs we need to get field names as in json message
		// (honor tags) and sort them alphabetically.
		type fld struct {
			n string
			i int
		}
		t := v.Type()
		fields := make([]fld, 0, t.NumField())
		for i := 0; i < t.NumField(); i++ {
			ftyp := t.Field(i)
			// Skip unexported fields.
			if !unicode.IsUpper(rune(ftyp.Name[0])) {
				continue
			}

			fname := ftyp.Name
			if tag := ftyp.Tag.Get("json"); tag != "" {
				parts := strings.Split(tag, ",")
				fname = parts[0]
				// Skip fields that are not rendered in json.
				if fname == "-" {
					continue
				}
				if len(parts) > 1 && parts[1] == "omitempty" {
					if v.Field(i).IsZero() {
						continue
					}
				}
			}

			fields = append(fields, fld{fname, i})
		}
		sort.Slice(fields, func(i, j int) bool {
			return fields[i].n < fields[j].n
		})
		for _, fs := range fields {
			f := v.Field(fs.i)
			if f.Kind() == reflect.Ptr {
				f = f.Elem()
			}

			b.WriteString(fs.n)
			b.WriteString(paramsToString(f.Interface()))
		}
	default:
		if v.IsValid() {
			b.WriteString(fmt.Sprintf("%v", v.Interface()))
		}
	}

	return b.String()
}

/*
	sigPayload := fmt.Sprint(requestBody.RequestId) + fmt.Sprint(requestBody.Timestamp) + requestBody.Method + apiKey + paramsString
	sigHash := hmac.New(sha256.New, []byte(apiSecret))
	sigHash.Write([]byte(sigPayload))
	sigBytes := sigHash.Sum(nil)
	return hex.EncodeToString(sigBytes), nil
*/

func signature(requestID, method, key, paramString, secret string, ts time.Time) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(requestID))
	h.Write([]byte(strconv.FormatInt(ts.UnixMilli(), 10)))
	h.Write([]byte(method))
	h.Write([]byte(key))
	h.Write([]byte(paramString))

	return hex.EncodeToString(h.Sum(nil))
}
