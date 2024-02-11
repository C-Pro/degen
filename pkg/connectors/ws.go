package connectors

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

type WS struct {
	conn       *websocket.Conn
	connCtx    context.Context
	connCancel context.CancelFunc
	mux        sync.Mutex
}

func (ws *WS) Connect(ctx context.Context, url string) error {
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		if resp != nil {
			body, errR := io.ReadAll(resp.Body)
			if errR != nil {
				return fmt.Errorf("failed to read websocket connect response: %w", err)
			}

			err = errors.Wrapf(err, "got message connecting to ws: %q", string(body))
		}

		return err
	}

	ws.conn = conn
	ws.connCtx, ws.connCancel = context.WithCancel(ctx)
	return nil
}

func (ws *WS) Listen(ch chan<- []byte) error {
	for {
		typ, msg, err := ws.conn.ReadMessage()
		if err != nil {
			ws.connCancel()
			return fmt.Errorf("websocket.Read error: %v", err)
		}

		if typ == websocket.PingMessage {
			//nolint:errcheck
			ws.conn.WriteMessage(websocket.PongMessage, msg)
			continue
		}

		ch <- msg

		select {
		case <-ws.connCtx.Done():
			return ws.conn.Close()
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func (ws *WS) Write(ctx context.Context, msg []byte) error {
	ws.mux.Lock()
	defer ws.mux.Unlock()
	return ws.conn.WriteMessage(websocket.TextMessage, msg)
}

func (ws *WS) Close() {
	ws.mux.Lock()
	defer ws.mux.Unlock()
	ws.connCancel()
}
