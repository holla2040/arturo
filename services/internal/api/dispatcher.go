package api

import (
	"sync"

	"github.com/holla2040/arturo/internal/protocol"
)

// ResponseDispatcher routes command responses to waiting API callers
// by matching correlation IDs.
type ResponseDispatcher struct {
	mu      sync.Mutex
	waiters map[string]chan *protocol.Message
}

// NewResponseDispatcher creates a new dispatcher.
func NewResponseDispatcher() *ResponseDispatcher {
	return &ResponseDispatcher{
		waiters: make(map[string]chan *protocol.Message),
	}
}

// Register creates a buffered channel for the given correlation ID
// and returns it. The caller should select on this channel with a timeout.
func (d *ResponseDispatcher) Register(correlationID string) chan *protocol.Message {
	ch := make(chan *protocol.Message, 1)
	d.mu.Lock()
	d.waiters[correlationID] = ch
	d.mu.Unlock()
	return ch
}

// Dispatch sends a response message to the waiter registered for its
// correlation ID. Returns true if a waiter was found.
func (d *ResponseDispatcher) Dispatch(msg *protocol.Message) bool {
	d.mu.Lock()
	ch, ok := d.waiters[msg.Envelope.CorrelationID]
	if ok {
		delete(d.waiters, msg.Envelope.CorrelationID)
	}
	d.mu.Unlock()

	if ok {
		ch <- msg
		return true
	}
	return false
}

// Deregister removes a waiter without sending a response.
// Used for cleanup after timeout.
func (d *ResponseDispatcher) Deregister(correlationID string) {
	d.mu.Lock()
	ch, ok := d.waiters[correlationID]
	if ok {
		delete(d.waiters, correlationID)
		close(ch)
	}
	d.mu.Unlock()
}

// PendingCount returns the number of active waiters (for diagnostics).
func (d *ResponseDispatcher) PendingCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.waiters)
}
