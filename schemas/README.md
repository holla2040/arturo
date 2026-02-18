# Message Schemas

JSON Schema definitions for Arturo Protocol v2 messages.

Phase 0 deliverable: define these four schemas before writing any code.

## Message Types

1. `device.command.request.v1.json` - Server sends command to ESP32
2. `device.command.response.v1.json` - ESP32 returns result to server
3. `service.heartbeat.v1.json` - ESP32 periodic health report
4. `system.emergency_stop.v1.json` - Emergency stop broadcast

## Envelope

All messages share a common envelope defined in `envelope.v1.json`.
