# CLAUDE.md

## Project Overview

Arturo is an industrial test automation system with ESP32 field devices and a centralized Go server, connected via Redis on a LAN.

- **ESP32 firmware**: C++ with Arduino/PlatformIO in `firmware/`
- **Go server**: `server/` with two main binaries (arturo-server, arturo-engine) and one debug tool (arturo-monitor)
- **Redis**: Streams for commands/responses, Pub/Sub for heartbeats/E-stop
- **Profiles**: Device YAML profiles in `profiles/`
- **Schemas**: JSON Schema message definitions in `schemas/`

## Architecture

- Up to 6 ESP32 nodes connect directly to Redis over WiFi/Ethernet
- One Ubuntu server runs Go processes + Redis
- All messages use Protocol v1.0.0 envelope format (see docs/architecture/MIGRATION_PLAN.md section 2.1)
- 4 message types for v1: `device.command.request`, `device.command.response`, `service.heartbeat`, `system.emergency_stop`

## Redis Channel Conventions

- `commands:{node-instance}` - Stream, server -> specific ESP32
- `responses:{requester-instance}` - Stream, ESP32 -> server
- `events:heartbeat` - Pub/Sub, ESP32 -> server
- `events:emergency_stop` - Pub/Sub + Stream, any -> all
- `device:{instance}:alive` - Key with 90s TTL for presence

## Build Commands

### Go Server
```bash
cd server && go build -o arturo-server ./cmd/arturo-server
cd server && go build -o arturo-engine ./cmd/arturo-engine
cd server && go build -o arturo-monitor ./cmd/arturo-monitor
```

### ESP32 Firmware
```bash
cd firmware && pio run                  # Build
cd firmware && pio run -t upload        # Flash
cd firmware && pio device monitor       # Serial console
```

## Development Guidelines

- Keep service count low. If it can be a function call inside arturo-server, it is not a separate service.
- Every message must use the Protocol v1.0.0 envelope format.
- ESP32 firmware uses ArduinoJson v7 with static allocation.
- Debug output on ESP32 goes to USB serial, controlled by DEBUG_LEVEL in config.h.
- Use `arturo-monitor` to observe all Redis traffic during development.
- Reference material from the original project is in `docs/reference/` and the [arturo-go-archive](https://github.com/holla2040/arturo-go-archive) repo.

## Key Files

- `docs/architecture/MIGRATION_PLAN.md` - Full build plan, architecture, phasing, debugging setup
- `docs/reference/PROTOCOL_ORIGINAL.md` - Original protocol spec (reference)
- `docs/reference/SCRIPTING_LANGUAGE_ORIGINAL.md` - Arturo DSL reference
- `schemas/` - JSON Schema message contracts
- `profiles/` - Device YAML profiles (SCPI, Modbus, CTI, etc.)
