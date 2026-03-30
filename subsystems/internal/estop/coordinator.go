package estop

import (
	"sync"
	"time"

	"github.com/holla2040/arturo/internal/protocol"
)

// State represents the current emergency stop state.
type State struct {
	Active      bool      `json:"active"`
	Reason      string    `json:"reason,omitempty"`
	Description string    `json:"description,omitempty"`
	Initiator   string    `json:"initiator,omitempty"`
	TriggeredAt time.Time `json:"triggered_at,omitempty"`
}

// Coordinator manages emergency stop state and notifies via callback.
// It contains no Redis logic — Redis subscription is wired externally.
type Coordinator struct {
	mu      sync.RWMutex
	state   State
	onEstop func(State)
}

// New creates a Coordinator with an inactive initial state.
// The onEstop callback is called on every trigger; it may be nil.
func New(onEstop func(State)) *Coordinator {
	return &Coordinator{
		onEstop: onEstop,
	}
}

// HandleMessage parses an EmergencyStopPayload from a protocol message
// and activates the e-stop. Returns an error if the payload cannot be parsed.
func (c *Coordinator) HandleMessage(msg *protocol.Message) error {
	p, err := protocol.ParseEmergencyStop(msg)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.state = State{
		Active:      true,
		Reason:      p.Reason,
		Description: p.Description,
		Initiator:   p.Initiator,
		TriggeredAt: time.Now(),
	}
	s := c.state
	cb := c.onEstop
	c.mu.Unlock()

	if cb != nil {
		cb(s)
	}
	return nil
}

// Trigger activates the e-stop with the given reason, description, and initiator.
// Returns the new state.
func (c *Coordinator) Trigger(reason, description, initiator string) State {
	c.mu.Lock()
	c.state = State{
		Active:      true,
		Reason:      reason,
		Description: description,
		Initiator:   initiator,
		TriggeredAt: time.Now(),
	}
	s := c.state
	cb := c.onEstop
	c.mu.Unlock()

	if cb != nil {
		cb(s)
	}
	return s
}

// Acknowledge clears the e-stop, returning to an inactive state.
func (c *Coordinator) Acknowledge() {
	c.mu.Lock()
	c.state = State{}
	c.mu.Unlock()
}

// GetState returns a copy of the current state.
func (c *Coordinator) GetState() State {
	c.mu.RLock()
	s := c.state
	c.mu.RUnlock()
	return s
}
