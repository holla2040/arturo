# System Emergency Stop Schema v1.0.0

## Overview

| Property | Value |
|----------|-------|
| Version | v1.0.0 |
| Format | JSON |
| Message Type | `system.emergency_stop` |
| Transport | Redis Pub/Sub |
| Channel | `events:emergency_stop` |
| Direction | Any -> All (broadcast) |
| Status | Active |

Emergency stop broadcast. Any node (ESP32 or server) can publish this message. All nodes subscribe to the E-stop channel and must immediately enter a safe state when received.

This is the highest-priority message in the system. ESP32 nodes check for E-stop on a dedicated high-priority FreeRTOS task (watchdogTask, priority 3).

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Transport | Pub/Sub (not Stream) | E-stop must reach all subscribers immediately. At-most-once delivery is acceptable -- the safe state is fail-safe. |
| Acknowledgment | None | Fire-and-forget. Nodes enter safe state locally regardless of acknowledgment. |
| Local action | Immediate | ESP32 cuts relay power via GPIO before responding to Redis. Physical safety first. |
| Duplicate handling | Idempotent | Receiving multiple E-stops is harmless. Nodes stay in safe state until explicitly cleared. |

## JSON Schema Definition

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://github.com/holla2040/arturo/schemas/v1.0.0/system-emergency-stop.json",
  "title": "System Emergency Stop",
  "description": "Emergency stop broadcast. All nodes must immediately enter safe state.",
  "type": "object",
  "required": ["envelope", "payload"],
  "additionalProperties": false,
  "properties": {
    "envelope": {
      "$ref": "../envelope/schema-definition.md#envelope",
      "properties": {
        "type": { "const": "system.emergency_stop" }
      },
      "required": ["id", "timestamp", "source", "schema_version", "type"]
    },
    "payload": {
      "type": "object",
      "required": ["reason"],
      "additionalProperties": false,
      "properties": {
        "reason": {
          "type": "string",
          "description": "Why the emergency stop was triggered.",
          "enum": [
            "button_press",
            "operator_command",
            "safety_interlock",
            "device_fault",
            "software_error"
          ]
        },
        "description": {
          "type": "string",
          "description": "Human-readable description of what triggered the E-stop.",
          "maxLength": 256
        },
        "initiator": {
          "type": "string",
          "description": "Instance ID of the node or operator that triggered the E-stop.",
          "maxLength": 64
        }
      }
    }
  }
}
```

## Field Descriptions

### Envelope Fields

No `correlation_id` or `reply_to` -- E-stop is fire-and-forget broadcast.

### Payload Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `reason` | string | Yes | Machine-readable reason code for the E-stop. |
| `description` | string | No | Human-readable description for logging and display. |
| `initiator` | string | No | Instance ID or operator name that triggered the stop. |

## Reason Codes

| Reason | Trigger | Typical Source |
|--------|---------|---------------|
| `button_press` | Physical E-stop button pressed on an ESP32 node | ESP32 E-stop node |
| `operator_command` | Operator manually triggered E-stop via UI or CLI | Go server |
| `safety_interlock` | Automated safety check failed (temperature, voltage, etc.) | ESP32 or server |
| `device_fault` | Device reported a critical error | ESP32 bridge node |
| `software_error` | Software detected an unrecoverable state | Go server |

## E-Stop Response Behavior

### ESP32 Nodes

When an ESP32 receives `system.emergency_stop`:

1. **Immediately** set all relay GPIOs to safe state (OFF)
2. Set status LED to rapid blink (error indicator)
3. Stop processing command queue (drain without executing)
4. Set node status to `"degraded"` in heartbeat
5. Continue publishing heartbeats (so server knows node is alive)
6. Reject all new commands with `E_COMMAND_FAILED` ("E-stop active")
7. Wait for explicit clear command before resuming

### Go Server

When the server receives or sends `system.emergency_stop`:

1. Log the E-stop event with full context
2. Stop sending new commands to all ESP32 nodes
3. Cancel pending command timeouts
4. Notify connected web clients
5. Set system state to "emergency stopped"
6. Require operator acknowledgment before resuming

### Local E-Stop (ESP32 Button)

The ESP32 E-stop node has a physical button wired to a GPIO interrupt:

1. GPIO interrupt fires (debounced, 50ms)
2. **Immediately** cut local relay power (before Redis)
3. Publish `system.emergency_stop` to Redis
4. Other ESP32 nodes receive and enter safe state

The local GPIO action happens before the Redis publish. Physical safety does not depend on network availability.

## Implementation Details

### ESP32 Firmware (C++)

```cpp
// E-stop subscription handler (runs in watchdogTask, priority 3)
void onEmergencyStop(const char* json) {
    // 1. Immediate hardware safe state
    setAllRelaysSafe();
    setStatusLED(LED_ESTOP);

    // 2. Parse for logging
    StaticJsonDocument<256> doc;
    deserializeJson(doc, json);
    Serial.printf("[ESTOP] reason=%s initiator=%s\n",
        doc["payload"]["reason"].as<const char*>(),
        doc["payload"]["initiator"].as<const char*>());

    // 3. Set global flag (checked by commandTask before executing)
    estopActive = true;
}

// E-stop button ISR (GPIO interrupt)
void IRAM_ATTR estopButtonISR() {
    // Debounce
    if (millis() - lastEstopPress < 50) return;
    lastEstopPress = millis();

    // Immediate local action
    setAllRelaysSafe();

    // Signal task to publish Redis message
    BaseType_t xHigherPriorityTaskWoken = pdFALSE;
    xTaskNotifyFromISR(watchdogTaskHandle, ESTOP_NOTIFY, eSetBits, &xHigherPriorityTaskWoken);
    portYIELD_FROM_ISR(xHigherPriorityTaskWoken);
}
```

### Go Server

```go
// Trigger E-stop from server
func (s *Server) EmergencyStop(reason, description string) {
    msg := map[string]interface{}{
        "envelope": map[string]interface{}{
            "id":             uuid.New().String(),
            "timestamp":      time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
            "source":         s.source,
            "schema_version": "v1.0.0",
            "type":           "system.emergency_stop",
        },
        "payload": map[string]interface{}{
            "reason":      reason,
            "description": description,
            "initiator":   s.instanceID,
        },
    }
    s.redis.Publish(ctx, "events:emergency_stop", marshal(msg))
    s.estopActive = true
}
```

## Version History

### v1.0.0 (Current)
- Initial emergency stop definition
- Five reason codes: button, operator, interlock, device fault, software
- Fire-and-forget Pub/Sub broadcast
- Hardware-first safety: local GPIO before network
