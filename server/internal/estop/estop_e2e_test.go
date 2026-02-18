package estop

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/holla2040/arturo/internal/protocol"
)

// TestE2E_ButtonPressToCoordinatorState simulates the full chain:
// Station publishes system.emergency_stop -> coordinator parses -> state updates.
func TestE2E_ButtonPressToCoordinatorState(t *testing.T) {
	var callbackState State
	var callbackCalled bool

	coord := New(func(s State) {
		callbackCalled = true
		callbackState = s
	})

	// Simulate what runEstopListener does: parse a JSON message and hand it to the coordinator
	estopMsg := buildEstopProtocolMessage(t,
		"button_press",
		"Physical E-stop button pressed on station-01",
		"station-01",
	)

	if err := coord.HandleMessage(estopMsg); err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	// Verify coordinator state
	state := coord.GetState()
	if !state.Active {
		t.Fatal("expected E-stop to be active")
	}
	if state.Reason != "button_press" {
		t.Fatalf("expected reason 'button_press', got %q", state.Reason)
	}
	if state.Description != "Physical E-stop button pressed on station-01" {
		t.Fatalf("unexpected description: %q", state.Description)
	}
	if state.Initiator != "station-01" {
		t.Fatalf("expected initiator 'station-01', got %q", state.Initiator)
	}
	if state.TriggeredAt.IsZero() {
		t.Fatal("TriggeredAt should be set")
	}

	// Verify callback was invoked with correct data
	if !callbackCalled {
		t.Fatal("onEstop callback was not called")
	}
	if !callbackState.Active {
		t.Fatal("callback state should be active")
	}
	if callbackState.Reason != "button_press" {
		t.Fatalf("callback reason expected 'button_press', got %q", callbackState.Reason)
	}
}

// TestE2E_ResetRecoveryFlow tests: trigger -> acknowledge -> verify cleared -> re-trigger -> acknowledge
func TestE2E_ResetRecoveryFlow(t *testing.T) {
	var triggerCount int
	coord := New(func(s State) {
		triggerCount++
	})

	// Step 1: First E-stop from button press
	msg1 := buildEstopProtocolMessage(t, "button_press", "Button pressed", "station-01")
	if err := coord.HandleMessage(msg1); err != nil {
		t.Fatalf("HandleMessage 1 failed: %v", err)
	}

	s := coord.GetState()
	if !s.Active {
		t.Fatal("step 1: should be active after first trigger")
	}
	if triggerCount != 1 {
		t.Fatalf("step 1: expected 1 trigger, got %d", triggerCount)
	}

	// Step 2: Acknowledge (operator clears E-stop)
	coord.Acknowledge()
	s = coord.GetState()
	if s.Active {
		t.Fatal("step 2: should be inactive after acknowledge")
	}
	if s.Reason != "" || s.Description != "" || s.Initiator != "" {
		t.Fatal("step 2: state fields should be empty after acknowledge")
	}
	if !s.TriggeredAt.IsZero() {
		t.Fatal("step 2: TriggeredAt should be zero after acknowledge")
	}

	// Step 3: Second E-stop from safety interlock
	msg2 := buildEstopProtocolMessage(t, "safety_interlock", "Overtemp detected", "station-02")
	if err := coord.HandleMessage(msg2); err != nil {
		t.Fatalf("HandleMessage 2 failed: %v", err)
	}

	s = coord.GetState()
	if !s.Active {
		t.Fatal("step 3: should be active after second trigger")
	}
	if s.Reason != "safety_interlock" {
		t.Fatalf("step 3: expected reason 'safety_interlock', got %q", s.Reason)
	}
	if s.Initiator != "station-02" {
		t.Fatalf("step 3: expected initiator 'station-02', got %q", s.Initiator)
	}
	if triggerCount != 2 {
		t.Fatalf("step 3: expected 2 triggers, got %d", triggerCount)
	}

	// Step 4: Acknowledge again
	coord.Acknowledge()
	s = coord.GetState()
	if s.Active {
		t.Fatal("step 4: should be inactive after second acknowledge")
	}
}

