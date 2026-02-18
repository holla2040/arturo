# Device Command Response Schema v1.0.0

## Overview

| Property | Value |
|----------|-------|
| Version | v1.0.0 |
| Format | JSON |
| Message Type | `device.command.response` |
| Transport | Redis Stream |
| Channel | `responses:{service}:{instance-id}` (from request's `reply_to`) |
| Direction | Station -> Controller |
| Status | Active |

Response from a device command execution. The station publishes this to the Redis Stream specified in the request's `reply_to` field. Covers both success and error cases in a single message type using the `success` boolean.

## JSON Schema Definition

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://github.com/holla2040/arturo/schemas/v1.0.0/device-command-response.json",
  "title": "Device Command Response",
  "description": "Response from a device command execution. Sent by a station to the controller via Redis Stream.",
  "type": "object",
  "required": ["envelope", "payload"],
  "additionalProperties": false,
  "properties": {
    "envelope": {
      "$ref": "../envelope/schema-definition.md#envelope",
      "properties": {
        "type": { "const": "device.command.response" }
      },
      "required": ["id", "timestamp", "source", "schema_version", "type", "correlation_id"]
    },
    "payload": {
      "type": "object",
      "required": ["device_id", "command_name", "success"],
      "additionalProperties": false,
      "properties": {
        "device_id": {
          "type": "string",
          "description": "Device that executed the command.",
          "pattern": "^[a-zA-Z0-9][a-zA-Z0-9_-]*$",
          "minLength": 1,
          "maxLength": 64
        },
        "command_name": {
          "type": "string",
          "description": "The command that was executed (echoed from request).",
          "minLength": 1,
          "maxLength": 256
        },
        "success": {
          "type": "boolean",
          "description": "Whether the command executed successfully."
        },
        "response": {
          "type": ["string", "null"],
          "description": "Device response data. Null if no output or failed.",
          "maxLength": 4096
        },
        "error": {
          "description": "Error information. Present only when success is false.",
          "$ref": "../error/schema-definition.md#error"
        },
        "duration_ms": {
          "type": "integer",
          "description": "Time from command send to response received, in milliseconds.",
          "minimum": 0
        }
      },
      "if": {
        "properties": { "success": { "const": false } }
      },
      "then": {
        "required": ["device_id", "command_name", "success", "error"]
      }
    }
  }
}
```

## Field Descriptions

### Envelope Fields (Required for this type)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `correlation_id` | string | Yes | Echoed from the original request. Controller uses this to match response to request. |
| `reply_to` | -- | Not used | Response is published to the stream specified in the request's `reply_to`, but the response itself does not include a `reply_to` field. |

### Payload Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `device_id` | string | Yes | Device that executed the command. Echoed from request. |
| `command_name` | string | Yes | Command that was executed. Echoed from request. |
| `success` | boolean | Yes | `true` if command completed without error, `false` otherwise. |
| `response` | string or null | No | Raw device response string. `null` if the command produced no output or failed. Max 4096 characters. |
| `error` | object | Conditional | Error details. **Required when `success` is `false`**. Uses the standard error object schema. |
| `duration_ms` | integer | No | Wall-clock time from sending the command to receiving the device response, in milliseconds. |

## Conditional Validation

The `error` field is required when `success` is `false`:

| `success` | `response` | `error` | Valid? |
|-----------|-----------|---------|--------|
| `true` | `"1.23456789"` | absent | Yes |
| `true` | `null` | absent | Yes (command with no output, e.g., relay set) |
| `false` | `null` | present | Yes |
| `false` | `null` | absent | **No** -- error is required when success is false |

## Response Data Types

The `response` field is always a string (the raw device output). The controller is responsible for parsing it into the appropriate type based on the command profile.

| Device Response | `response` Value | Controller Parses As |
|----------------|-----------------|------------------|
| DC voltage | `"1.23456789"` | float64 |
| Relay state | `"ON"` | boolean |
| Device ID | `"FLUKE,8846A,12345,1.0"` | string (parsed by profile) |
| No output | `null` | -- |

## Implementation Details

### Station Firmware (C++)

```cpp
// Build success response
void buildSuccessResponse(JsonDocument& doc, const DeviceCommand& cmd, const char* response, uint32_t durationMs) {
    JsonObject envelope = doc.createNestedObject("envelope");
    envelope["id"] = generateUUID();
    envelope["timestamp"] = getISO8601Timestamp();
    JsonObject source = envelope.createNestedObject("source");
    source["service"] = SERVICE_NAME;
    source["instance"] = INSTANCE_ID;
    source["version"] = FIRMWARE_VERSION;
    envelope["schema_version"] = "v1.0.0";
    envelope["type"] = "device.command.response";
    envelope["correlation_id"] = cmd.correlation_id;

    JsonObject payload = doc.createNestedObject("payload");
    payload["device_id"] = cmd.device_id;
    payload["command_name"] = cmd.command_name;
    payload["success"] = true;
    payload["response"] = response;
    payload["duration_ms"] = durationMs;
}

// Build error response
void buildErrorResponse(JsonDocument& doc, const DeviceCommand& cmd, const char* errCode, const char* errMsg, uint32_t durationMs) {
    // ... envelope same as above ...
    JsonObject payload = doc.createNestedObject("payload");
    payload["device_id"] = cmd.device_id;
    payload["command_name"] = cmd.command_name;
    payload["success"] = false;
    payload["response"] = nullptr;
    JsonObject error = payload.createNestedObject("error");
    error["code"] = errCode;
    error["message"] = errMsg;
    payload["duration_ms"] = durationMs;
}
```

### Controller (Go)

```go
// Wait for response by correlation ID
func (c *Controller) WaitForResponse(correlationID string, timeout time.Duration) (*CommandResponse, error) {
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()

    for {
        results, err := c.redis.XRead(ctx, &redis.XReadArgs{
            Streams: []string{c.responseStream, c.lastResponseID},
            Block:   timeout,
            Count:   1,
        }).Result()
        if err != nil { return nil, err }

        for _, msg := range results[0].Messages {
            var resp CommandResponse
            json.Unmarshal([]byte(msg.Values["message"].(string)), &resp)
            if resp.Envelope.CorrelationID == correlationID {
                return &resp, nil
            }
        }
    }
}
```

## Version History

### v1.0.0 (Current)
- Initial command response definition
- Unified success/error in single message type
- Conditional `error` field required when `success` is false
- String-only response data (controller parses by profile)
- Duration tracking in milliseconds
