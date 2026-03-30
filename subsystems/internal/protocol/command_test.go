package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildCommandRequest(t *testing.T) {
	src := testSource()
	msg, err := BuildCommandRequest(src, "fluke-8846a", "measure_dc_voltage", nil, 5000)
	if err != nil {
		t.Fatalf("BuildCommandRequest() error: %v", err)
	}

	if msg.Envelope.Type != TypeDeviceCommandRequest {
		t.Errorf("Type = %q, want %q", msg.Envelope.Type, TypeDeviceCommandRequest)
	}
	if !uuidV4Pattern.MatchString(msg.Envelope.ID) {
		t.Errorf("ID is not valid UUIDv4: %q", msg.Envelope.ID)
	}
	if !uuidV4Pattern.MatchString(msg.Envelope.CorrelationID) {
		t.Errorf("CorrelationID is not valid UUIDv4: %q", msg.Envelope.CorrelationID)
	}
	if msg.Envelope.ID == msg.Envelope.CorrelationID {
		t.Error("ID and CorrelationID should be different UUIDs")
	}
	if msg.Envelope.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", msg.Envelope.SchemaVersion, SchemaVersion)
	}
	if msg.Envelope.Timestamp <= 0 {
		t.Errorf("Timestamp should be positive, got %d", msg.Envelope.Timestamp)
	}
}

func TestBuildCommandRequestValidates(t *testing.T) {
	src := testSource()
	msg, err := BuildCommandRequest(src, "fluke-8846a", "measure_dc_voltage", nil, 5000)
	if err != nil {
		t.Fatalf("BuildCommandRequest() error: %v", err)
	}

	if err := Validate(msg); err != nil {
		t.Errorf("Validate() error on BuildCommandRequest message: %v", err)
	}
}

func TestBuildCommandRequestPayload(t *testing.T) {
	src := testSource()
	params := map[string]string{"range": "10V", "resolution": "0.001"}
	msg, err := BuildCommandRequest(src, "fluke-8846a", "measure_dc_voltage", params, 3000)
	if err != nil {
		t.Fatalf("BuildCommandRequest() error: %v", err)
	}

	var p CommandRequestPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if p.DeviceID != "fluke-8846a" {
		t.Errorf("DeviceID = %q, want %q", p.DeviceID, "fluke-8846a")
	}
	if p.CommandName != "measure_dc_voltage" {
		t.Errorf("CommandName = %q, want %q", p.CommandName, "measure_dc_voltage")
	}
	if p.Parameters["range"] != "10V" {
		t.Errorf("Parameters[range] = %q, want %q", p.Parameters["range"], "10V")
	}
	if p.Parameters["resolution"] != "0.001" {
		t.Errorf("Parameters[resolution] = %q, want %q", p.Parameters["resolution"], "0.001")
	}
	if p.TimeoutMs == nil || *p.TimeoutMs != 3000 {
		t.Errorf("TimeoutMs = %v, want 3000", p.TimeoutMs)
	}
}

func TestBuildCommandRequestReplyTo(t *testing.T) {
	tests := []struct {
		name     string
		instance string
		wantRT   string
	}{
		{"ctrl-01", "ctrl-01", "responses:ctrl-01"},
		{"server-01", "server-01", "responses:server-01"},
		{"engine-main", "engine-main", "responses:engine-main"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := Source{Service: "controller", Instance: tt.instance, Version: "1.0.0"}
			msg, err := BuildCommandRequest(src, "dev-1", "cmd", nil, 5000)
			if err != nil {
				t.Fatalf("BuildCommandRequest() error: %v", err)
			}
			if msg.Envelope.ReplyTo != tt.wantRT {
				t.Errorf("ReplyTo = %q, want %q", msg.Envelope.ReplyTo, tt.wantRT)
			}
			if !strings.HasPrefix(msg.Envelope.ReplyTo, "responses:") {
				t.Errorf("ReplyTo should start with 'responses:', got %q", msg.Envelope.ReplyTo)
			}
		})
	}
}

func TestBuildCommandRequestNilParams(t *testing.T) {
	src := testSource()
	msg, err := BuildCommandRequest(src, "fluke-8846a", "*IDN?", nil, 5000)
	if err != nil {
		t.Fatalf("BuildCommandRequest() error: %v", err)
	}

	var p CommandRequestPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if p.CommandName != "*IDN?" {
		t.Errorf("CommandName = %q, want %q", p.CommandName, "*IDN?")
	}
}