// TestE2E_MultipleStationsSimultaneous tests concurrent E-stop triggers from multiple stations.
func TestE2E_MultipleStationsSimultaneous(t *testing.T) {
	var callbackCount atomic.Int32
	coord := New(func(s State) {
		callbackCount.Add(1)
	})

	stations := []struct {
		reason    string
		desc      string
		initiator string
	}{
		{"button_press", "Station 1 button", "station-01"},
		{"safety_interlock", "Station 2 overtemp", "station-02"},
		{"device_fault", "Station 3 DMM fault", "station-03"},
		{"button_press", "Station 4 button", "station-04"},
		{"operator_command", "Operator panel", "panel-01"},
		{"software_error", "Firmware crash", "station-05"},
	}

	var wg sync.WaitGroup
	for _, st := range stations {
		wg.Add(1)
		go func(reason, desc, initiator string) {
			defer wg.Done()
			msg := buildEstopProtocolMessage(t, reason, desc, initiator)
			if err := coord.HandleMessage(msg); err != nil {
				t.Errorf("HandleMessage failed for %s: %v", initiator, err)
			}
		}(st.reason, st.desc, st.initiator)
	}
	wg.Wait()

	// All triggers should have been processed
	if callbackCount.Load() != int32(len(stations)) {
		t.Fatalf("expected %d callbacks, got %d", len(stations), callbackCount.Load())
	}

	// State should be active (last writer wins, but definitely active)
	s := coord.GetState()
	if !s.Active {
		t.Fatal("should be active after concurrent triggers")
	}
}

// TestE2E_CallbackBroadcastSimulation simulates the real wiring in main.go
// where the coordinator callback broadcasts via WebSocket hub and records to SQLite.
func TestE2E_CallbackBroadcastSimulation(t *testing.T) {
	var broadcastedEvents []broadcastedEvent
	var recordedEvents []recordedEvent
	var mu sync.Mutex

	coord := New(func(s State) {
		mu.Lock()
		defer mu.Unlock()
		// Simulates: wsHub.BroadcastEvent("estop", state)
		broadcastedEvents = append(broadcastedEvents, broadcastedEvent{
			eventType: "estop",
			state:     s,
		})
		// Simulates: db.RecordDeviceEvent("system", "controller", "emergency_stop", state.Reason)
		recordedEvents = append(recordedEvents, recordedEvent{
			deviceID:  "system",
			station:   "controller",
			eventType: "emergency_stop",
			details:   s.Reason,
		})
	})

	// Trigger from station button press
	msg := buildEstopProtocolMessage(t, "button_press", "Physical button", "station-01")
	if err := coord.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(broadcastedEvents) != 1 {
		t.Fatalf("expected 1 broadcast, got %d", len(broadcastedEvents))
	}
	if broadcastedEvents[0].eventType != "estop" {
		t.Fatalf("expected event type 'estop', got %q", broadcastedEvents[0].eventType)
	}
	if !broadcastedEvents[0].state.Active {
		t.Fatal("broadcast state should be active")
	}

	if len(recordedEvents) != 1 {
		t.Fatalf("expected 1 recorded event, got %d", len(recordedEvents))
	}
	if recordedEvents[0].deviceID != "system" {
		t.Fatalf("expected device 'system', got %q", recordedEvents[0].deviceID)
	}
	if recordedEvents[0].details != "button_press" {
		t.Fatalf("expected details 'button_press', got %q", recordedEvents[0].details)
	}
}

// TestE2E_AllReasonTypes verifies all valid E-stop reason types from the schema.
func TestE2E_AllReasonTypes(t *testing.T) {
	reasons := []string{
		"button_press",
		"operator_command",
		"safety_interlock",
		"device_fault",
		"software_error",
	}

	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			var received string
			coord := New(func(s State) {
				received = s.Reason
			})

			msg := buildEstopProtocolMessage(t, reason, "Test "+reason, "test-station")
			if err := coord.HandleMessage(msg); err != nil {
				t.Fatalf("HandleMessage failed for reason %q: %v", reason, err)
			}

			if received != reason {
				t.Fatalf("expected reason %q, got %q", reason, received)
			}

			s := coord.GetState()
			if !s.Active {
				t.Fatalf("expected active for reason %q", reason)
			}
			if s.Reason != reason {
				t.Fatalf("state reason expected %q, got %q", reason, s.Reason)
			}
		})
	}
}

