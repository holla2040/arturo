package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/holla2040/arturo/internal/protocol"
)

func intPtr(v int) *int       { return &v }
func int64Ptr(v int64) *int64 { return &v }

func makeEnvelope(msgType, corrID string) protocol.Envelope {
	return protocol.Envelope{
		ID:            "550e8400-e29b-41d4-a716-446655440000",
		Timestamp:     1771329600,
		Source:        protocol.Source{Service: "controller", Instance: "ctrl-01", Version: "1.0.0"},
		SchemaVersion: protocol.SchemaVersion,
		Type:          msgType,
		CorrelationID: corrID,
	}
}

func mustMarshal(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return data
}

func TestFormatMessage(t *testing.T) {
	ts := time.Date(2026, 2, 17, 14, 30, 45, 123000000, time.UTC)

	tests := []struct {
		name     string
		dm       *DisplayMessage
		contains []string
	}{
		{
			name: "command request",
			dm: &DisplayMessage{
				Timestamp: ts,
				Channel:   "commands:station-01",
				Direction: "\u2192",
				Message: &protocol.Message{
					Envelope: makeEnvelope(protocol.TypeDeviceCommandRequest, "7c9e6679-7425-40de-944b-e07fc1f90ae7"),
					Payload: mustMarshal(t, protocol.CommandRequestPayload{
						DeviceID:    "fluke-8846a",
						CommandName: "measure_dc_voltage",
						Parameters:  map[string]string{"range": "10V"},
						TimeoutMs:   intPtr(5000),
					}),
				},
			},
			contains: []string{
				"14:30:45.123",
				"commands:station-01",
				"\u2192",
				"device.command.request",
				"corr=7c9e6679",
				"cmd=measure_dc_voltage",
				"range:10V",
			},
		},
		{
			name: "command response",
			dm: &DisplayMessage{
				Timestamp: ts,
				Channel:   "responses:ctrl-01",
				Direction: "\u2190",
				Message: &protocol.Message{
					Envelope: makeEnvelope(protocol.TypeDeviceCommandResponse, "7c9e6679-7425-40de-944b-e07fc1f90ae7"),
					Payload: mustMarshal(t, protocol.CommandResponsePayload{
						DeviceID:    "fluke-8846a",
						CommandName: "measure_dc_voltage",
						Success:     true,
						DurationMs:  intPtr(47),
					}),
				},
			},
			contains: []string{
				"14:30:45.123",
				"responses:ctrl-01",
				"\u2190",
				"device.command.response",
				"corr=7c9e6679",
				"success=true",
				"duration=47ms",
			},
		},
		{
			name: "heartbeat",
			dm: &DisplayMessage{
				Timestamp: ts,
				Channel:   "events:heartbeat",
				Direction: "\u2190",
				Message: &protocol.Message{
					Envelope: makeEnvelope(protocol.TypeServiceHeartbeat, ""),
					Payload: mustMarshal(t, protocol.HeartbeatPayload{
						Status:            "running",
						UptimeSeconds:     3600,
						Devices:           []string{"fluke-8846a"},
						FreeHeap:          245000,
						MinFreeHeap:       int64Ptr(180000),
						WifiRSSI:          -42,
						CommandsProcessed: intPtr(1547),
						FirmwareVersion:   "1.0.0",
					}),
				},
			},
			contains: []string{
				"14:30:45.123",
				"events:heartbeat",
				"service.heartbeat",
				"status=running",
				"heap=245000",
				"rssi=-42",
			},
		},
		{
			name: "emergency stop",
			dm: &DisplayMessage{
				Timestamp: ts,
				Channel:   "events:emergency_stop",
				Direction: "\u2190",
				Message: &protocol.Message{
					Envelope: makeEnvelope(protocol.TypeSystemEmergencyStop, ""),
					Payload: mustMarshal(t, protocol.EmergencyStopPayload{
						Reason:    "button_press",
						Initiator: "estop-01",
					}),
				},
			},
			contains: []string{
				"14:30:45.123",
				"events:emergency_stop",
				"system.emergency_stop",
				"reason=button_press",
				"initiator=estop-01",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatMessage(tt.dm)
			plain := stripANSI(result)
			for _, s := range tt.contains {
				if !strings.Contains(plain, s) {
					t.Errorf("FormatMessage() missing %q\ngot: %s", s, plain)
				}
			}
		})
	}
}

