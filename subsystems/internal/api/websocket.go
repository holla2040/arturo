package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

// WSEvent is the JSON envelope broadcast to WebSocket clients.
type WSEvent struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// Hub manages WebSocket client connections and broadcasts events.
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]bool

	registerCh   chan *Client
	unregisterCh chan *Client
	broadcastCh  chan []byte
}

// Client wraps a single WebSocket connection.
type Client struct {
	conn *websocket.Conn
	send chan []byte
}

// NewHub creates a new WebSocket hub.
func NewHub() *Hub {
	return &Hub{
		clients:      make(map[*Client]bool),
		registerCh:   make(chan *Client, 16),
		unregisterCh: make(chan *Client, 16),
		broadcastCh:  make(chan []byte, 256),
	}
}

// Run processes register, unregister, and broadcast events.
// Blocks until ctx is cancelled.
func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			h.mu.Lock()
			for c := range h.clients {
				close(c.send)
				delete(h.clients, c)
			}
			h.mu.Unlock()
			return

		case client := <-h.registerCh:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregisterCh:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				close(client.send)
				delete(h.clients, client)
			}
			h.mu.Unlock()

		case data := <-h.broadcastCh:
			h.mu.RLock()
			for c := range h.clients {
				select {
				case c.send <- data:
				default:
					// Client buffer full, skip
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends data to all connected clients.
// Safe to call from any goroutine.
func (h *Hub) Broadcast(data []byte) {
	select {
	case h.broadcastCh <- data:
	default:
		// Broadcast channel full, drop message
	}
}

// BroadcastEvent marshals a WSEvent and broadcasts it.
func (h *Hub) BroadcastEvent(eventType string, payload interface{}) {
	evt := WSEvent{Type: eventType, Payload: payload}
	data, err := json.Marshal(evt)
	if err != nil {
		log.Printf("websocket: failed to marshal event: %v", err)
		return
	}
	h.Broadcast(data)
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// HandleWebSocket is an HTTP handler that upgrades to WebSocket.
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Allow all origins for LAN use
	})
	if err != nil {
		log.Printf("websocket: accept failed: %v", err)
		return
	}

	client := &Client{
		conn: conn,
		send: make(chan []byte, 64),
	}

	h.registerCh <- client

	go h.writePump(r.Context(), client)
	h.readPump(r.Context(), client)
}

// writePump sends messages from the client's send channel to the WebSocket.
func (h *Hub) writePump(ctx context.Context, c *Client) {
	defer func() {
		c.conn.Close(websocket.StatusNormalClosure, "")
	}()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := c.conn.Write(writeCtx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// readPump reads messages from the WebSocket (drains, we don't expect client messages).
func (h *Hub) readPump(ctx context.Context, c *Client) {
	defer func() {
		h.unregisterCh <- c
	}()

	for {
		_, _, err := c.conn.Read(ctx)
		if err != nil {
			return
		}
		// We don't process incoming messages from clients
	}
}
