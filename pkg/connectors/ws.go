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

type wsFrame struct {
	messageType int
	data        []byte
}

type WS struct {
	mu         sync.RWMutex
	conn       *websocket.Conn
	connCtx    context.Context
	connCancel context.CancelFunc
	writeCh    chan wsFrame
}

func (ws *WS) Connect(ctx context.Context, url string) error {
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil) // nosemgrep
	if err != nil {
		if resp != nil {
			body, errR := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if errR != nil {
				return fmt.Errorf("failed to read websocket connect response: %w", err)
			}

			err = errors.Wrapf(err, "got message connecting to ws: %q", string(body))
		}

		return err
	}

	ws.mu.Lock()
	ws.conn = conn
	ws.connCtx, ws.connCancel = context.WithCancel(ctx)
	ws.writeCh = make(chan wsFrame, 100)
	connCtx := ws.connCtx
	writeCh := ws.writeCh
	connCancel := ws.connCancel
	ws.mu.Unlock()

	conn.SetPingHandler(func(appData string) error {
		select {
		case <-connCtx.Done():
			return nil
		case writeCh <- wsFrame{messageType: websocket.PongMessage, data: []byte(appData)}:
			return nil
		}
	})

	go ws.startWriteLoop(conn, connCtx, writeCh, connCancel)
	return nil
}

func (ws *WS) startWriteLoop(conn *websocket.Conn, connCtx context.Context, writeCh chan wsFrame, connCancel context.CancelFunc) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-connCtx.Done():
			return
		case frame := <-writeCh:
			if err := conn.WriteMessage(frame.messageType, frame.data); err != nil {
				connCancel()
				return
			}
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				connCancel()
				return
			}
		}
	}
}

func (ws *WS) Listen(ch chan<- []byte) error {
	// Capture this connection generation's conn/ctx/cancel together. On a read
	// error we must cancel THIS generation only — reading ws.connCancel fresh
	// could cancel a newer connection installed by a concurrent reconnect. [H1]
	ws.mu.RLock()
	conn := ws.conn
	connCtx := ws.connCtx
	connCancel := ws.connCancel
	ws.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if connCancel != nil {
				connCancel()
			}
			return fmt.Errorf("websocket.Read error: %v", err)
		}

		// log.Printf("WS IN: %s", string(msg))

		// Use a selectable send so a slow/stalled consumer cannot park this
		// goroutine forever and ignore connection cancellation. [#34]
		select {
		case ch <- msg:
		case <-connCtx.Done():
			return conn.Close()
		}
	}
}

func (ws *WS) Write(ctx context.Context, msg []byte) error {
	ws.mu.RLock()
	connCtx := ws.connCtx
	writeCh := ws.writeCh
	ws.mu.RUnlock()

	if connCtx == nil || writeCh == nil {
		return fmt.Errorf("websocket closed")
	}

	select {
	case <-connCtx.Done():
		return fmt.Errorf("websocket closed")
	case <-ctx.Done():
		return ctx.Err()
	case writeCh <- wsFrame{messageType: websocket.TextMessage, data: msg}:
		return nil
	}
}

func (ws *WS) Close() {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if ws.connCancel != nil {
		ws.connCancel()
	}
	if ws.conn != nil {
		_ = ws.conn.Close()
	}
}
