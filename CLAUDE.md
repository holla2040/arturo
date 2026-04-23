# CLAUDE.md

## Project Overview

Arturo is an industrial test automation system with ESP32 stations and a centralized Go controller, connected via Redis on a LAN.

**Terminology:** Stations (ESP32 + instruments), Terminal (operator UI), Controller (Go processes), File server (report storage).

- **Station firmware**: C++ with Arduino in `subsystems/station/`
- **Subsystems**: Go processes in `subsystems/` — controller, console, terminal
- **Tools**: Go tools in `tools/` — engine, monitor
- **Redis**: Streams for commands/responses, Pub/Sub for heartbeats/E-stop
- **Profiles**: Device YAML profiles in `profiles/`
- **Schemas**: JSON Schema message definitions in `schemas/`

## Architecture

- Up to 6 stations connect directly to Redis over WiFi/Ethernet
- One Ubuntu machine runs the controller (Go processes) + Redis + terminal (operator UI)
- All messages use Protocol v1.0.0 envelope format (see docs/architecture/ARCHITECTURE.md section 2.1)
- 6 message types: `device.command.request`, `device.command.response`, `service.heartbeat`, `system.emergency_stop`, `system.ota.request`, `test.state.update`
- Test definitions are `.art` script files — the single unit of orchestration (see ARCHITECTURE.md section 2.8)
- Scripts are authorable by humans and LLMs; the engine provides parse-only validation and structured JSON errors
- Scripts are **station-scoped** — they run on one station, never address stations or devices by name
- Scripts **must follow the HAL** (`docs/SCRIPTING_HAL.md`). SEND and QUERY take a logical command name only — no device IDs, no raw protocol commands. The device profile maps the command name to the wire protocol. Example: `SEND "pump_on"`, `QUERY "get_temp_1st_stage" t1 TIMEOUT 5000`

## Redis Channel Conventions

- `commands:{station-instance}` - Stream, controller -> specific station
- `responses:{requester-instance}` - Stream, station -> controller
- `events:heartbeat` - Pub/Sub, station -> controller
- `events:emergency_stop` - Pub/Sub + Stream, any -> all
- `device:{instance}:alive` - Key with 90s TTL for presence

## Build Commands

### Controller (Go)
```bash
cd subsystems && go build -o controller ./cmd/controller
cd tools/engine && go build -o engine
cd tools/monitor && go build -o monitor
cd subsystems && go build -o console ./cmd/console
cd subsystems && go build -o terminal ./cmd/terminal
```

### Station Firmware (ESP32)
**Use the station Makefile only. Never call `pio` directly.**
```bash
cd subsystems/station && make                         # compile (default target)
cd subsystems/station && make flash                   # compile + flash + restart logger
cd subsystems/station && make test                    # run unit tests on host
cd subsystems/station && make monitor                 # serial monitor (foreground)
```

## Collaboration Rules

- **Do exactly what is asked. Nothing more.**
- **Do not touch files, systems, or subsystems not mentioned in the request.** If the task is firmware, do not touch Go. If the task is Go, do not touch firmware. Stay in scope.
- **Do not fix things you notice along the way.** If you spot an unrelated bug, mention it — do not fix it.
- **Ask before expanding scope.** If you think a related change is needed, say so and wait for confirmation.

## Development Guidelines

- **Always rebuild after changing Go code.** Go is compiled — edits have no effect until you build the affected binary.
- Keep service count low. If it can be a function call inside the controller, it is not a separate service.
- Every message must use the Protocol v1.0.0 envelope format.
- Station firmware uses ArduinoJson v7 with static allocation.
- Debug output on stations goes to USB serial, controlled by DEBUG_LEVEL in config.h.
- Use `monitor` to observe all Redis traffic during development.
- Scripts go in `scripts/` (.art files) with shared libraries in `scripts/lib/` (.artlib files).
- Script engine interfaces (validation, error reporting, device introspection) must be LLM-usable — structured JSON, no implicit context.
- Reference material from the original project is in `docs/reference/` and the [arturo-go-archive](https://github.com/holla2040/arturo-go-archive) repo.
- **pendant2** (`~/pendant2`) is a predecessor/reference project. Check there for design precedents when implementing station display features.

## Key Files

- `docs/architecture/ARCHITECTURE.md` - Architecture decisions, protocol spec, phasing, debugging setup
- `docs/architecture/TEST_EVENTS.md` - Scope of the operator-facing Test Events log and allowed event types
- `docs/SCRIPTING_HAL.md` - HAL reference for script authors (abstract command vocabulary per device type)
- `docs/reference/PROTOCOL_ORIGINAL.md` - Original protocol spec (reference)
- `docs/reference/SCRIPTING_LANGUAGE_ORIGINAL.md` - Arturo DSL reference
- `docs/hardware/psram-lcd-jitter/` - PSRAM bus contention fix (WiFi + RGB LCD jitter research, plan, results)
- `SCRIPTING_DISCUSSION.md` - Scripting design decisions, engine status, and open questions
- `schemas/` - JSON Schema message contracts
- `profiles/` - Device YAML profiles (SCPI, Modbus, CTI, etc.)
- `scripts/` - Test scripts (.art) and shared libraries (.artlib)
