package estop

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/holla2040/arturo/internal/protocol"
)

func TestNewCoordinatorInactive(t *testing.T) {
	c := New(nil)
	s := c.GetState()
	if s.Active {
		t.Fatal("new coordinator should be inactive")
	}
	if s.Reason != "" || s.Description != "" || s.Initiator != "" {
		t.Fatal("new coordinator state fields should be empty")
	}
	if !s.TriggeredAt.IsZero() {
		t.Fatal("new coordinator TriggeredAt should be zero")
	}
}

func TestTriggerActivates(t *testing.T) {
	c := New(nil)
	s := c.Trigger("hardware_fault", "Pump overtemp", "station-01")
	if !s.Active {
		t.Fatal("state should be active after Trigger")
	}
	if s.Reason != "hardware_fault" {
		t.Fatalf("expected reason %q, got %q", "hardware_fault", s.Reason)
	}
	if s.Description != "Pump overtemp" {
		t.Fatalf("expected description %q, got %q", "Pump overtemp", s.Description)
	}
	if s.Initiator != "station-01" {
		t.Fatalf("expected initiator %q, got %q", "station-01", s.Initiator)
	}
	if s.TriggeredAt.IsZero() {
		t.Fatal("TriggeredAt should be set")
	}
}

func TestTriggerCallsCallback(t *testing.T) {
	var called bool
	var received State
	c := New(func(s State) {
		called = true
		received = s
	})
	c.Trigger("test", "desc", "init")
	if !called {
		t.Fatal("onEstop callback should have been called")
	}
	if !received.Active || received.Reason != "test" {
		t.Fatal("callback received incorrect state")
	}
}

func TestGetStateReturnsCurrent(t *testing.T) {
	c := New(nil)
	c.Trigger("r", "d", "i")
	s := c.GetState()
	if !s.Active {
		t.Fatal("GetState should reflect triggered state")
	}
	if s.Reason != "r" {
		t.Fatalf("expected reason %q, got %q", "r", s.Reason)
	}
}

func TestAcknowledgeClearsState(t *testing.T) {
	c := New(nil)
	c.Trigger("fault", "desc", "init")
	c.Acknowledge()
	s := c.GetState()
	if s.Active {
		t.Fatal("state should be inactive after Acknowledge")
	}
	if s.Reason != "" || s.Description != "" || s.Initiator != "" {
		t.Fatal("state fields should be empty after Acknowledge")
	}
	if !s.TriggeredAt.IsZero() {
		t.Fatal("TriggeredAt should be zero after Acknowledge")
	}
}

func TestAcknowledgeAfterTrigger(t *testing.T) {
	c := New(nil)
	c.Trigger("fault", "desc", "init")
	if !c.GetState().Active {
		t.Fatal("should be active after Trigger")
	}
	c.Acknowledge()
	s := c.GetState()
	if s.Active {
		t.Fatal("should be inactive after Acknowledge")
	}
}

func makeEstopMessage(t *testing.T, reason, desc, initiator string) *protocol.Message {
	t.Helper()
	payload := protocol.EmergencyStopPayload{
		Reason:      reason,
		Description: desc,
		Initiator:   initiator,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return &protocol.Message{
		Envelope: protocol.Envelope{Type: protocol.TypeSystemEmergencyStop},
		Payload:  json.RawMessage(payloadBytes),
	}
}

func TestHandleMessageActivates(t *testing.T) {
	c := New(nil)
	msg := makeEstopMessage(t, "operator", "Button pressed", "panel-01")
	if err := c.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage error: %v", err)
	}
	s := c.GetState()
	if !s.Active {
		t.Fatal("state should be active after HandleMessage")
	}
	if s.Reason != "operator" {
		t.Fatalf("expected reason %q, got %q", "operator", s.Reason)
	}
	if s.Description != "Button pressed" {
		t.Fatalf("expected description %q, got %q", "Button pressed", s.Description)
	}
	if s.Initiator != "panel-01" {
		t.Fatalf("expected initiator %q, got %q", "panel-01", s.Initiator)
	}
}

func TestHandleMessageCallsCallback(t *testing.T) {
	var called bool
	c := New(func(s State) {
		called = true
	})
	msg := makeEstopMessage(t, "test", "desc", "init")
	if err := c.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage error: %v", err)
	}
	if !called {
		t.Fatal("onEstop callback should have been called")
	}
}

func TestHandleMessageInvalidPayload(t *testing.T) {
	c := New(nil)
	msg := &protocol.Message{
		Envelope: protocol.Envelope{Type: protocol.TypeSystemEmergencyStop},
		Payload:  json.RawMessage(`not valid json`),
	}
	if err := c.HandleMessage(msg); err == nil {
		t.Fatal("expected error for invalid payload")
	}
}

func TestTriggerAfterAcknowledgeReactivates(t *testing.T) {
	c := New(nil)
	c.Trigger("first", "d1", "i1")
	c.Acknowledge()
	s := c.Trigger("second", "d2", "i2")
	if !s.Active {
		t.Fatal("should be active after re-trigger")
	}
	if s.Reason != "second" {
		t.Fatalf("expected reason %q, got %q", "second", s.Reason)
	}
}

func TestMultipleTriggersUpdateState(t *testing.T) {
	c := New(nil)
	c.Trigger("first", "d1", "i1")
	s := c.Trigger("second", "d2", "i2")
	if s.Reason != "second" {
		t.Fatalf("expected reason %q, got %q", "second", s.Reason)
	}
	if s.Description != "d2" {
		t.Fatalf("expected description %q, got %q", "d2", s.Description)
	}
}

func TestNilCallbackDoesNotPanic(t *testing.T) {
	c := New(nil)
	// Should not panic.
	c.Trigger("test", "desc", "init")
	msg := makeEstopMessage(t, "test", "desc", "init")
	if err := c.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage error: %v", err)
	}
}

func TestConcurrentTriggerAndGetState(t *testing.T) {
	c := New(nil)
	var wg sync.WaitGroup
	const n = 100

	wg.Add(2 * n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			c.Trigger("concurrent", "stress", "goroutine")
		}()
		go func() {
			defer wg.Done()
			_ = c.GetState()
		}()
	}
	wg.Wait()

	s := c.GetState()
	if !s.Active {
		t.Fatal("state should be active after concurrent triggers")
	}
}
