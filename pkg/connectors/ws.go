package connectors

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/pkg/errors"
	"nhooyr.io/websocket"
)

type WS struct {
	conn *websocket.Conn
}

func (ws *WS) Connect(ctx context.Context, url string) error {
	conn, resp, err := websocket.Dial(context.Background(), url, nil)
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
		_, msg, err := ws.conn.Read(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}

			return fmt.Errorf("websocket.Read error: %v", err)
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
	return ws.conn.Write(ctx, websocket.MessageText, msg)
}
