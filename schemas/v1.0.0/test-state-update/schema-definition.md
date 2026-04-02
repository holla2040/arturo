# Test State Update Schema v1.0.0

## Overview

| Property | Value |
|----------|-------|
| Version | v1.0.0 |
| Format | JSON |
| Message Type | `test.state.update` |
| Transport | Redis Pub/Sub |
| Channel | `commands:{station-instance}` (shared with command requests) |
| Direction | Controller -> Station |
| Status | Active |

Notifies a station that the test state has changed. The station uses this to update its LCD display (test name, elapsed time, status bar color) and to lock out manual controls while a test is running.

This is a fire-and-forget notification -- no response is expected. Neither `correlation_id` nor `reply_to` is required.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Transport | Redis Pub/Sub on `commands:` channel | Station already subscribes to this channel for commands. No additional subscription needed. |
| Response | None (fire-and-forget) | Display update only. No acknowledgment needed. |
| Frequency | On state transitions only | Sent when test starts, pauses, resumes, completes, or aborts. Not periodic. |

## JSON Schema Definition

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://github.com/holla2040/arturo/schemas/v1.0.0/test-state-update.json",
  "title": "Test State Update",
  "description": "Notification of test state change for station display and control lockout.",
  "type": "object",
  "required": ["envelope", "payload"],
  "additionalProperties": false,
  "properties": {
    "envelope": {
      "$ref": "../envelope/schema-definition.md#envelope",
      "properties": {
        "type": { "const": "test.state.update" }
      },
      "required": ["id", "timestamp", "source", "schema_version", "type"]
    },
    "payload": {
      "type": "object",
      "required": ["state", "test_id", "test_name", "elapsed_seconds"],
      "additionalProperties": false,
      "properties": {
        "state": {
          "type": "string",
          "description": "Current test state.",
          "enum": ["running", "paused", "completed", "aborted"]
        },
        "test_id": {
          "type": "string",
          "description": "Unique identifier for the test run."
        },
        "test_name": {
          "type": "string",
          "description": "Human-readable test name shown on the station display."
        },
        "elapsed_seconds": {
          "type": "integer",
          "description": "Seconds elapsed since the test started.",
          "minimum": 0
        }
      }
    }
  }
}
```

## Field Descriptions

### Envelope Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `correlation_id` | string | No | Not used. Fire-and-forget notification. |
| `reply_to` | string | No | Not used. No response expected. |

### Payload Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `state` | string | Yes | Current test state: `running`, `paused`, `completed`, or `aborted`. |
| `test_id` | string | Yes | Unique test run identifier (matches the test run record in the controller database). |
| `test_name` | string | Yes | Display name for the test. Shown on the station LCD and terminal UI. |
| `elapsed_seconds` | integer | Yes | Seconds since the test started. Used for the elapsed time display on the station LCD. |

## Station Display Behavior

The station firmware updates its LCD based on the `state` field:

| State | Display |
|-------|---------|
| `running` | Amber bar with test name and elapsed time (HH:MM:SS) |
| `paused` | Yellow bar with "PAUSED: " + test name |
| `completed` | Returns to gray bar showing "No active test" |
| `aborted` | Returns to gray bar showing "No active test" |

## Version History

### v1.0.0 (Current)
- Initial test state update definition
- Four states: running, paused, completed, aborted
- Fire-and-forget delivery via Pub/Sub on station command channel