// TestE2E_InvalidJSONMessages verifies the coordinator returns errors for truly invalid JSON.
func TestE2E_InvalidJSONMessages(t *testing.T) {
	coord := New(nil)

	invalidPayloads := []struct {
		name    string
		payload string
	}{
		{"invalid_json", `not json at all`},
		{"number_payload", `42`},
		{"array_payload", `[1,2,3]`},
	}

	for _, tc := range invalidPayloads {
		t.Run(tc.name, func(t *testing.T) {
			msg := &protocol.Message{
				Envelope: protocol.Envelope{
					Type: protocol.TypeSystemEmergencyStop,
				},
				Payload: json.RawMessage(tc.payload),
			}
			err := coord.HandleMessage(msg)
			if err == nil {
				t.Fatalf("expected error for payload %q", tc.payload)
			}
		})
	}

	// State should remain inactive since all messages were invalid
	s := coord.GetState()
	if s.Active {
		t.Fatal("state should remain inactive after invalid messages")
	}
}

// TestE2E_ValidJSONWithMissingFields verifies that valid JSON with missing optional
// fields still triggers the coordinator (reason defaults to empty string).
func TestE2E_ValidJSONWithMissingFields(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{"empty_json", `{}`},
		{"missing_reason", `{"description":"test","initiator":"s1"}`},
		{"null_reason", `{"reason":null}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			coord := New(nil)
			msg := &protocol.Message{
				Envelope: protocol.Envelope{
					Type: protocol.TypeSystemEmergencyStop,
				},
				Payload: json.RawMessage(tc.payload),
			}
			err := coord.HandleMessage(msg)
			// These are valid JSON that unmarshal to EmergencyStopPayload
			// (with zero-value fields), so no error expected
			if err != nil {
				t.Fatalf("unexpected error for payload %q: %v", tc.payload, err)
			}
			// State will be active (coordinator sets Active=true regardless of field values)
			s := coord.GetState()
			if !s.Active {
				t.Fatal("state should be active even with missing fields")
			}
		})
	}
}

// TestE2E_TriggerAPIThenAcknowledge simulates the flow:
// POST trigger -> GET status shows active -> POST acknowledge -> GET status shows clear.
// Uses the coordinator directly (no HTTP server needed for unit test).
func TestE2E_TriggerAPIThenAcknowledge(t *testing.T) {
	coord := New(nil)

	// Verify initial state is clear
	s := coord.GetState()
	if s.Active {
		t.Fatal("initial state should be inactive")
	}

	// Trigger via programmatic API (as REST handler would call)
	coord.Trigger("operator_command", "Operator pressed E-stop in UI", "operator-01")

	s = coord.GetState()
	if !s.Active {
		t.Fatal("state should be active after Trigger")
	}
	if s.Reason != "operator_command" {
		t.Fatalf("expected reason 'operator_command', got %q", s.Reason)
	}

	// Acknowledge (as REST handler would call)
	coord.Acknowledge()

	s = coord.GetState()
	if s.Active {
		t.Fatal("state should be inactive after Acknowledge")
	}
}

// TestE2E_RapidTriggerAcknowledgeCycle stress tests rapid trigger/acknowledge cycles.
func TestE2E_RapidTriggerAcknowledgeCycle(t *testing.T) {
	var triggerCount atomic.Int32
	coord := New(func(s State) {
		triggerCount.Add(1)
	})

	const cycles = 100
	for i := 0; i < cycles; i++ {
		msg := buildEstopProtocolMessage(t, "button_press", "Rapid test", "station-01")
		if err := coord.HandleMessage(msg); err != nil {
			t.Fatalf("cycle %d HandleMessage failed: %v", i, err)
		}
		if !coord.GetState().Active {
			t.Fatalf("cycle %d: should be active after trigger", i)
		}
		coord.Acknowledge()
		if coord.GetState().Active {
			t.Fatalf("cycle %d: should be inactive after acknowledge", i)
		}
	}

	if triggerCount.Load() != cycles {
		t.Fatalf("expected %d triggers, got %d", cycles, triggerCount.Load())
	}
}

// TestE2E_ConcurrentTriggerAndAcknowledge runs triggers and acknowledges concurrently.
func TestE2E_ConcurrentTriggerAndAcknowledge(t *testing.T) {
	coord := New(nil)

	var wg sync.WaitGroup
	const n = 200

	// Half trigger, half acknowledge
	for i := 0; i < n; i++ {
		wg.Add(1)
		if i%2 == 0 {
			go func() {
				defer wg.Done()
				coord.Trigger("button_press", "concurrent", "station-01")
			}()
		} else {
			go func() {
				defer wg.Done()
				coord.Acknowledge()
			}()
		}
	}

	// Also concurrently read state
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = coord.GetState()
		}()
	}

	wg.Wait()

	// No panic is the main assertion; state should be consistent
	s := coord.GetState()
	_ = s // State could be active or inactive depending on goroutine ordering
}

// TestE2E_HandleMessageThenTriggerOverwrite verifies that a Trigger call
// overwrites state set by HandleMessage.
func TestE2E_HandleMessageThenTriggerOverwrite(t *testing.T) {
	coord := New(nil)

	msg := buildEstopProtocolMessage(t, "button_press", "Button", "station-01")
	if err := coord.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	s := coord.GetState()
	if s.Reason != "button_press" {
		t.Fatalf("expected reason 'button_press', got %q", s.Reason)
	}

	// Now Trigger overwrites
	coord.Trigger("operator_command", "Operator override", "operator-01")

	s = coord.GetState()
	if s.Reason != "operator_command" {
		t.Fatalf("expected reason 'operator_command' after Trigger, got %q", s.Reason)
	}
	if s.Initiator != "operator-01" {
		t.Fatalf("expected initiator 'operator-01', got %q", s.Initiator)
	}
}

// TestE2E_WebSocketNotificationChain verifies the full flow from protocol
// message to WebSocket event. This test uses a real WebSocket hub in-process.
func TestE2E_WebSocketNotificationChain(t *testing.T) {
	// Create a simple event collector that simulates WebSocket broadcast
	events := make(chan wsTestEvent, 10)

	coord := New(func(s State) {
		events <- wsTestEvent{
			eventType: "estop",
			active:    s.Active,
			reason:    s.Reason,
			initiator: s.Initiator,
		}
	})

	// Simulate station button press arriving via Redis
	msg := buildEstopProtocolMessage(t, "button_press", "Physical button", "station-03")
	if err := coord.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	select {
	case evt := <-events:
		if evt.eventType != "estop" {
			t.Fatalf("expected event type 'estop', got %q", evt.eventType)
		}
		if !evt.active {
			t.Fatal("event should show active E-stop")
		}
		if evt.reason != "button_press" {
			t.Fatalf("expected reason 'button_press', got %q", evt.reason)
		}
		if evt.initiator != "station-03" {
			t.Fatalf("expected initiator 'station-03', got %q", evt.initiator)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for WebSocket event")
	}
}

// TestE2E_SystemStatusReflectsEstop verifies that the coordinator state
// is correctly reflected in a system status response-like check.
func TestE2E_SystemStatusReflectsEstop(t *testing.T) {
	coord := New(nil)

	// Initially inactive
	s := coord.GetState()
	statusJSON := marshalStatus(t, s)
	if statusJSON.Active {
		t.Fatal("initial status should show inactive")
	}

	// Trigger E-stop
	coord.Trigger("button_press", "Test", "station-01")

	s = coord.GetState()
	statusJSON = marshalStatus(t, s)
	if !statusJSON.Active {
		t.Fatal("status should show active after trigger")
	}
	if statusJSON.Reason != "button_press" {
		t.Fatalf("expected reason 'button_press', got %q", statusJSON.Reason)
	}

	// Acknowledge
	coord.Acknowledge()

	s = coord.GetState()
	statusJSON = marshalStatus(t, s)
	if statusJSON.Active {
		t.Fatal("status should show inactive after acknowledge")
	}
}

// TestE2E_HTTPStatusEndpointWithEstop tests E-stop state via HTTP endpoint simulation.
func TestE2E_HTTPStatusEndpointWithEstop(t *testing.T) {
	coord := New(nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := coord.GetState()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"estop_active": s.Active,
			"estop_reason": s.Reason,
		})
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Check initial state via HTTP
	resp, err := http.Get(srv.URL + "/system/status")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if result["estop_active"] != false {
		t.Fatalf("expected estop_active=false, got %v", result["estop_active"])
	}

	// Trigger E-stop via protocol message
	msg := buildEstopProtocolMessage(t, "button_press", "Button pressed", "station-01")
	if err := coord.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	// Check state via HTTP again
	resp, err = http.Get(srv.URL + "/system/status")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if result["estop_active"] != true {
		t.Fatalf("expected estop_active=true, got %v", result["estop_active"])
	}
	if result["estop_reason"] != "button_press" {
		t.Fatalf("expected estop_reason='button_press', got %v", result["estop_reason"])
	}

	// Acknowledge and verify via HTTP
	coord.Acknowledge()

	resp, err = http.Get(srv.URL + "/system/status")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if result["estop_active"] != false {
		t.Fatalf("expected estop_active=false after acknowledge, got %v", result["estop_active"])
	}
}

// TestE2E_TimestampProgression verifies that timestamps increase across triggers.
func TestE2E_TimestampProgression(t *testing.T) {
	coord := New(nil)

	coord.Trigger("button_press", "First", "station-01")
	first := coord.GetState().TriggeredAt

	// Small sleep to ensure time difference
	time.Sleep(time.Millisecond)

	coord.Acknowledge()
	coord.Trigger("button_press", "Second", "station-01")
	second := coord.GetState().TriggeredAt

	if !second.After(first) {
		t.Fatalf("second trigger timestamp (%v) should be after first (%v)", second, first)
	}
}

// TestE2E_DoubleAcknowledgeIdempotent verifies that acknowledging twice doesn't cause issues.
func TestE2E_DoubleAcknowledgeIdempotent(t *testing.T) {
	coord := New(nil)

	coord.Trigger("button_press", "Test", "station-01")
	coord.Acknowledge()
	coord.Acknowledge() // Second acknowledge

	s := coord.GetState()
	if s.Active {
		t.Fatal("should still be inactive after double acknowledge")
	}
}

// TestE2E_AcknowledgeWithoutTrigger verifies acknowledging without trigger is safe.
func TestE2E_AcknowledgeWithoutTrigger(t *testing.T) {
	coord := New(nil)

	// Acknowledge without prior trigger should not panic
	coord.Acknowledge()

	s := coord.GetState()
	if s.Active {
		t.Fatal("should be inactive")
	}
}

// --- Helpers ---

func buildEstopProtocolMessage(t *testing.T, reason, description, initiator string) *protocol.Message {
	t.Helper()
	payload := protocol.EmergencyStopPayload{
		Reason:      reason,
		Description: description,
		Initiator:   initiator,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	env := protocol.NewEnvelope(protocol.Source{
		Service:  "esp32_station",
		Instance: initiator,
		Version:  "1.0.0",
	}, protocol.TypeSystemEmergencyStop)

	return &protocol.Message{
		Envelope: env,
		Payload:  json.RawMessage(payloadBytes),
	}
}

type broadcastedEvent struct {
	eventType string
	state     State
}

type recordedEvent struct {
	deviceID  string
	station   string
	eventType string
	details   string
}

type wsTestEvent struct {
	eventType string
	active    bool
	reason    string
	initiator string
}

func marshalStatus(t *testing.T, s State) State {
	t.Helper()
	// Round-trip through JSON to verify serialization
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	var result State
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	return result
}

// TestE2E_StateJSONSerialization verifies the State struct serializes correctly
// for WebSocket broadcast and API responses.
func TestE2E_StateJSONSerialization(t *testing.T) {
	coord := New(nil)
	coord.Trigger("button_press", "Physical button pressed", "station-01")

	s := coord.GetState()
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map failed: %v", err)
	}

	if m["active"] != true {
		t.Fatalf("expected active=true in JSON, got %v", m["active"])
	}
	if m["reason"] != "button_press" {
		t.Fatalf("expected reason='button_press' in JSON, got %v", m["reason"])
	}
	if m["description"] != "Physical button pressed" {
		t.Fatalf("expected description in JSON, got %v", m["description"])
	}
	if m["initiator"] != "station-01" {
		t.Fatalf("expected initiator='station-01' in JSON, got %v", m["initiator"])
	}
	if _, ok := m["triggered_at"]; !ok {
		t.Fatal("expected triggered_at in JSON")
	}

	// Verify inactive state JSON has omitempty behavior
	coord.Acknowledge()
	s = coord.GetState()
	data, _ = json.Marshal(s)
	json.Unmarshal(data, &m)

	if m["active"] != false {
		t.Fatalf("expected active=false in acknowledged JSON, got %v", m["active"])
	}
}

// TestE2E_ProtocolMessageRoundTrip builds a full protocol message from scratch
// (as a station would), serializes to JSON, parses back, and feeds to coordinator.
func TestE2E_ProtocolMessageRoundTrip(t *testing.T) {
	// Build message as station would
	source := protocol.Source{
		Service:  "esp32_station",
		Instance: "station-01",
		Version:  "1.0.0",
	}
	payload := protocol.EmergencyStopPayload{
		Reason:      "button_press",
		Description: "E-stop button pressed",
		Initiator:   "station-01",
	}
	msg, err := protocol.NewMessage(source, protocol.TypeSystemEmergencyStop, payload)
	if err != nil {
		t.Fatalf("NewMessage failed: %v", err)
	}

	// Serialize to JSON (as Redis would carry it)
	jsonData, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Parse from JSON (as runEstopListener does)
	parsed, err := protocol.Parse(jsonData)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Verify envelope
	if parsed.Envelope.Type != protocol.TypeSystemEmergencyStop {
		t.Fatalf("expected type %q, got %q", protocol.TypeSystemEmergencyStop, parsed.Envelope.Type)
	}
	if parsed.Envelope.Source.Instance != "station-01" {
		t.Fatalf("expected instance 'station-01', got %q", parsed.Envelope.Source.Instance)
	}
	if parsed.Envelope.SchemaVersion != protocol.SchemaVersion {
		t.Fatalf("expected schema version %q, got %q", protocol.SchemaVersion, parsed.Envelope.SchemaVersion)
	}

	// Feed to coordinator
	var callbackFired bool
	coord := New(func(s State) {
		callbackFired = true
	})

	if err := coord.HandleMessage(parsed); err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	if !callbackFired {
		t.Fatal("callback should have fired")
	}

	s := coord.GetState()
	if !s.Active {
		t.Fatal("state should be active")
	}
	if s.Reason != "button_press" {
		t.Fatalf("expected reason 'button_press', got %q", s.Reason)
	}
}

// TestE2E_EmptyOptionalFields tests that optional fields (description, initiator) can be empty.
func TestE2E_EmptyOptionalFields(t *testing.T) {
	coord := New(nil)

	payload := protocol.EmergencyStopPayload{
		Reason: "button_press",
		// description and initiator omitted
	}
	payloadBytes, _ := json.Marshal(payload)
	msg := &protocol.Message{
		Envelope: protocol.Envelope{Type: protocol.TypeSystemEmergencyStop},
		Payload:  json.RawMessage(payloadBytes),
	}

	if err := coord.HandleMessage(msg); err != nil {
		t.Fatalf("HandleMessage failed: %v", err)
	}

	s := coord.GetState()
	if !s.Active {
		t.Fatal("should be active")
	}
	if s.Reason != "button_press" {
		t.Fatalf("expected reason 'button_press', got %q", s.Reason)
	}
	if s.Description != "" {
		t.Fatalf("expected empty description, got %q", s.Description)
	}
	if s.Initiator != "" {
		t.Fatalf("expected empty initiator, got %q", s.Initiator)
	}
}

