package pintupro

import (
	"context"
	"encoding/json"
	"log"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

func TestPintuProWSLive(t *testing.T) {
	url := "wss://stream.pintu.pro"
	log.Printf("[TEST] Connecting to %s...", url)

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("Dial error: %v", err)
	}
	defer conn.Close()
	log.Println("[TEST] Connected!")

	// Send subscription request
	subReq := Envelope{
		RequestID: uuid.NewString(),
		Method:    "subscribe",
		Params:    map[string]interface{}{"channels": []string{"aggrbook.snapshot.1.WLD-IDR"}},
		Timestamp: time.Now().UnixMilli(),
	}
	b, err := json.Marshal(subReq)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	log.Printf("[TEST] Sending subscription request: %s", string(b))
	if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Read messages
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				_, msg, err := conn.ReadMessage()
				if err != nil {
					log.Printf("[TEST] ReadMessage error: %v", err)
					return
				}
				log.Printf("[TEST] Received msg: %s", string(msg))

				// Parse to check heartbeat
				var r wsMessage
				if err := json.Unmarshal(msg, &r); err == nil {
					log.Printf("[TEST] Parsed method: %q, channel: %q, code: %d, msg: %q", r.Method, r.Channel, r.Code, r.Message)
					if r.Method == "heartbeat-request" {
						resp := wsMessage{
							RequestID: r.RequestID,
							Timestamp: time.Now().UnixMilli(),
							Method:    "heartbeat-response",
						}
						respB, _ := json.Marshal(resp)
						log.Printf("[TEST] Responding to heartbeat-request: %s", string(respB))
						if err := conn.WriteMessage(websocket.TextMessage, respB); err != nil {
							log.Printf("[TEST] Failed to write heartbeat response: %v", err)
						}
					}
				}
			}
		}
	}()

	<-ctx.Done()
	<-done
}
