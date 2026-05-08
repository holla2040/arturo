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

## Two-Machine Workflow

This repo is checked out on two machines that share work via `origin`. Detect which one you are on with `hostname` (or check the working directory):

- **Dev box** — hostname `cryo`, working dir `/home/holla/arturo`. The **only** place where source is edited — firmware, Go, schemas, docs, terminal HTML/JS/CSS. PlatformIO/Arduino builds and ESP32 flashing happen here. Has Go 1.18, which can't build the controller; that's expected — do not try to `go build` here.
- **arturo-01** — hostname `arturo-01`, working dir `/home/cryo/arturo`. The build and runtime host for Go (controller, terminal, engine, console) and for Redis. Treat as a build/run target, **not an editing host**. SSH as `cryo` for builds and tests; SSH as `root` for systemd service restarts (`cryo` has no passwordless sudo).

**Editing rule — single direction.** Always edit on the dev box, **including** Go source under `subsystems/cmd/`, `subsystems/internal/`, `subsystems/pkg/`. **Never edit anything on arturo-01** — that checkout is pull-only. One editing direction is what keeps the two checkouts in sync; bidirectional edits are how repos drift.

**Sync rule — git only.** Source flows through `origin`. **Never** `scp`/`rsync` source between checkouts; that bypasses history and produces drift that needs manual reconciliation. WIP that needs to be tried on arturo-01 without a "real" commit goes through a `wip` or feature branch pushed to origin and pulled on the other side.

**Session start checklist (every session that may touch source):**
1. `hostname` — confirm which machine you're on.
2. `git fetch && git status` — confirm working tree state and branch position.
3. If behind, `git pull --ff-only`. Stop and ask if the pull won't fast-forward — never resolve the divergence by force.

**Driving arturo-01 from a dev-box Claude session.** This is the canonical pattern for changes that span both machines. Mechanical builds, tests, and service restarts run via SSH from the same Claude session — do not spawn a separate Claude on arturo-01 unless arturo-01 needs to *make decisions* (debugging runtime state you can't reproduce locally, iterating on a design where its test output drives the next edit).

```bash
# After editing, committing, and pushing on dev box:
ssh cryo@arturo-01 'cd /home/cryo/arturo && git pull --ff-only'
ssh cryo@arturo-01 'export PATH=/usr/local/go/bin:$PATH && cd /home/cryo/arturo/subsystems && go build -o controller ./cmd/controller && go build -o terminal ./cmd/terminal'
ssh cryo@arturo-01 'export PATH=/usr/local/go/bin:$PATH && cd /home/cryo/arturo/subsystems && go test ./...'
ssh root@arturo-01 'systemctl restart arturo-controller arturo-terminal'
```

Flash station firmware from dev box: `cd subsystems/station && make flash` (default `STATION=station-05`, the dev box's wired pump; override with `make flash STATION=station-NN`).

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
