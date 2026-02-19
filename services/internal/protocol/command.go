package protocol

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// BuildCommandRequest creates a device.command.request message ready to send to a station.
// It generates a correlation_id and sets reply_to to "responses:{source.Instance}".
func BuildCommandRequest(source Source, deviceID, cmdName string, params map[string]string, timeoutMs int) (*Message, error) {
	env := NewEnvelope(source, TypeDeviceCommandRequest)
	env.CorrelationID = uuid.New().String()
	env.ReplyTo = "responses:" + source.Instance

	payload := CommandRequestPayload{
		DeviceID:    deviceID,
		CommandName: cmdName,
		Parameters:  params,
		TimeoutMs:   &timeoutMs,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal command request payload: %w", err)
	}

	return &Message{
		Envelope: env,
		Payload:  json.RawMessage(payloadBytes),
	}, nil
}

// BuildOTARequest creates a system.ota.request message ready to send to a station.
// It generates a correlation_id and sets reply_to to "responses:{source.Instance}".
func BuildOTARequest(source Source, firmwareURL, version, sha256 string, force bool) (*Message, error) {
	env := NewEnvelope(source, TypeSystemOTARequest)
	env.CorrelationID = uuid.New().String()
	env.ReplyTo = "responses:" + source.Instance

	payload := OTARequestPayload{
		FirmwareURL: firmwareURL,
		Version:     version,
		SHA256:      sha256,
		Force:       &force,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal OTA request payload: %w", err)
	}

	return &Message{
		Envelope: env,
		Payload:  json.RawMessage(payloadBytes),
	}, nil
}
