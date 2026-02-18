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

// extractInstance extracts the station instance name from a Redis key.
// Key format: "device:{instance}:alive" or "commands:{instance}" or "responses:{instance}"
func extractInstance(key string) string {
	parts := strings.Split(key, ":")
	if len(parts) >= 3 && parts[0] == "device" && parts[len(parts)-1] == "alive" {
		return strings.Join(parts[1:len(parts)-1], ":")
	}
	if len(parts) >= 2 && (parts[0] == "commands" || parts[0] == "responses") {
		return strings.Join(parts[1:], ":")
	}
	return key
}

// FormatMessage formats a DisplayMessage for terminal output with ANSI color coding.
// Format: [tag]  instance  detail fields
func FormatMessage(dm *DisplayMessage) string {
	instance := dm.Message.Envelope.Source.Instance

	corrID := dm.Message.Envelope.CorrelationID
	if len(corrID) > 8 {
		corrID = corrID[:8]
	}

	var tag, detail, color string
	switch dm.Message.Envelope.Type {
	case protocol.TypeDeviceCommandRequest:
		tag = "command"
		req, err := protocol.ParseCommandRequest(dm.Message)
		if err == nil {
			params := formatParams(req.Parameters)
			detail = fmt.Sprintf("corr=%s  cmd=%s  params=%s", corrID, req.CommandName, params)
		} else {
			detail = fmt.Sprintf("corr=%s  (parse error)", corrID)
		}

	case protocol.TypeDeviceCommandResponse:
		tag = "response"
		resp, err := protocol.ParseCommandResponse(dm.Message)
		if err == nil {
			durationMs := 0
			if resp.DurationMs != nil {
				durationMs = *resp.DurationMs
			}
			detail = fmt.Sprintf("corr=%s  success=%t  duration=%dms", corrID, resp.Success, durationMs)
			if resp.Success {
				color = colorGreen
			} else {
				color = colorRed
			}
		} else {
			detail = fmt.Sprintf("corr=%s  (parse error)", corrID)
		}

	case protocol.TypeServiceHeartbeat:
		tag = "heartbeat"
		color = colorCyan
		hb, err := protocol.ParseHeartbeat(dm.Message)
		if err == nil {
			detail = fmt.Sprintf("%s  heap=%d  rssi=%d", hb.Status, hb.FreeHeap, hb.WifiRSSI)
			warnings := HealthWarnings(hb)
			if len(warnings) > 0 {
				detail += fmt.Sprintf("  %s[%s]%s", colorYellow, strings.Join(warnings, ", "), colorCyan)
			}
		} else {
			detail = "(parse error)"
		}

	case protocol.TypeSystemEmergencyStop:
		tag = "estop"
		color = colorRed
		estop, err := protocol.ParseEmergencyStop(dm.Message)
		if err == nil {
			detail = fmt.Sprintf("reason=%s  initiator=%s", estop.Reason, estop.Initiator)
		} else {
			detail = "(parse error)"
		}

	default:
		tag = "message"
		detail = fmt.Sprintf("corr=%s", corrID)
	}

	body := fmt.Sprintf("[%-9s]  %-12s  %s", tag, instance, detail)
	if color != "" {
		return color + body + colorReset
	}
	return body
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

// FormatPresence formats a presence key status for terminal display with color coding.
// Key format: "device:{instance}:alive"
func FormatPresence(key string, ttl int64, state StationState, lastSeen time.Time) string {
	instance := extractInstance(key)

	var color string
	switch state {
	case StateOnline:
		color = colorGreen
	case StateStale:
		color = colorYellow
	case StateOffline:
		color = colorRed
	}

	detail := fmt.Sprintf("TTL=%-4d  %s", ttl, state.String())

	if state == StateStale || state == StateOffline {
		if !lastSeen.IsZero() {
			ago := time.Since(lastSeen).Round(time.Second)
			detail += fmt.Sprintf("  (last seen %s ago)", ago)
		}
	}

	line := fmt.Sprintf("[%-9s]  %-12s  %s", "presence", instance, detail)
	return color + line + colorReset
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
