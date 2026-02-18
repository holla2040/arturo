# Arturo Error Object v1.0.0

## Overview

| Property | Value |
|----------|-------|
| Version | v1.0.0 |
| Format | JSON |
| Status | Active |

Standard error object embedded in response payloads when a command fails. This is not a standalone message type -- it is referenced by `device.command.response` when `success` is `false`.

## JSON Schema Definition

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://github.com/holla2040/arturo/schemas/v1.0.0/error.json",
  "title": "Arturo Error Object",
  "description": "Standard error object used in response payloads when a command fails.",
  "type": "object",
  "required": ["code", "message"],
  "additionalProperties": false,
  "properties": {
    "code": {
      "type": "string",
      "description": "Machine-readable error code.",
      "pattern": "^E_[A-Z_]+$",
      "enum": [
        "E_DEVICE_TIMEOUT",
        "E_DEVICE_NOT_FOUND",
        "E_DEVICE_NOT_CONNECTED",
        "E_DEVICE_ERROR",
        "E_COMMAND_FAILED",
        "E_VALIDATION_FAILED",
        "E_INVALID_PARAMETER",
        "E_INTERNAL"
      ]
    },
    "message": {
      "type": "string",
      "description": "Human-readable error description.",
      "minLength": 1,
      "maxLength": 512
    },
    "details": {
      "type": "object",
      "description": "Additional context about the error. Free-form key-value pairs.",
      "additionalProperties": true
    }
  }
}
```

## Field Descriptions

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `code` | string | Yes | Machine-readable error code. Always prefixed with `E_`. Used for programmatic error handling. |
| `message` | string | Yes | Human-readable error description. Suitable for logging and display. Max 512 characters. |
| `details` | object | No | Additional context as free-form key-value pairs. Content varies by error type. |

## Error Codes

| Code | Category | Description |
|------|----------|-------------|
| `E_DEVICE_TIMEOUT` | Device | Device did not respond within the specified `timeout_ms`. |
| `E_DEVICE_NOT_FOUND` | Device | Target `device_id` does not match any connected device. |
| `E_DEVICE_NOT_CONNECTED` | Device | Device is known but currently disconnected (cable, power, etc.). |
| `E_DEVICE_ERROR` | Device | Device returned an error response (e.g., SCPI error, Modbus exception). |
| `E_COMMAND_FAILED` | Command | Command execution failed for a reason other than device errors. |
| `E_VALIDATION_FAILED` | Validation | Message failed schema validation before execution. |
| `E_INVALID_PARAMETER` | Validation | A command parameter value is out of range or wrong type. |
| `E_INTERNAL` | System | Unexpected internal error (bug, memory, etc.). |

## Details Field by Error Code

The `details` object carries error-specific context:

| Error Code | Common Details Fields | Example |
|------------|----------------------|---------|
| `E_DEVICE_TIMEOUT` | `timeout_ms` | `{"timeout_ms": 5000}` |
| `E_DEVICE_NOT_FOUND` | `device_id`, `known_devices` | `{"device_id": "fluke-8846a", "known_devices": ["relay-8ch"]}` |
| `E_DEVICE_ERROR` | `device_error`, `scpi_error_code` | `{"device_error": "-100,\"Command error\""}` |
| `E_VALIDATION_FAILED` | `field`, `reason` | `{"field": "payload.command_name", "reason": "empty string"}` |
| `E_INVALID_PARAMETER` | `parameter`, `value`, `expected` | `{"parameter": "channel", "value": "9", "expected": "1-8"}` |

## Example Usage

### Timeout Error

```json
{
  "code": "E_DEVICE_TIMEOUT",
  "message": "Device did not respond within 5000ms",
  "details": {
    "timeout_ms": 5000
  }
}
```

### Device Not Connected

```json
{
  "code": "E_DEVICE_NOT_CONNECTED",
  "message": "fluke-8846a is not connected",
  "details": {
    "device_id": "fluke-8846a",
    "last_seen": "2026-02-17T11:55:00.000Z"
  }
}
```

### Invalid Parameter

```json
{
  "code": "E_INVALID_PARAMETER",
  "message": "Relay channel 9 out of range",
  "details": {
    "parameter": "channel",
    "value": "9",
    "expected": "1-8"
  }
}
```

## Implementation Details

### ESP32 Firmware (C++)

```cpp
struct ArturoError {
    const char* code;
    char message[512];
    JsonObject details;  // ArduinoJson object
};

// Error creation helper
void buildError(JsonObject& error, const char* code, const char* message) {
    error["code"] = code;
    error["message"] = message;
}

// Usage in command handler
if (millis() - startTime > cmd.timeout_ms) {
    JsonObject error = payload.createNestedObject("error");
    buildError(error, "E_DEVICE_TIMEOUT", "Device did not respond within timeout");
    error.createNestedObject("details")["timeout_ms"] = cmd.timeout_ms;
}
```

### Go Server

```go
type ArturoError struct {
    Code    string                 `json:"code"`
    Message string                 `json:"message"`
    Details map[string]interface{} `json:"details,omitempty"`
}
```

## Adding New Error Codes

1. Add the new code to the `enum` list in this schema
2. Follow the naming convention: `E_` prefix, uppercase, underscores
3. Document the code, category, and description in the Error Codes table
4. Document expected `details` fields
5. Update both Go and ESP32 implementations

## Version History

### v1.0.0 (Current)
- Initial error object definition
- Eight error codes across device, command, validation, and system categories
- Optional `details` field for error-specific context
