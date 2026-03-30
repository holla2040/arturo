package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testSource() Source {
	return Source{
		Service:  "controller",
		Instance: "ctrl-01",
		Version:  "1.0.0",
	}
}

func TestNewEnvelope(t *testing.T) {
	src := testSource()
	env := NewEnvelope(src, TypeServiceHeartbeat)

	if !uuidV4Pattern.MatchString(env.ID) {
		t.Errorf("NewEnvelope ID is not valid UUIDv4: %q", env.ID)
	}
	if env.Timestamp <= 0 {
		t.Errorf("NewEnvelope Timestamp should be positive, got %d", env.Timestamp)
	}
	if env.SchemaVersion != SchemaVersion {
		t.Errorf("NewEnvelope SchemaVersion = %q, want %q", env.SchemaVersion, SchemaVersion)
	}
	if env.Type != TypeServiceHeartbeat {
		t.Errorf("NewEnvelope Type = %q, want %q", env.Type, TypeServiceHeartbeat)
	}
	if env.Source.Service != src.Service {
		t.Errorf("NewEnvelope Source.Service = %q, want %q", env.Source.Service, src.Service)
	}
}

func TestNewMessageRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		msgType string
		payload interface{}
	}{
		{
			name:    "heartbeat",
			msgType: TypeServiceHeartbeat,
			payload: HeartbeatPayload{
				Status:          "running",
				UptimeSeconds:   3600,
				Devices:         []string{"fluke-8846a"},
				FreeHeap:        245000,
				WifiRSSI:        -42,
				FirmwareVersion: "1.0.0",
			},
		},
		{
			name:    "command_request",
			msgType: TypeDeviceCommandRequest,
			payload: CommandRequestPayload{
				DeviceID:    "fluke-8846a",
				CommandName: "measure_dc_voltage",
			},
		},
		{
			name:    "emergency_stop",
			msgType: TypeSystemEmergencyStop,
			payload: EmergencyStopPayload{
				Reason:      "button_press",
				Description: "Physical E-stop button pressed",
				Initiator:   "estop-01",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := NewMessage(testSource(), tt.msgType, tt.payload)
			if err != nil {
				t.Fatalf("NewMessage() error: %v", err)
			}

			data, err := json.Marshal(msg)
			if err != nil {
				t.Fatalf("json.Marshal() error: %v", err)
			}

			parsed, err := Parse(data)
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}

			if parsed.Envelope.Type != tt.msgType {
				t.Errorf("round-trip Type = %q, want %q", parsed.Envelope.Type, tt.msgType)
			}
			if parsed.Envelope.ID != msg.Envelope.ID {
				t.Errorf("round-trip ID = %q, want %q", parsed.Envelope.ID, msg.Envelope.ID)
			}
			if parsed.Envelope.SchemaVersion != SchemaVersion {
				t.Errorf("round-trip SchemaVersion = %q, want %q", parsed.Envelope.SchemaVersion, SchemaVersion)
			}
		})
	}
}

// schemasDir returns the path to the schemas directory.
func schemasDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "schemas", "v1.0.0")
}

func TestParseExampleFiles(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		msgType string
	}{
		{"heartbeat_healthy", "service-heartbeat/examples/healthy.json", TypeServiceHeartbeat},
		{"command_request_voltage", "device-command-request/examples/measure_voltage.json", TypeDeviceCommandRequest},
		{"command_request_relay", "device-command-request/examples/set_relay.json", TypeDeviceCommandRequest},
		{"command_response_success", "device-command-response/examples/success.json", TypeDeviceCommandResponse},
		{"command_response_error", "device-command-response/examples/error_timeout.json", TypeDeviceCommandResponse},
		{"emergency_stop_button", "system-emergency-stop/examples/button_press.json", TypeSystemEmergencyStop},
		{"ota_request_standard", "system-ota-request/examples/standard_update.json", TypeSystemOTARequest},
	}

	base := schemasDir()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(base, tt.path))
			if err != nil {
				t.Fatalf("read example file: %v", err)
			}

			msg, err := Parse(data)
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}

			if msg.Envelope.Type != tt.msgType {
				t.Errorf("Type = %q, want %q", msg.Envelope.Type, tt.msgType)
			}
			if msg.Envelope.SchemaVersion != SchemaVersion {
				t.Errorf("SchemaVersion = %q, want %q", msg.Envelope.SchemaVersion, SchemaVersion)
			}
		})
	}
}

