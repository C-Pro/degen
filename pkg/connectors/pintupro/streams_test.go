package pintupro

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"degen/pkg/models"

	"github.com/gorilla/websocket"
)

func TestPintuProReconnectionAndSubscriptions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Upgrader for mock WebSocket server
	upgrader := websocket.Upgrader{}

	var (
		mu               sync.Mutex
		connectionsCount int64
		connectTimes     []time.Time
		subscribeCount   int64
		authCount        int64
		latestSubChannel string
	)

	// Mock WebSocket server handler
	handler := func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&connectionsCount, 1)
		mu.Lock()
		connectTimes = append(connectTimes, time.Now())
		mu.Unlock()

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("Upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var envelope struct {
				Method string `json:"method"`
				Params struct {
					Channels []string `json:"channels"`
				} `json:"params"`
				RequestID string `json:"request_id"`
			}
			if err := json.Unmarshal(msg, &envelope); err != nil {
				continue
			}

			if envelope.Method == "public/auth" {
				atomic.AddInt64(&authCount, 1)
				// Respond with success
				resp := wsMessage{
					RequestID: envelope.RequestID,
					Timestamp: time.Now().UnixMilli(),
					Method:    envelope.Method,
					Code:      0,
				}
				b, _ := json.Marshal(resp)
				_ = conn.WriteMessage(websocket.TextMessage, b)
			} else if envelope.Method == "subscribe" {
				atomic.AddInt64(&subscribeCount, 1)
				for _, ch := range envelope.Params.Channels {
					mu.Lock()
					latestSubChannel = ch
					mu.Unlock()
					// Respond with success
					resp := wsMessage{
						RequestID: envelope.RequestID,
						Timestamp: time.Now().UnixMilli(),
						Method:    "subscribe",
						Code:      0,
						Data:      json.RawMessage(`{"channel":"` + ch + `"}`),
					}
					b, _ := json.Marshal(resp)
					_ = conn.WriteMessage(websocket.TextMessage, b)
				}
			}
		}
	}

	// Start local server
	l, err := net.Listen("tcp", "127.0.0.1:")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	s := &http.Server{Handler: http.HandlerFunc(handler)}
	go func() {
		_ = s.Serve(l)
	}()

	wsURL := "ws://" + l.Addr().String()

	// 1. Initialize PintuPro
	p, err := NewPintuPro(ctx, "mock_key", "mock_secret", "http://mock-api", wsURL)
	if err != nil {
		t.Fatalf("Failed to initialize PintuPro: %v", err)
	}

	// Reduce idle timeout for faster testing
	p.idleTimeout = 2 * time.Second

	// 2. Start listening
	ch := make(chan models.ExchangeMessage, 100)
	go p.Listen(ctx, ch)

	// 3. Subscribe to a channel
	err = p.SubscribeBookTickers(ctx, []string{"WLD-IDR"})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Give it some time to receive subscription response
	time.Sleep(200 * time.Millisecond)

	if atomic.LoadInt64(&authCount) != 1 {
		t.Errorf("Expected exactly 1 auth, got %d", authCount)
	}
	if atomic.LoadInt64(&subscribeCount) != 1 {
		t.Errorf("Expected exactly 1 subscribe, got %d", subscribeCount)
	}
	mu.Lock()
	subChannel := latestSubChannel
	mu.Unlock()
	if subChannel != "aggrbook.snapshot.1.WLD-IDR" {
		t.Errorf("Expected subscription to WLD-IDR channel, got %s", subChannel)
	}

	// 4. Test that NO immediate idle timeout occurs.
	// Since idleTimeout is 2s, wait for 1 second. If lastReceived wasn't initialized,
	// it would have timed out immediately.
	time.Sleep(1 * time.Second)
	if atomic.LoadInt64(&connectionsCount) != 1 {
		t.Errorf("Expected no idle timeout reconnections yet, but got %d connections", connectionsCount)
	}

	// 5. Trigger manual reconnection
	p.RequestReconnect("test reconnect")

	// Wait for the rate-limiting sleep (5 seconds) to finish and reconnect
	time.Sleep(5500 * time.Millisecond)

	if atomic.LoadInt64(&connectionsCount) < 2 {
		t.Errorf("Expected at least 2 connections after reconnect request, got %d", connectionsCount)
	}

	// 6. Verify that it automatically resubscribed on reconnect
	if atomic.LoadInt64(&subscribeCount) < 2 {
		t.Errorf("Expected resubscription on reconnection, subscribeCount is %d", subscribeCount)
	}

	// 7. Verify rate limiting (at least 5 seconds between connection attempts)
	mu.Lock()
	numConnects := len(connectTimes)
	var duration time.Duration
	if numConnects >= 2 {
		duration = connectTimes[1].Sub(connectTimes[0])
	}
	mu.Unlock()
	if numConnects >= 2 {
		t.Logf("Time between first and second connection: %s", duration)
		if duration < 5*time.Second {
			t.Errorf("Expected rate limiting of >= 5 seconds between connections, but got %s", duration)
		}
	}
}