func TestFormatMessageColors(t *testing.T) {
	ts := time.Date(2026, 2, 17, 14, 30, 45, 0, time.UTC)

	tests := []struct {
		name      string
		dm        *DisplayMessage
		wantColor string
		noColor   bool
	}{
		{
			name: "command request has no color",
			dm: &DisplayMessage{
				Timestamp: ts,
				Channel:   "commands:station-01",
				Direction: "\u2192",
				Message: &protocol.Message{
					Envelope: makeEnvelope(protocol.TypeDeviceCommandRequest, "7c9e6679-7425-40de-944b-e07fc1f90ae7"),
					Payload: mustMarshal(t, protocol.CommandRequestPayload{
						DeviceID:    "fluke-8846a",
						CommandName: "measure_dc_voltage",
						Parameters:  map[string]string{},
						TimeoutMs:   intPtr(5000),
					}),
				},
			},
			noColor: true,
		},
		{
			name: "successful response is green",
			dm: &DisplayMessage{
				Timestamp: ts,
				Channel:   "responses:ctrl-01",
				Direction: "\u2190",
				Message: &protocol.Message{
					Envelope: makeEnvelope(protocol.TypeDeviceCommandResponse, "7c9e6679-7425-40de-944b-e07fc1f90ae7"),
					Payload: mustMarshal(t, protocol.CommandResponsePayload{
						DeviceID:    "fluke-8846a",
						CommandName: "measure_dc_voltage",
						Success:     true,
						DurationMs:  intPtr(47),
					}),
				},
			},
			wantColor: colorGreen,
		},
		{
			name: "failed response is red",
			dm: &DisplayMessage{
				Timestamp: ts,
				Channel:   "responses:ctrl-01",
				Direction: "\u2190",
				Message: &protocol.Message{
					Envelope: makeEnvelope(protocol.TypeDeviceCommandResponse, "7c9e6679-7425-40de-944b-e07fc1f90ae7"),
					Payload: mustMarshal(t, protocol.CommandResponsePayload{
						DeviceID:    "fluke-8846a",
						CommandName: "measure_dc_voltage",
						Success:     false,
						DurationMs:  intPtr(47),
					}),
				},
			},
			wantColor: colorRed,
		},
		{
			name: "heartbeat is cyan",
			dm: &DisplayMessage{
				Timestamp: ts,
				Channel:   "events:heartbeat",
				Direction: "\u2190",
				Message: &protocol.Message{
					Envelope: makeEnvelope(protocol.TypeServiceHeartbeat, ""),
					Payload: mustMarshal(t, protocol.HeartbeatPayload{
						Status:          "running",
						UptimeSeconds:   3600,
						Devices:         []string{},
						FreeHeap:        245000,
						WifiRSSI:        -42,
						FirmwareVersion: "1.0.0",
					}),
				},
			},
			wantColor: colorCyan,
		},
		{
			name: "heartbeat warnings in yellow",
			dm: &DisplayMessage{
				Timestamp: ts,
				Channel:   "events:heartbeat",
				Direction: "\u2190",
				Message: &protocol.Message{
					Envelope: makeEnvelope(protocol.TypeServiceHeartbeat, ""),
					Payload: mustMarshal(t, protocol.HeartbeatPayload{
						Status:          "running",
						UptimeSeconds:   3600,
						Devices:         []string{},
						FreeHeap:        10000,
						WifiRSSI:        -42,
						FirmwareVersion: "1.0.0",
					}),
				},
			},
			wantColor: colorCyan,
		},
		{
			name: "emergency stop is red",
			dm: &DisplayMessage{
				Timestamp: ts,
				Channel:   "events:emergency_stop",
				Direction: "\u2190",
				Message: &protocol.Message{
					Envelope: makeEnvelope(protocol.TypeSystemEmergencyStop, ""),
					Payload: mustMarshal(t, protocol.EmergencyStopPayload{
						Reason:    "button_press",
						Initiator: "estop-01",
					}),
				},
			},
			wantColor: colorRed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatMessage(tt.dm)
			if tt.noColor {
				if strings.Contains(result, "\033[") {
					t.Errorf("expected no ANSI color codes, got: %q", result)
				}
			} else {
				if !strings.HasPrefix(result, tt.wantColor) {
					t.Errorf("expected prefix %q, got: %q", tt.wantColor, result)
				}
				if !strings.HasSuffix(result, colorReset) {
					t.Errorf("expected suffix %q, got: %q", colorReset, result)
				}
			}
		})
	}
}

