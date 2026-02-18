package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/holla2040/arturo/internal/protocol"
)

// DisplayMessage wraps a parsed protocol message with channel and display metadata.
type DisplayMessage struct {
	Timestamp time.Time
	Channel   string
	Direction string // "→" for outgoing/requests, "←" for incoming/responses
	Message   *protocol.Message
	StreamID  string
}

// FormatMessage formats a DisplayMessage for terminal output.
// Line 1: timestamp, channel, direction, message type.
// Line 2 (indented): correlation_id (first 8 chars) + type-specific fields.
func FormatMessage(dm *DisplayMessage) string {
	ts := dm.Timestamp.Format("15:04:05.000")
	line1 := fmt.Sprintf("%s  %s %s %s", ts, dm.Channel, dm.Direction, dm.Message.Envelope.Type)

	corrID := dm.Message.Envelope.CorrelationID
	if len(corrID) > 8 {
		corrID = corrID[:8]
	}

	var detail string
	switch dm.Message.Envelope.Type {
	case protocol.TypeDeviceCommandRequest:
		req, err := protocol.ParseCommandRequest(dm.Message)
		if err == nil {
			params := formatParams(req.Parameters)
			detail = fmt.Sprintf("corr=%s cmd=%s params=%s", corrID, req.CommandName, params)
		} else {
			detail = fmt.Sprintf("corr=%s (parse error)", corrID)
		}

	case protocol.TypeDeviceCommandResponse:
		resp, err := protocol.ParseCommandResponse(dm.Message)
		if err == nil {
			durationMs := 0
			if resp.DurationMs != nil {
				durationMs = *resp.DurationMs
			}
			detail = fmt.Sprintf("corr=%s success=%t duration=%dms", corrID, resp.Success, durationMs)
		} else {
			detail = fmt.Sprintf("corr=%s (parse error)", corrID)
		}

	case protocol.TypeServiceHeartbeat:
		hb, err := protocol.ParseHeartbeat(dm.Message)
		if err == nil {
			detail = fmt.Sprintf("status=%s heap=%d rssi=%d", hb.Status, hb.FreeHeap, hb.WifiRSSI)
			warnings := HealthWarnings(hb)
			if len(warnings) > 0 {
				detail += fmt.Sprintf(" [%s]", strings.Join(warnings, ", "))
			}
		} else {
			detail = "(parse error)"
		}

	case protocol.TypeSystemEmergencyStop:
		estop, err := protocol.ParseEmergencyStop(dm.Message)
		if err == nil {
			detail = fmt.Sprintf("reason=%s initiator=%s", estop.Reason, estop.Initiator)
		} else {
			detail = "(parse error)"
		}

	default:
		detail = fmt.Sprintf("corr=%s", corrID)
	}

	return fmt.Sprintf("%s\n    %s", line1, detail)
}

// formatParams formats a map as {k:v,...} for compact display.
func formatParams(params map[string]string) string {
	if len(params) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(params))
	for k, v := range params {
		parts = append(parts, fmt.Sprintf("%s:%s", k, v))
	}
	return fmt.Sprintf("{%s}", strings.Join(parts, ","))
}

// FormatPresence formats a presence key status for terminal display.
// Key format: "device:{instance}:alive"
func FormatPresence(key string, ttl int64) string {
	instance := key
	parts := strings.Split(key, ":")
	if len(parts) >= 3 && parts[0] == "device" && parts[len(parts)-1] == "alive" {
		instance = strings.Join(parts[1:len(parts)-1], ":")
	}

	status := "ONLINE"
	if ttl <= 0 {
		status = "OFFLINE"
	}

	return fmt.Sprintf("%-20s TTL=%-4d %s", instance, ttl, status)
}

// ParseStreamFields extracts a protocol.Message from Redis stream fields.
// The stream stores JSON as a single "data" field, or falls back to the first field value.
func ParseStreamFields(fields map[string]string) (*protocol.Message, error) {
	data, ok := fields["data"]
	if !ok {
		// Fall back to first field value
		for _, v := range fields {
			data = v
			break
		}
	}
	if data == "" {
		return nil, fmt.Errorf("no message data in stream fields")
	}

	var msg protocol.Message
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		return nil, fmt.Errorf("parse stream message: %w", err)
	}
	return &msg, nil
}

// HealthWarnings returns warning strings for concerning heartbeat values.
func HealthWarnings(hb *protocol.HeartbeatPayload) []string {
	var warnings []string

	if hb.FreeHeap < 20000 {
		warnings = append(warnings, "CRITICAL HEAP")
	} else if hb.FreeHeap < 50000 {
		warnings = append(warnings, "LOW HEAP")
	}

	if hb.WifiRSSI < -80 {
		warnings = append(warnings, "POOR WIFI")
	} else if hb.WifiRSSI < -70 {
		warnings = append(warnings, "WEAK WIFI")
	}

	if hb.CommandsFailed != nil && *hb.CommandsFailed > 0 {
		warnings = append(warnings, "FAILURES")
	}

	if hb.WatchdogResets != nil && *hb.WatchdogResets > 0 {
		warnings = append(warnings, "WATCHDOG RESETS")
	}

	return warnings
}
