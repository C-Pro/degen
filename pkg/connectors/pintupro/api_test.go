package pintupro

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Regression for L6: a non-2xx HTTP response must surface as an error rather
// than being decoded as an empty (Code==0) success.
func TestAPI_Non2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("upstream down"))
	}))
	defer srv.Close()

	api := NewAPI("", "", srv.URL)
	if _, err := api.GetBBO(context.Background(), "WLD-IDR"); err == nil {
		t.Fatal("expected an error for HTTP 503, got nil")
	}
}

// Regression for L7: the context passed by the caller must actually be honored
// (the old code ignored ctx entirely).
func TestAPI_ContextHonored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	api := NewAPI("", "", srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before the call

	if _, err := api.GetBBO(ctx, "WLD-IDR"); err == nil {
		t.Fatal("expected an error for a cancelled context, got nil")
	}
}
