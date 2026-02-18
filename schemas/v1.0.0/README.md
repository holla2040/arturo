# Arturo Protocol v1.0.0 Schemas

JSON Schema definitions for the Arturo messaging protocol. These schemas are the single source of truth for all messages exchanged between the Go server and ESP32 field devices over Redis.

## Message Types

| Type | Transport | Direction | Description |
|------|-----------|-----------|-------------|
| `device.command.request` | Redis Stream | Server -> ESP32 | Execute a command on a device |
| `device.command.response` | Redis Stream | ESP32 -> Server | Result of a device command |
| `service.heartbeat` | Redis Pub/Sub | ESP32 -> Server | Periodic health report |
| `system.emergency_stop` | Redis Pub/Sub | Any -> All | Emergency stop broadcast |
| `system.ota.request` | Redis Stream | Server -> ESP32 | Firmware update request |

## Shared Definitions

| Schema | Purpose |
|--------|---------|
| `envelope` | Common message wrapper with metadata, routing, and correlation |
| `error` | Standard error object used in response payloads |

## Directory Structure

```
v1.0.0/
├── README.md                          # This file
├── envelope/
│   └── schema-definition.md           # Message envelope (shared by all types)
├── error/
│   └── schema-definition.md           # Error object (used in responses)
├── device-command-request/
│   ├── schema-definition.md           # Command request schema
│   └── examples/
│       ├── measure_voltage.json       # SCPI measurement command
│       └── set_relay.json             # Relay control with parameters
├── device-command-response/
│   ├── schema-definition.md           # Command response schema
│   └── examples/
│       ├── success.json               # Successful measurement
│       └── error_timeout.json         # Device timeout error
├── service-heartbeat/
│   ├── schema-definition.md           # Heartbeat schema
│   └── examples/
│       ├── healthy.json               # Normal operation
│       └── degraded.json              # Device with errors
├── system-emergency-stop/
│   ├── schema-definition.md           # E-stop schema
│   └── examples/
│       ├── button_press.json          # Physical button press
│       └── operator_command.json      # Operator-initiated stop
└── system-ota-request/
    ├── schema-definition.md           # OTA update schema
    └── examples/
        ├── standard_update.json       # Normal version upgrade
        └── forced_update.json         # Forced rollback/update
```

## Validation

All messages must validate against their respective JSON Schema before being sent. Both the Go server and ESP32 firmware validate incoming messages.

## Version History

### v1.0.0 (Current)
- Initial schema release
- Five message types: command request/response, heartbeat, emergency stop, OTA
- Common envelope with UUIDv4 IDs, RFC3339 timestamps, correlation tracking