func TestBuildCommandRequestRoundTrip(t *testing.T) {
	src := testSource()
	msg, err := BuildCommandRequest(src, "fluke-8846a", "measure_dc_voltage", nil, 5000)
	if err != nil {
		t.Fatalf("BuildCommandRequest() error: %v", err)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}

	parsed, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if parsed.Envelope.Type != TypeDeviceCommandRequest {
		t.Errorf("round-trip Type = %q, want %q", parsed.Envelope.Type, TypeDeviceCommandRequest)
	}
	if parsed.Envelope.CorrelationID != msg.Envelope.CorrelationID {
		t.Errorf("round-trip CorrelationID = %q, want %q", parsed.Envelope.CorrelationID, msg.Envelope.CorrelationID)
	}
	if parsed.Envelope.ReplyTo != msg.Envelope.ReplyTo {
		t.Errorf("round-trip ReplyTo = %q, want %q", parsed.Envelope.ReplyTo, msg.Envelope.ReplyTo)
	}

	p, err := ParseCommandRequest(parsed)
	if err != nil {
		t.Fatalf("ParseCommandRequest() error: %v", err)
	}
	if p.DeviceID != "fluke-8846a" {
		t.Errorf("round-trip DeviceID = %q, want %q", p.DeviceID, "fluke-8846a")
	}
}

// --- OTA Request Builder Tests ---

func TestBuildOTARequest(t *testing.T) {
	src := testSource()
	msg, err := BuildOTARequest(src,
		"http://192.168.1.10:8080/firmware/v1.1.0.bin",
		"1.1.0",
		"a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1",
		false,
	)
	if err != nil {
		t.Fatalf("BuildOTARequest() error: %v", err)
	}

	if msg.Envelope.Type != TypeSystemOTARequest {
		t.Errorf("Type = %q, want %q", msg.Envelope.Type, TypeSystemOTARequest)
	}
	if !uuidV4Pattern.MatchString(msg.Envelope.ID) {
		t.Errorf("ID is not valid UUIDv4: %q", msg.Envelope.ID)
	}
	if !uuidV4Pattern.MatchString(msg.Envelope.CorrelationID) {
		t.Errorf("CorrelationID is not valid UUIDv4: %q", msg.Envelope.CorrelationID)
	}
	if msg.Envelope.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", msg.Envelope.SchemaVersion, SchemaVersion)
	}
	if !strings.HasPrefix(msg.Envelope.ReplyTo, "responses:") {
		t.Errorf("ReplyTo should start with 'responses:', got %q", msg.Envelope.ReplyTo)
	}
}

func TestBuildOTARequestPayload(t *testing.T) {
	src := testSource()
	msg, err := BuildOTARequest(src,
		"http://192.168.1.10:8080/firmware/v1.1.0.bin",
		"1.1.0",
		"a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1",
		false,
	)
	if err != nil {
		t.Fatalf("BuildOTARequest() error: %v", err)
	}

	var p OTARequestPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if p.FirmwareURL != "http://192.168.1.10:8080/firmware/v1.1.0.bin" {
		t.Errorf("FirmwareURL = %q, want %q", p.FirmwareURL, "http://192.168.1.10:8080/firmware/v1.1.0.bin")
	}
	if p.Version != "1.1.0" {
		t.Errorf("Version = %q, want %q", p.Version, "1.1.0")
	}
	if p.SHA256 != "a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1" {
		t.Errorf("SHA256 = %q, want expected hash", p.SHA256)
	}
	if p.Force == nil || *p.Force != false {
		t.Errorf("Force = %v, want false", p.Force)
	}
}

func TestBuildOTARequestForced(t *testing.T) {
	src := testSource()
	msg, err := BuildOTARequest(src,
		"http://example.com/fw.bin",
		"2.0.0",
		"0000000000000000000000000000000000000000000000000000000000000000",
		true,
	)
	if err != nil {
		t.Fatalf("BuildOTARequest() error: %v", err)
	}

	var p OTARequestPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if p.Force == nil || *p.Force != true {
		t.Errorf("Force = %v, want true", p.Force)
	}
}

func TestBuildOTARequestRoundTrip(t *testing.T) {
	src := testSource()
	msg, err := BuildOTARequest(src,
		"http://192.168.1.10:8080/firmware/v1.1.0.bin",
		"1.1.0",
		"a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1",
		false,
	)
	if err != nil {
		t.Fatalf("BuildOTARequest() error: %v", err)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}

	parsed, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if parsed.Envelope.Type != TypeSystemOTARequest {
		t.Errorf("round-trip Type = %q, want %q", parsed.Envelope.Type, TypeSystemOTARequest)
	}

	p, err := ParseOTARequest(parsed)
	if err != nil {
		t.Fatalf("ParseOTARequest() error: %v", err)
	}
	if p.FirmwareURL != "http://192.168.1.10:8080/firmware/v1.1.0.bin" {
		t.Errorf("round-trip FirmwareURL = %q", p.FirmwareURL)
	}
	if p.Version != "1.1.0" {
		t.Errorf("round-trip Version = %q", p.Version)
	}
}

func TestBuildOTARequestValidates(t *testing.T) {
	src := testSource()
	msg, err := BuildOTARequest(src,
		"http://192.168.1.10:8080/firmware/v1.1.0.bin",
		"1.1.0",
		"a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1",
		false,
	)
	if err != nil {
		t.Fatalf("BuildOTARequest() error: %v", err)
	}

	if err := Validate(msg); err != nil {
		t.Errorf("Validate() error on BuildOTARequest message: %v", err)
	}
}