func TestParseInvalidJSON(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"empty", ""},
		{"not_json", "this is not json"},
		{"incomplete", `{"envelope":`},
		{"wrong_type", `[]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.data))
			if err == nil {
				t.Error("Parse() expected error, got nil")
			}
		})
	}
}

func TestTypedPayloadParsers(t *testing.T) {
	base := schemasDir()

	t.Run("heartbeat", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(base, "service-heartbeat/examples/healthy.json"))
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		msg, err := Parse(data)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		p, err := ParseHeartbeat(msg)
		if err != nil {
			t.Fatalf("ParseHeartbeat: %v", err)
		}
		if p.Status != "running" {
			t.Errorf("Status = %q, want %q", p.Status, "running")
		}
		if p.UptimeSeconds != 3600 {
			t.Errorf("UptimeSeconds = %d, want 3600", p.UptimeSeconds)
		}
		if len(p.Devices) != 1 || p.Devices[0] != "fluke-8846a" {
			t.Errorf("Devices = %v, want [fluke-8846a]", p.Devices)
		}
		if p.FirmwareVersion != "1.0.0" {
			t.Errorf("FirmwareVersion = %q, want %q", p.FirmwareVersion, "1.0.0")
		}
	})

	t.Run("command_request", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(base, "device-command-request/examples/measure_voltage.json"))
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		msg, err := Parse(data)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		p, err := ParseCommandRequest(msg)
		if err != nil {
			t.Fatalf("ParseCommandRequest: %v", err)
		}
		if p.DeviceID != "fluke-8846a" {
			t.Errorf("DeviceID = %q, want %q", p.DeviceID, "fluke-8846a")
		}
		if p.CommandName != "measure_dc_voltage" {
			t.Errorf("CommandName = %q, want %q", p.CommandName, "measure_dc_voltage")
		}
	})

	t.Run("command_response_success", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(base, "device-command-response/examples/success.json"))
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		msg, err := Parse(data)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		p, err := ParseCommandResponse(msg)
		if err != nil {
			t.Fatalf("ParseCommandResponse: %v", err)
		}
		if !p.Success {
			t.Error("Success should be true")
		}
		if p.Response == nil || *p.Response != "1.23456789" {
			t.Errorf("Response = %v, want \"1.23456789\"", p.Response)
		}
		if p.Error != nil {
			t.Error("Error should be nil on success")
		}
	})

	t.Run("command_response_error", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(base, "device-command-response/examples/error_timeout.json"))
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		msg, err := Parse(data)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		p, err := ParseCommandResponse(msg)
		if err != nil {
			t.Fatalf("ParseCommandResponse: %v", err)
		}
		if p.Success {
			t.Error("Success should be false")
		}
		if p.Error == nil {
			t.Fatal("Error should not be nil")
		}
		if p.Error.Code != "E_DEVICE_TIMEOUT" {
			t.Errorf("Error.Code = %q, want %q", p.Error.Code, "E_DEVICE_TIMEOUT")
		}
	})

	t.Run("emergency_stop", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(base, "system-emergency-stop/examples/button_press.json"))
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		msg, err := Parse(data)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		p, err := ParseEmergencyStop(msg)
		if err != nil {
			t.Fatalf("ParseEmergencyStop: %v", err)
		}
		if p.Reason != "button_press" {
			t.Errorf("Reason = %q, want %q", p.Reason, "button_press")
		}
		if p.Initiator != "estop-01" {
			t.Errorf("Initiator = %q, want %q", p.Initiator, "estop-01")
		}
	})

	t.Run("ota_request", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(base, "system-ota-request/examples/standard_update.json"))
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		msg, err := Parse(data)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		p, err := ParseOTARequest(msg)
		if err != nil {
			t.Fatalf("ParseOTARequest: %v", err)
		}
		if p.Version != "1.1.0" {
			t.Errorf("Version = %q, want %q", p.Version, "1.1.0")
		}
		if p.SHA256 != "a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1" {
			t.Errorf("SHA256 = %q, want expected hash", p.SHA256)
		}
	})
}