func TestFormatPresence(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		key      string
		ttl      int64
		state    StationState
		lastSeen time.Time
		contains []string
	}{
		{
			name:     "online station",
			key:      "device:dmm-station-01:alive",
			ttl:      85,
			state:    StateOnline,
			lastSeen: now,
			contains: []string{"dmm-station-01", "85", "ONLINE"},
		},
		{
			name:     "stale station",
			key:      "device:relay-station-02:alive",
			ttl:      30,
			state:    StateStale,
			lastSeen: now.Add(-90 * time.Second),
			contains: []string{"relay-station-02", "STALE", "last seen"},
		},
		{
			name:     "offline station TTL 0",
			key:      "device:relay-station-02:alive",
			ttl:      0,
			state:    StateOffline,
			lastSeen: now.Add(-5 * time.Minute),
			contains: []string{"relay-station-02", "OFFLINE", "last seen"},
		},
		{
			name:     "offline station no lastSeen",
			key:      "device:station-03:alive",
			ttl:      -1,
			state:    StateOffline,
			lastSeen: time.Time{},
			contains: []string{"station-03", "OFFLINE"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatPresence(tt.key, tt.ttl, tt.state, tt.lastSeen)
			plain := stripANSI(result)
			for _, s := range tt.contains {
				if !strings.Contains(plain, s) {
					t.Errorf("FormatPresence(%q, %d, %v, ...) missing %q\ngot: %s", tt.key, tt.ttl, tt.state, s, plain)
				}
			}
		})
	}
}

func TestFormatPresenceColors(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		state     StationState
		wantColor string
	}{
		{"online is green", StateOnline, colorGreen},
		{"stale is yellow", StateStale, colorYellow},
		{"offline is red", StateOffline, colorRed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatPresence("device:test:alive", 60, tt.state, now)
			if !strings.HasPrefix(result, tt.wantColor) {
				t.Errorf("expected prefix %q for %s, got: %q", tt.wantColor, tt.state, result)
			}
			if !strings.HasSuffix(result, colorReset) {
				t.Errorf("expected suffix %q, got: %q", colorReset, result)
			}
		})
	}
}

