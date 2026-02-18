# CLAUDE.md

## Project Overview

Arturo is an industrial test automation system with ESP32 stations and a centralized Go controller, connected via Redis on a LAN.

**Terminology:** Stations (ESP32 + instruments), Terminal (operator UI), Controller (Go processes), File server (report storage).

- **Station firmware**: C++ with Arduino in `firmware/`
- **Controller**: `server/` with two main binaries (arturo-server, arturo-engine) and one debug tool (arturo-monitor)
- **Redis**: Streams for commands/responses, Pub/Sub for heartbeats/E-stop
- **Profiles**: Device YAML profiles in `profiles/`
- **Schemas**: JSON Schema message definitions in `schemas/`

## Architecture

- Up to 6 stations connect directly to Redis over WiFi/Ethernet
- One Ubuntu machine runs the controller (Go processes) + Redis + terminal (operator UI)
- All messages use Protocol v1.0.0 envelope format (see docs/architecture/MIGRATION_PLAN.md section 2.1)
- 5 message types: `device.command.request`, `device.command.response`, `service.heartbeat`, `system.emergency_stop`, `system.ota.request`

## Redis Channel Conventions

- `commands:{station-instance}` - Stream, controller -> specific station
- `responses:{requester-instance}` - Stream, station -> controller
- `events:heartbeat` - Pub/Sub, station -> controller
- `events:emergency_stop` - Pub/Sub + Stream, any -> all
- `device:{instance}:alive` - Key with 90s TTL for presence

## Build Commands

### Controller (Go)
```bash
cd server && go build -o arturo-server ./cmd/arturo-server
cd server && go build -o arturo-engine ./cmd/arturo-engine
cd server && go build -o arturo-monitor ./cmd/arturo-monitor
```

### Station Firmware (ESP32)
```bash
arduino-cli compile --fqbn esp32:esp32:esp32s3 firmware/
arduino-cli upload --fqbn esp32:esp32:esp32s3 --port /dev/ttyUSB0 firmware/
arduino-cli monitor --port /dev/ttyUSB0 --config baudrate=115200
```

## Development Guidelines

- Keep service count low. If it can be a function call inside arturo-server, it is not a separate service.
- Every message must use the Protocol v1.0.0 envelope format.
- Station firmware uses ArduinoJson v7 with static allocation.
- Debug output on stations goes to USB serial, controlled by DEBUG_LEVEL in config.h.
- Use `arturo-monitor` to observe all Redis traffic during development.
- Reference material from the original project is in `docs/reference/` and the [arturo-go-archive](https://github.com/holla2040/arturo-go-archive) repo.

## Key Files

- `docs/architecture/MIGRATION_PLAN.md` - Full build plan, architecture, phasing, debugging setup
- `docs/reference/PROTOCOL_ORIGINAL.md` - Original protocol spec (reference)
- `docs/reference/SCRIPTING_LANGUAGE_ORIGINAL.md` - Arturo DSL reference
- `schemas/` - JSON Schema message contracts
- `profiles/` - Device YAML profiles (SCPI, Modbus, CTI, etc.)
