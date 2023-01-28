package connectors

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"nhooyr.io/websocket"
)

func echo(ctx context.Context, c *websocket.Conn) error {
	typ, r, err := c.Reader(ctx)
	if err != nil {
		return err
	}

	w, err := c.Writer(ctx, typ)
	if err != nil {
		return err
	}

	_, err = io.Copy(w, r)
	if err != nil {
		return fmt.Errorf("failed to io.Copy: %w", err)
	}

	return w.Close()
}

func serve(ctx context.Context, t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer c.Close(websocket.StatusInternalError, "the sky is falling")

		for {
			err = echo(r.Context(), c)
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				return
			}
			if err != nil {
				t.Errorf("failed to echo with %v: %v", r.RemoteAddr, err)
				return
			}
		}
	}
}

func TestWS(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l, err := net.Listen("tcp", "127.0.0.1:")
	if err != nil {
		t.Fatal(err)
	}

	s := &http.Server{Handler: serve(ctx, t)}
	go func() {
		if err := s.Serve(l); err != nil {
			if err != http.ErrServerClosed {
				t.Fatalf("server returner error: %v", err)
			}
		}
	}()

	ws := &WS{}
	if err := ws.Connect(ctx, "ws://"+l.Addr().String()); err != nil {
		t.Fatalf("unexpected error in ws.Connect: %v", err)
	}

	ch := make(chan []byte)
	go func() {
		if err := ws.Listen(ctx, ch); err != nil {
			t.Fatalf("ws.Listen returned error: %v", err)
		}
	}()

	expected := []byte("test")
	if err := ws.Write(ctx, expected); err != nil {
		t.Fatalf("unexpected error in ws.Write: %v", err)
	}

	select {
	case msg := <-ch:
		if !bytes.Equal(expected, msg) {
			t.Errorf("expected to got %q, but got %q", string(expected), string(msg))
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for echo")
	}
}
