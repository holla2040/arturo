package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"nhooyr.io/websocket"
)

func TestHubStartsAndStops(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		hub.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("hub did not stop")
	}
}

func TestHubClientCount(t *testing.T) {
	hub := NewHub()
	if hub.ClientCount() != 0 {
		t.Fatalf("expected 0 clients, got %d", hub.ClientCount())
	}
}

func TestHubBroadcastToClient(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	srv := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	defer srv.Close()

	// Connect WebSocket client
	wsURL := "ws" + srv.URL[4:] // http -> ws
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Wait for registration
	time.Sleep(50 * time.Millisecond)

	if hub.ClientCount() != 1 {
		t.Fatalf("expected 1 client, got %d", hub.ClientCount())
	}

	// Broadcast a message
	hub.BroadcastEvent("heartbeat", map[string]string{"instance": "station-01"})

	// Read the broadcast message
	readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
	defer readCancel()

	_, data, err := conn.Read(readCtx)
	if err != nil {
		t.Fatalf("websocket read failed: %v", err)
	}

	var evt WSEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}
	if evt.Type != "heartbeat" {
		t.Errorf("expected type heartbeat, got %s", evt.Type)
	}
}

func TestHubClientDisconnect(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	srv := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:]
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}

	// Wait for registration
	time.Sleep(50 * time.Millisecond)
	if hub.ClientCount() != 1 {
		t.Fatalf("expected 1 client, got %d", hub.ClientCount())
	}

	// Close the connection
	conn.Close(websocket.StatusNormalClosure, "done")

	// Wait for unregister
	time.Sleep(100 * time.Millisecond)
	if hub.ClientCount() != 0 {
		t.Fatalf("expected 0 clients after disconnect, got %d", hub.ClientCount())
	}
}

func TestHubMultipleClients(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	srv := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:]
	var conns []*websocket.Conn

	for i := 0; i < 3; i++ {
		conn, _, err := websocket.Dial(ctx, wsURL, nil)
		if err != nil {
			t.Fatalf("websocket dial %d failed: %v", i, err)
		}
		conns = append(conns, conn)
	}
	defer func() {
		for _, c := range conns {
			c.Close(websocket.StatusNormalClosure, "")
		}
	}()

	// Wait for all registrations
	time.Sleep(100 * time.Millisecond)
	if hub.ClientCount() != 3 {
		t.Fatalf("expected 3 clients, got %d", hub.ClientCount())
	}

	// Broadcast should reach all clients
	hub.BroadcastEvent("test", "hello")

	for i, conn := range conns {
		readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
		_, data, err := conn.Read(readCtx)
		readCancel()
		if err != nil {
			t.Fatalf("client %d read failed: %v", i, err)
		}

		var evt WSEvent
		json.Unmarshal(data, &evt)
		if evt.Type != "test" {
			t.Errorf("client %d: expected type test, got %s", i, evt.Type)
		}
	}
}

func TestBroadcastEventMarshal(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	srv := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:]
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	time.Sleep(50 * time.Millisecond)

	payload := map[string]interface{}{
		"active": true,
		"reason": "manual",
	}
	hub.BroadcastEvent("estop", payload)

	readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
	defer readCancel()
	_, data, err := conn.Read(readCtx)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	var evt WSEvent
	json.Unmarshal(data, &evt)
	if evt.Type != "estop" {
		t.Errorf("expected type estop, got %s", evt.Type)
	}

	p, ok := evt.Payload.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map payload, got %T", evt.Payload)
	}
	if p["active"] != true {
		t.Errorf("expected active=true, got %v", p["active"])
	}
}
