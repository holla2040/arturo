# Device Command Request Schema v1.0.0

## Overview

| Property | Value |
|----------|-------|
| Version | v1.0.0 |
| Format | JSON |
| Message Type | `device.command.request` |
| Transport | Redis Stream |
| Channel | `commands:{instance-id}` (per-station stream) |
| Direction | Controller -> Station |
| Status | Active |

Request to execute a command on a physical device. The controller publishes this message to a station-specific Redis Stream using XADD. The target station reads it with XREAD BLOCK and dispatches the command to the hardware.

Commands can be either profile command names (e.g., `measure_dc_voltage`) that map to device-specific SCPI/Modbus/serial sequences, or raw device commands (e.g., `MEAS:VOLT:DC?`) sent directly to the instrument.

## JSON Schema Definition

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://github.com/holla2040/arturo/schemas/v1.0.0/device-command-request.json",
  "title": "Device Command Request",
  "description": "Request to execute a command on a device. Sent by the controller to a station via Redis Stream.",
  "type": "object",
  "required": ["envelope", "payload"],
  "additionalProperties": false,
  "properties": {
    "envelope": {
      "$ref": "../envelope/schema-definition.md#envelope",
      "properties": {
        "type": { "const": "device.command.request" }
      },
      "required": ["id", "timestamp", "source", "schema_version", "type", "correlation_id", "reply_to"]
    },
    "payload": {
      "type": "object",
      "required": ["device_id", "command_name"],
      "additionalProperties": false,
      "properties": {
        "device_id": {
          "type": "string",
          "description": "Target device identifier.",
          "pattern": "^[a-zA-Z0-9][a-zA-Z0-9_-]*$",
          "minLength": 1,
          "maxLength": 64
        },
        "command_name": {
          "type": "string",
          "description": "Command to execute. Profile name or raw device command.",
          "minLength": 1,
          "maxLength": 256
        },
        "parameters": {
          "type": "object",
          "description": "Command parameters as key-value string pairs.",
          "additionalProperties": { "type": "string" },
          "default": {}
        },
        "timeout_ms": {
          "type": "integer",
          "description": "Command timeout in milliseconds.",
          "minimum": 100,
          "maximum": 300000,
          "default": 5000
        }
      }
    }
  }
}
```

## Field Descriptions

### Envelope Fields (Required for this type)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `correlation_id` | string | Yes | UUIDv4 linking this request to its response. Controller generates this. |
| `reply_to` | string | Yes | Redis Stream where the station should publish the response (e.g., `responses:controller:ctrl-01`). |

### Payload Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `device_id` | string | Yes | -- | Target device identifier. Must match a device connected to the station (e.g., `fluke-8846a`, `relay-8ch`). |
| `command_name` | string | Yes | -- | Profile command name (e.g., `measure_dc_voltage`) or raw command (e.g., `MEAS:VOLT:DC?`). |
| `parameters` | object | No | `{}` | Key-value string pairs for parameterized commands (e.g., `{"channel": "3", "state": "on"}`). |
| `timeout_ms` | integer | No | `5000` | How long the station should wait for a device response. Returns `E_DEVICE_TIMEOUT` if exceeded. Range: 100-300000ms. |

## Command Types

### Profile Commands

Profile commands map to device-specific sequences defined in YAML profile files. The station looks up the command in the loaded device profile and translates it to the appropriate protocol.

| Command Name | Device | Protocol | What it does |
|-------------|--------|----------|-------------|
| `measure_dc_voltage` | Fluke 8846A | SCPI | Sends `MEAS:VOLT:DC?`, parses response |
| `measure_ac_voltage` | Fluke 8846A | SCPI | Sends `MEAS:VOLT:AC?`, parses response |
| `set_relay` | Relay Board | GPIO | Sets relay channel on/off |
| `read_temperature` | Omega CN7500 | Modbus | Reads temperature register |

### Raw Commands

If `command_name` doesn't match a profile command, it is sent directly to the device as a raw string. This is useful for debugging and ad-hoc queries.

```json
{
  "command_name": "MEAS:VOLT:DC?",
  "parameters": {}
}
```

## Redis Stream Usage

```
Controller:  XADD commands:dmm-station-01 * message <json>
Station:     XREAD BLOCK 0 STREAMS commands:dmm-station-01 $last_id
```

The controller publishes to the station-specific stream. Each station reads only from its own stream. After processing, the station publishes the response to the `reply_to` stream.

## Implementation Details

### Controller (Go)

```go
// Build and send command request
func (c *Controller) SendCommand(stationID, deviceID, commandName string, params map[string]string, timeoutMs int) (string, error) {
    correlationID := uuid.New().String()
    msg := map[string]interface{}{
        "envelope": map[string]interface{}{
            "id":             uuid.New().String(),
            "timestamp":      time.Now().Unix(),
            "source":         c.source,
            "schema_version": "v1.0.0",
            "type":           "device.command.request",
            "correlation_id": correlationID,
            "reply_to":       fmt.Sprintf("responses:controller:%s", c.instanceID),
        },
        "payload": map[string]interface{}{
            "device_id":    deviceID,
            "command_name": commandName,
            "parameters":   params,
            "timeout_ms":   timeoutMs,
        },
    }
    streamKey := fmt.Sprintf("commands:%s", stationID)
    c.redis.XAdd(ctx, &redis.XAddArgs{Stream: streamKey, Values: map[string]interface{}{"message": marshal(msg)}})
    return correlationID, nil
}
```

### Station Firmware (C++)

```cpp
// Parse incoming command from Redis Stream
bool parseCommandRequest(const char* json, DeviceCommand& cmd) {
    StaticJsonDocument<1024> doc;
    if (deserializeJson(doc, json) != DeserializationError::Ok) return false;

    strlcpy(cmd.device_id, doc["payload"]["device_id"], sizeof(cmd.device_id));
    strlcpy(cmd.command_name, doc["payload"]["command_name"], sizeof(cmd.command_name));
    cmd.timeout_ms = doc["payload"]["timeout_ms"] | 5000;
    strlcpy(cmd.correlation_id, doc["envelope"]["correlation_id"], sizeof(cmd.correlation_id));
    strlcpy(cmd.reply_to, doc["envelope"]["reply_to"], sizeof(cmd.reply_to));
    return true;
}
```

## Version History

### v1.0.0 (Current)
- Initial command request definition
- Profile commands and raw command support
- Per-station Redis Stream channels
- Configurable timeout with 5-second default
- String-only parameter values