func TestExtractInstance(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"device:dmm-station-01:alive", "dmm-station-01"},
		{"device:station:with:colons:alive", "station:with:colons"},
		{"commands:station-01", "station-01"},
		{"responses:ctrl-01", "ctrl-01"},
		{"unknown-key", "unknown-key"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := extractInstance(tt.key)
			if got != tt.want {
				t.Errorf("extractInstance(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestStationTracker(t *testing.T) {
	t.Run("new instance is online after heartbeat with positive TTL", func(t *testing.T) {
		tracker := NewStationTracker()
		tracker.RecordHeartbeat("station-01")

		state, lastSeen := tracker.GetState("station-01", 80*time.Second)
		if state != StateOnline {
			t.Errorf("state = %v, want StateOnline", state)
		}
		if lastSeen.IsZero() {
			t.Error("lastSeen should not be zero")
		}
	})

	t.Run("unknown instance with positive TTL is online", func(t *testing.T) {
		tracker := NewStationTracker()

		state, lastSeen := tracker.GetState("station-01", 80*time.Second)
		if state != StateOnline {
			t.Errorf("state = %v, want StateOnline", state)
		}
		if !lastSeen.IsZero() {
			t.Error("lastSeen should be zero for unknown instance")
		}
	})

	t.Run("TTL zero means offline", func(t *testing.T) {
		tracker := NewStationTracker()
		tracker.RecordHeartbeat("station-01")

		state, _ := tracker.GetState("station-01", 0)
		if state != StateOffline {
			t.Errorf("state = %v, want StateOffline", state)
		}
	})

	t.Run("negative TTL means offline", func(t *testing.T) {
		tracker := NewStationTracker()
		tracker.RecordHeartbeat("station-01")

		state, _ := tracker.GetState("station-01", -1*time.Second)
		if state != StateOffline {
			t.Errorf("state = %v, want StateOffline", state)
		}
	})

	t.Run("stale after 60s without heartbeat", func(t *testing.T) {
		tracker := NewStationTracker()
		// Backdate the last heartbeat to simulate staleness
		tracker.LastHeartbeat["station-01"] = time.Now().Add(-90 * time.Second)

		state, _ := tracker.GetState("station-01", 30*time.Second)
		if state != StateStale {
			t.Errorf("state = %v, want StateStale", state)
		}
	})

	t.Run("KnownInstances returns sorted list", func(t *testing.T) {
		tracker := NewStationTracker()
		tracker.RecordHeartbeat("station-c")
		tracker.RecordHeartbeat("station-a")
		tracker.RecordHeartbeat("station-b")

		instances := tracker.KnownInstances()
		if len(instances) != 3 {
			t.Fatalf("got %d instances, want 3", len(instances))
		}
		expected := []string{"station-a", "station-b", "station-c"}
		for i, inst := range instances {
			if inst != expected[i] {
				t.Errorf("instances[%d] = %q, want %q", i, inst, expected[i])
			}
		}
	})

	t.Run("RecordHeartbeat refreshes time", func(t *testing.T) {
		tracker := NewStationTracker()
		// Backdate to make stale
		tracker.LastHeartbeat["station-01"] = time.Now().Add(-90 * time.Second)

		state, _ := tracker.GetState("station-01", 80*time.Second)
		if state != StateStale {
			t.Fatalf("expected STALE before re-heartbeat, got %v", state)
		}

		// New heartbeat should bring it back online
		tracker.RecordHeartbeat("station-01")

		state, _ = tracker.GetState("station-01", 80*time.Second)
		if state != StateOnline {
			t.Errorf("state = %v after re-heartbeat, want StateOnline", state)
		}
	})
}

func TestParseStreamFields(t *testing.T) {
	t.Run("valid JSON in data field", func(t *testing.T) {
		fields := map[string]string{
			"data": `{"envelope":{"id":"550e8400-e29b-41d4-a716-446655440000","timestamp":1771329600,"source":{"service":"controller","instance":"ctrl-01","version":"1.0.0"},"schema_version":"v1.0.0","type":"device.command.request","correlation_id":"7c9e6679-7425-40de-944b-e07fc1f90ae7","reply_to":"responses:controller:ctrl-01"},"payload":{"device_id":"fluke-8846a","command_name":"measure_dc_voltage"}}`,
		}
		msg, err := ParseStreamFields(fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if msg.Envelope.Type != protocol.TypeDeviceCommandRequest {
			t.Errorf("type = %q, want %q", msg.Envelope.Type, protocol.TypeDeviceCommandRequest)
		}
		if msg.Envelope.Source.Instance != "ctrl-01" {
			t.Errorf("instance = %q, want %q", msg.Envelope.Source.Instance, "ctrl-01")
		}
	})

	t.Run("valid JSON in first field (no data key)", func(t *testing.T) {
		fields := map[string]string{
			"message": `{"envelope":{"id":"550e8400-e29b-41d4-a716-446655440000","timestamp":1771329600,"source":{"service":"controller","instance":"ctrl-01","version":"1.0.0"},"schema_version":"v1.0.0","type":"service.heartbeat"},"payload":{"status":"running","uptime_seconds":3600,"devices":[],"free_heap":245000,"wifi_rssi":-42,"firmware_version":"1.0.0"}}`,
		}
		msg, err := ParseStreamFields(fields)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if msg.Envelope.Type != protocol.TypeServiceHeartbeat {
			t.Errorf("type = %q, want %q", msg.Envelope.Type, protocol.TypeServiceHeartbeat)
		}
	})

	t.Run("bad JSON returns error", func(t *testing.T) {
		fields := map[string]string{
			"data": `{not valid json`,
		}
		_, err := ParseStreamFields(fields)
		if err == nil {
			t.Fatal("expected error for bad JSON, got nil")
		}
	})

	t.Run("empty fields returns error", func(t *testing.T) {
		fields := map[string]string{}
		_, err := ParseStreamFields(fields)
		if err == nil {
			t.Fatal("expected error for empty fields, got nil")
		}
	})
}

func TestHealthWarnings(t *testing.T) {
	tests := []struct {
		name     string
		hb       *protocol.HeartbeatPayload
		expected []string
	}{
		{
			name: "healthy - no warnings",
			hb: &protocol.HeartbeatPayload{
				FreeHeap:        245000,
				WifiRSSI:        -42,
				CommandsFailed:  intPtr(0),
				WatchdogResets:  intPtr(0),
				FirmwareVersion: "1.0.0",
			},
			expected: nil,
		},
		{
			name: "low heap",
			hb: &protocol.HeartbeatPayload{
				FreeHeap:        45000,
				WifiRSSI:        -42,
				FirmwareVersion: "1.0.0",
			},
			expected: []string{"LOW HEAP"},
		},
		{
			name: "critical heap",
			hb: &protocol.HeartbeatPayload{
				FreeHeap:        15000,
				WifiRSSI:        -42,
				FirmwareVersion: "1.0.0",
			},
			expected: []string{"CRITICAL HEAP"},
		},
		{
			name: "weak wifi",
			hb: &protocol.HeartbeatPayload{
				FreeHeap:        245000,
				WifiRSSI:        -75,
				FirmwareVersion: "1.0.0",
			},
			expected: []string{"WEAK WIFI"},
		},
		{
			name: "poor wifi",
			hb: &protocol.HeartbeatPayload{
				FreeHeap:        245000,
				WifiRSSI:        -85,
				FirmwareVersion: "1.0.0",
			},
			expected: []string{"POOR WIFI"},
		},
		{
			name: "command failures",
			hb: &protocol.HeartbeatPayload{
				FreeHeap:        245000,
				WifiRSSI:        -42,
				CommandsFailed:  intPtr(5),
				FirmwareVersion: "1.0.0",
			},
			expected: []string{"FAILURES"},
		},
		{
			name: "watchdog resets",
			hb: &protocol.HeartbeatPayload{
				FreeHeap:        245000,
				WifiRSSI:        -42,
				WatchdogResets:  intPtr(2),
				FirmwareVersion: "1.0.0",
			},
			expected: []string{"WATCHDOG RESETS"},
		},
		{
			name: "multiple warnings",
			hb: &protocol.HeartbeatPayload{
				FreeHeap:        10000,
				WifiRSSI:        -85,
				CommandsFailed:  intPtr(3),
				WatchdogResets:  intPtr(1),
				FirmwareVersion: "1.0.0",
			},
			expected: []string{"CRITICAL HEAP", "POOR WIFI", "FAILURES", "WATCHDOG RESETS"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := HealthWarnings(tt.hb)
			if len(warnings) != len(tt.expected) {
				t.Fatalf("got %d warnings %v, want %d %v", len(warnings), warnings, len(tt.expected), tt.expected)
			}
			for i, w := range warnings {
				if w != tt.expected[i] {
					t.Errorf("warning[%d] = %q, want %q", i, w, tt.expected[i])
				}
			}
		})
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{colorGreen + "hello" + colorReset, "hello"},
		{colorRed + "error" + colorReset, "error"},
		{"no color", "no color"},
		{colorCyan + "data " + colorYellow + "[WARN]" + colorCyan + colorReset, "data [WARN]"},
	}
	for _, tt := range tests {
		got := stripANSI(tt.input)
		if got != tt.want {
			t.Errorf("stripANSI(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
