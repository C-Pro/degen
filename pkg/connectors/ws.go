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
	conn *websocket.Conn
	mux  sync.Mutex
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
	return nil
}

func (ws *WS) Listen(ctx context.Context, ch chan<- []byte) error {
	for {
		typ, msg, err := ws.conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("websocket.Read error: %v", err)
		}

		if typ == websocket.PingMessage {
			ws.conn.WriteMessage(websocket.PongMessage, msg)
			continue
		}

		ch <- msg

		select {
		case <-ctx.Done():
			return nil
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
