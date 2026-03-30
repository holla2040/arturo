package protocol

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Message type constants.
const (
	TypeDeviceCommandRequest  = "device.command.request"
	TypeDeviceCommandResponse = "device.command.response"
	TypeServiceHeartbeat      = "service.heartbeat"
	TypeSystemEmergencyStop   = "system.emergency_stop"
	TypeSystemOTARequest      = "system.ota.request"
)

// ValidMessageTypes lists all valid message types.
var ValidMessageTypes = []string{
	TypeDeviceCommandRequest,
	TypeDeviceCommandResponse,
	TypeServiceHeartbeat,
	TypeSystemEmergencyStop,
	TypeSystemOTARequest,
}

// SchemaVersion is the current protocol version.
const SchemaVersion = "v1.0.0"

// Message is the top-level protocol message containing an envelope and payload.
type Message struct {
	Envelope Envelope        `json:"envelope"`
	Payload  json.RawMessage `json:"payload"`
}

// Envelope contains message metadata and routing information.
type Envelope struct {
	ID            string `json:"id"`
	Timestamp     int64  `json:"timestamp"`
	Source        Source `json:"source"`
	SchemaVersion string `json:"schema_version"`
	Type          string `json:"type"`
	CorrelationID string `json:"correlation_id,omitempty"`
	ReplyTo       string `json:"reply_to,omitempty"`
}

// Source identifies who sent a message.
type Source struct {
	Service  string `json:"service"`
	Instance string `json:"instance"`
	Version  string `json:"version"`
}

// Error is a standard error object used in response payloads.
type Error struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// HeartbeatPayload contains fields from the service.heartbeat payload.
type HeartbeatPayload struct {
	Status            string            `json:"status"`
	UptimeSeconds     int64             `json:"uptime_seconds"`
	Devices           []string          `json:"devices"`
	DeviceTypes       map[string]string `json:"device_types,omitempty"`
	FreeHeap          int64             `json:"free_heap"`
	MinFreeHeap       *int64   `json:"min_free_heap,omitempty"`
	WifiRSSI          int      `json:"wifi_rssi"`
	WifiReconnects    *int     `json:"wifi_reconnects,omitempty"`
	RedisReconnects   *int     `json:"redis_reconnects,omitempty"`
	CommandsProcessed *int     `json:"commands_processed,omitempty"`
	CommandsFailed    *int     `json:"commands_failed,omitempty"`
	LastError         *string  `json:"last_error"`
	WatchdogResets    *int     `json:"watchdog_resets,omitempty"`
	FirmwareVersion   string   `json:"firmware_version"`
}

// CommandRequestPayload contains fields from the device.command.request payload.
type CommandRequestPayload struct {
	DeviceID    string            `json:"device_id"`
	CommandName string            `json:"command_name"`
	Parameters  map[string]string `json:"parameters,omitempty"`
	TimeoutMs   *int              `json:"timeout_ms,omitempty"`
}

// CommandResponsePayload contains fields from the device.command.response payload.
type CommandResponsePayload struct {
	DeviceID    string  `json:"device_id"`
	CommandName string  `json:"command_name"`
	Success     bool    `json:"success"`
	Response    *string `json:"response"`
	Error       *Error  `json:"error,omitempty"`
	DurationMs  *int    `json:"duration_ms,omitempty"`
}

// EmergencyStopPayload contains fields from the system.emergency_stop payload.
type EmergencyStopPayload struct {
	Reason      string `json:"reason"`
	Description string `json:"description,omitempty"`
	Initiator   string `json:"initiator,omitempty"`
}

// OTARequestPayload contains fields from the system.ota.request payload.
type OTARequestPayload struct {
	FirmwareURL string `json:"firmware_url"`
	Version     string `json:"version"`
	SHA256      string `json:"sha256"`
	Force       *bool  `json:"force,omitempty"`
}

// NewEnvelope creates a new envelope with a generated UUIDv4 and current UTC timestamp.
func NewEnvelope(source Source, msgType string) Envelope {
	return Envelope{
		ID:            uuid.New().String(),
		Timestamp:     time.Now().UTC().Unix(),
		Source:        source,
		SchemaVersion: SchemaVersion,
		Type:          msgType,
	}
}

// NewMessage builds a complete message with envelope and marshaled payload.
func NewMessage(source Source, msgType string, payload interface{}) (*Message, error) {
	env := NewEnvelope(source, msgType)

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	return &Message{
		Envelope: env,
		Payload:  json.RawMessage(payloadBytes),
	}, nil
}

// Parse unmarshals JSON bytes into a Message.
func Parse(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("parse message: %w", err)
	}
	return &msg, nil
}

// ParseHeartbeat extracts a HeartbeatPayload from a Message.
func ParseHeartbeat(msg *Message) (*HeartbeatPayload, error) {
	var p HeartbeatPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		return nil, fmt.Errorf("parse heartbeat payload: %w", err)
	}
	return &p, nil
}

// ParseCommandRequest extracts a CommandRequestPayload from a Message.
func ParseCommandRequest(msg *Message) (*CommandRequestPayload, error) {
	var p CommandRequestPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		return nil, fmt.Errorf("parse command request payload: %w", err)
	}
	return &p, nil
}

// ParseCommandResponse extracts a CommandResponsePayload from a Message.
func ParseCommandResponse(msg *Message) (*CommandResponsePayload, error) {
	var p CommandResponsePayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		return nil, fmt.Errorf("parse command response payload: %w", err)
	}
	return &p, nil
}

// ParseEmergencyStop extracts an EmergencyStopPayload from a Message.
func ParseEmergencyStop(msg *Message) (*EmergencyStopPayload, error) {
	var p EmergencyStopPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		return nil, fmt.Errorf("parse emergency stop payload: %w", err)
	}
	return &p, nil
}

// ParseOTARequest extracts an OTARequestPayload from a Message.
func ParseOTARequest(msg *Message) (*OTARequestPayload, error) {
	var p OTARequestPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		return nil, fmt.Errorf("parse OTA request payload: %w", err)
	}
	return &p, nil
}
