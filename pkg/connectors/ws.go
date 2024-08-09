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

const pingInterval = time.Second * 15

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
	lastPing := time.Now()
	for {
		typ, msg, err := ws.conn.ReadMessage()
		if err != nil {
			ws.connCancel()
			return fmt.Errorf("websocket.Read error: %v", err)
		}

		if typ == websocket.PingMessage {
			if err := ws.conn.WriteMessage(websocket.PongMessage, msg); err != nil {
				ws.connCancel()
				return fmt.Errorf("websocket.Pong error: %v", err)
			}
			continue
		}

		if time.Since(lastPing) > pingInterval {
			if err := ws.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				ws.connCancel()
				return fmt.Errorf("websocket.Ping error: %v", err)
			}
			lastPing = time.Now()
		}

		// log.Printf("WS IN: %s", string(msg))

		ch <- msg

		select {
		case <-ws.connCtx.Done():
			return ws.conn.Close()
		default:
		}
	}
}

func (ws *WS) Write(ctx context.Context, msg []byte) error {
	ws.mux.Lock()
	defer ws.mux.Unlock()
	// log.Printf("WS OUT: %s", string(msg))

	return ws.conn.WriteMessage(websocket.TextMessage, msg)
}

func (ws *WS) Close() {
	ws.mux.Lock()
	defer ws.mux.Unlock()
	ws.connCancel()
}
