package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// validHeartbeatMessage returns a minimal valid heartbeat message for testing.
func validHeartbeatMessage() *Message {
	payload := HeartbeatPayload{
		Status:          "running",
		UptimeSeconds:   100,
		Devices:         []string{"dev-1"},
		FreeHeap:        200000,
		WifiRSSI:        -50,
		FirmwareVersion: "1.0.0",
	}
	payloadBytes, _ := json.Marshal(payload)
	return &Message{
		Envelope: Envelope{
			ID:            "550e8400-e29b-41d4-a716-446655440000",
			Timestamp:     1771329600,
			Source:        Source{Service: "esp32_tcp_bridge", Instance: "station-01", Version: "1.0.0"},
			SchemaVersion: "v1.0.0",
			Type:          TypeServiceHeartbeat,
		},
		Payload: json.RawMessage(payloadBytes),
	}
}

func validCommandRequestMessage() *Message {
	payload := CommandRequestPayload{
		DeviceID:    "fluke-8846a",
		CommandName: "measure_dc_voltage",
	}
	payloadBytes, _ := json.Marshal(payload)
	return &Message{
		Envelope: Envelope{
			ID:            "550e8400-e29b-41d4-a716-446655440000",
			Timestamp:     1771329600,
			Source:        Source{Service: "controller", Instance: "ctrl-01", Version: "1.0.0"},
			SchemaVersion: "v1.0.0",
			Type:          TypeDeviceCommandRequest,
			CorrelationID: "7c9e6679-7425-40de-944b-e07fc1f90ae7",
			ReplyTo:       "responses:controller:ctrl-01",
		},
		Payload: json.RawMessage(payloadBytes),
	}
}

func validCommandResponseMessage() *Message {
	resp := "1.23456789"
	payload := CommandResponsePayload{
		DeviceID:    "fluke-8846a",
		CommandName: "measure_dc_voltage",
		Success:     true,
		Response:    &resp,
	}
	payloadBytes, _ := json.Marshal(payload)
	return &Message{
		Envelope: Envelope{
			ID:            "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
			Timestamp:     1771329600,
			Source:        Source{Service: "esp32_tcp_bridge", Instance: "dmm-station-01", Version: "1.0.0"},
			SchemaVersion: "v1.0.0",
			Type:          TypeDeviceCommandResponse,
			CorrelationID: "7c9e6679-7425-40de-944b-e07fc1f90ae7",
		},
		Payload: json.RawMessage(payloadBytes),
	}
}

func validEmergencyStopMessage() *Message {
	payload := EmergencyStopPayload{
		Reason:    "button_press",
		Initiator: "estop-01",
	}
	payloadBytes, _ := json.Marshal(payload)
	return &Message{
		Envelope: Envelope{
			ID:            "e4f5a6b7-c8d9-4e0f-9a2b-3c4d5e6f7a8b",
			Timestamp:     1771329795,
			Source:        Source{Service: "esp32_estop", Instance: "estop-01", Version: "1.0.0"},
			SchemaVersion: "v1.0.0",
			Type:          TypeSystemEmergencyStop,
		},
		Payload: json.RawMessage(payloadBytes),
	}
}

func validOTARequestMessage() *Message {
	payload := OTARequestPayload{
		FirmwareURL: "http://192.168.1.10:8080/firmware/v1.1.0.bin",
		Version:     "1.1.0",
		SHA256:      "a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1",
	}
	payloadBytes, _ := json.Marshal(payload)
	return &Message{
		Envelope: Envelope{
			ID:            "a6b7c8d9-e0f1-4a2b-8c4d-5e6f7a8b9c0d",
			Timestamp:     1771336800,
			Source:        Source{Service: "controller", Instance: "ctrl-01", Version: "1.0.0"},
			SchemaVersion: "v1.0.0",
			Type:          TypeSystemOTARequest,
			CorrelationID: "b7c8d9e0-f1a2-4b3c-8d5e-6f7a8b9c0d1e",
			ReplyTo:       "responses:controller:ctrl-01",
		},
		Payload: json.RawMessage(payloadBytes),
	}
}

func TestValidateAllTypes(t *testing.T) {
	tests := []struct {
		name string
		msg  *Message
	}{
		{"heartbeat", validHeartbeatMessage()},
		{"command_request", validCommandRequestMessage()},
		{"command_response", validCommandResponseMessage()},
		{"emergency_stop", validEmergencyStopMessage()},
		{"ota_request", validOTARequestMessage()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Validate(tt.msg); err != nil {
				t.Errorf("Validate() error: %v", err)
			}
		})
	}
}

