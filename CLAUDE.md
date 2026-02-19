# CLAUDE.md

## Project Overview

Arturo is an industrial test automation system with ESP32 stations and a centralized Go controller, connected via Redis on a LAN.

**Terminology:** Stations (ESP32 + instruments), Terminal (operator UI), Controller (Go processes), File server (report storage).

- **Station firmware**: C++ with Arduino in `firmware/`
- **Services**: Go processes in `services/` — arturo-controller, arturo-engine, arturo-monitor, arturo-console
- **Redis**: Streams for commands/responses, Pub/Sub for heartbeats/E-stop
- **Profiles**: Device YAML profiles in `profiles/`
- **Schemas**: JSON Schema message definitions in `schemas/`

## Architecture

- Up to 6 stations connect directly to Redis over WiFi/Ethernet
- One Ubuntu machine runs the controller (Go processes) + Redis + terminal (operator UI)
- All messages use Protocol v1.0.0 envelope format (see docs/architecture/ARCHITECTURE.md section 2.1)
- 5 message types: `device.command.request`, `device.command.response`, `service.heartbeat`, `system.emergency_stop`, `system.ota.request`
- Test definitions are `.art` script files — the single unit of orchestration (see ARCHITECTURE.md section 2.8)
- Scripts are authorable by humans and LLMs; the engine provides parse-only validation and structured JSON errors

## Redis Channel Conventions

- `commands:{station-instance}` - Stream, controller -> specific station
- `responses:{requester-instance}` - Stream, station -> controller
- `events:heartbeat` - Pub/Sub, station -> controller
- `events:emergency_stop` - Pub/Sub + Stream, any -> all
- `device:{instance}:alive` - Key with 90s TTL for presence

## Build Commands

### Controller (Go)
```bash
cd services && go build -o arturo-controller ./cmd/arturo-controller
cd services && go build -o arturo-engine ./cmd/arturo-engine
cd services && go build -o arturo-monitor ./cmd/arturo-monitor
cd services && go build -o arturo-console ./cmd/arturo-console
```

### Station Firmware (ESP32)
```bash
cd firmware && pio run -e esp32s3                    # compile
cd firmware && pio run -e esp32s3 -t upload           # flash
cd firmware && pio device monitor --baud 115200       # serial monitor
cd firmware && pio test -e native                     # run unit tests on host
```

## Development Guidelines

- Keep service count low. If it can be a function call inside arturo-controller, it is not a separate service.
- Every message must use the Protocol v1.0.0 envelope format.
- Station firmware uses ArduinoJson v7 with static allocation.
- Debug output on stations goes to USB serial, controlled by DEBUG_LEVEL in config.h.
- Use `arturo-monitor` to observe all Redis traffic during development.
- Scripts go in `scripts/` (.art files) with shared libraries in `scripts/lib/` (.artlib files).
- Script engine interfaces (validation, error reporting, device introspection) must be LLM-usable — structured JSON, no implicit context.
- Reference material from the original project is in `docs/reference/` and the [arturo-go-archive](https://github.com/holla2040/arturo-go-archive) repo.

## Key Files

- `docs/architecture/ARCHITECTURE.md` - Architecture decisions, protocol spec, phasing, debugging setup
- `docs/reference/PROTOCOL_ORIGINAL.md` - Original protocol spec (reference)
- `docs/reference/SCRIPTING_LANGUAGE_ORIGINAL.md` - Arturo DSL reference
- `schemas/` - JSON Schema message contracts
- `profiles/` - Device YAML profiles (SCPI, Modbus, CTI, etc.)
- `scripts/` - Test scripts (.art) and shared libraries (.artlib)
