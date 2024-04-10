package connectors

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func serve(ctx context.Context, t *testing.T) http.HandlerFunc {
	upgrader := websocket.Upgrader{}
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer c.Close()
		for {
			mt, message, err := c.ReadMessage()
			if err != nil {
				t.Errorf("read: %v", err)
				break
			}
			t.Logf("recv: %s", message)
			err = c.WriteMessage(mt, message)
			if err != nil {
				t.Errorf("write: %v", err)
				break
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
				t.Errorf("server returner error: %v", err)
				return
			}
		}
	}()

	ws := &WS{}
	if err := ws.Connect(ctx, "ws://"+l.Addr().String()); err != nil {
		t.Fatalf("unexpected error in ws.Connect: %v", err)
	}

	ch := make(chan []byte)
	go func() {
		if err := ws.Listen(ch); err != nil {
			t.Errorf("ws.Listen returned error: %v", err)
			return
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