func TestValidateExampleFiles(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"heartbeat_healthy", "service-heartbeat/examples/healthy.json"},
		{"command_request_voltage", "device-command-request/examples/measure_voltage.json"},
		{"command_request_relay", "device-command-request/examples/set_relay.json"},
		{"command_response_success", "device-command-response/examples/success.json"},
		{"command_response_error", "device-command-response/examples/error_timeout.json"},
		{"emergency_stop_button", "system-emergency-stop/examples/button_press.json"},
		{"ota_request_standard", "system-ota-request/examples/standard_update.json"},
	}

	base := schemasDir()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(base, tt.path))
			if err != nil {
				t.Fatalf("read file: %v", err)
			}
			msg, err := Parse(data)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if err := Validate(msg); err != nil {
				t.Errorf("Validate() error: %v", err)
			}
		})
	}
}

func TestValidateInvalidMessages(t *testing.T) {
	tests := []struct {
		name   string
		modify func(msg *Message)
	}{
		{
			name: "empty_id",
			modify: func(msg *Message) {
				msg.Envelope.ID = ""
			},
		},
		{
			name: "invalid_id_format",
			modify: func(msg *Message) {
				msg.Envelope.ID = "not-a-uuid"
			},
		},
		{
			name: "uuid_v1_rejected",
			modify: func(msg *Message) {
				// UUIDv1 has version nibble '1' instead of '4'
				msg.Envelope.ID = "550e8400-e29b-11d4-a716-446655440000"
			},
		},
		{
			name: "negative_timestamp",
			modify: func(msg *Message) {
				msg.Envelope.Timestamp = -1
			},
		},
		{
			name: "wrong_schema_version",
			modify: func(msg *Message) {
				msg.Envelope.SchemaVersion = "v2.0.0"
			},
		},
		{
			name: "unknown_type",
			modify: func(msg *Message) {
				msg.Envelope.Type = "unknown.type"
			},
		},
		{
			name: "invalid_source_service_uppercase",
			modify: func(msg *Message) {
				msg.Envelope.Source.Service = "Controller"
			},
		},
		{
			name: "invalid_source_service_starts_with_number",
			modify: func(msg *Message) {
				msg.Envelope.Source.Service = "1controller"
			},
		},
		{
			name: "empty_source_service",
			modify: func(msg *Message) {
				msg.Envelope.Source.Service = ""
			},
		},
		{
			name: "invalid_source_instance",
			modify: func(msg *Message) {
				msg.Envelope.Source.Instance = "STATION 01"
			},
		},
		{
			name: "invalid_source_version",
			modify: func(msg *Message) {
				msg.Envelope.Source.Version = "v1.0"
			},
		},
		{
			name: "invalid_correlation_id_format",
			modify: func(msg *Message) {
				msg.Envelope.CorrelationID = "not-a-valid-uuid"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := validHeartbeatMessage()
			tt.modify(msg)
			if err := Validate(msg); err == nil {
				t.Error("Validate() expected error, got nil")
			}
		})
	}
}

func TestValidateRequestMissingCorrelationID(t *testing.T) {
	msg := validCommandRequestMessage()
	msg.Envelope.CorrelationID = ""
	if err := Validate(msg); err == nil {
		t.Error("Validate() expected error for missing correlation_id on request")
	}
}

func TestValidateRequestMissingReplyTo(t *testing.T) {
	msg := validCommandRequestMessage()
	msg.Envelope.ReplyTo = ""
	if err := Validate(msg); err == nil {
		t.Error("Validate() expected error for missing reply_to on request")
	}
}

func TestValidateOTARequestMissingCorrelationID(t *testing.T) {
	msg := validOTARequestMessage()
	msg.Envelope.CorrelationID = ""
	if err := Validate(msg); err == nil {
		t.Error("Validate() expected error for missing correlation_id on OTA request")
	}
}

func TestValidateOTARequestMissingReplyTo(t *testing.T) {
	msg := validOTARequestMessage()
	msg.Envelope.ReplyTo = ""
	if err := Validate(msg); err == nil {
		t.Error("Validate() expected error for missing reply_to on OTA request")
	}
}

func TestValidateResponseMissingCorrelationID(t *testing.T) {
	msg := validCommandResponseMessage()
	msg.Envelope.CorrelationID = ""
	if err := Validate(msg); err == nil {
		t.Error("Validate() expected error for missing correlation_id on response")
	}
}

func TestValidateHeartbeatOnlyRequiredFields(t *testing.T) {
	msg := validHeartbeatMessage()
	// Heartbeat doesn't require correlation_id or reply_to
	msg.Envelope.CorrelationID = ""
	msg.Envelope.ReplyTo = ""
	if err := Validate(msg); err != nil {
		t.Errorf("Validate() error on minimal heartbeat: %v", err)
	}
}
