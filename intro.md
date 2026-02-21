# Arturo — Project Introduction

Comprehensive reference for onboarding into the Arturo codebase. Read this before starting work.

---

## What Arturo Is

Arturo is an **industrial test automation system** for controlling lab instruments (multimeters, power supplies, cryopumps, temperature controllers, relay boards) from a central operator station. It runs on a single LAN with no cloud dependency.

The system has three layers:

1. **Stations** — ESP32-S3 microcontrollers that physically connect to instruments and bridge them to Redis over WiFi. Up to 6 stations.
2. **Controller** — Go processes running on one Ubuntu machine. Manages stations, orchestrates tests, stores data, serves the operator UI.
3. **Redis** — Message backbone. Streams for reliable command/response delivery, Pub/Sub for heartbeats and emergency stop.

There is no MQTT, no cloud, no middleware. Stations talk directly to Redis.

---

## Terminology (Use These Exactly)

| Term | What It Is | What It Is NOT |
|------|-----------|----------------|
| **Station** | ESP32 + its connected instruments | Not "node" or "device" |
| **Terminal** | Operator web UI (screen, keyboard, barcode scanner) | Not "dashboard" |
| **Controller** | Headless Go backend (REST API, device registry, data store) | Not "server" |
| **Console** | Developer mock/test tool that simulates stations | Not "terminal" |
| **Monitor** | CLI Redis traffic viewer for debugging | Not "logger" |
| **Engine** | CLI tool for script validation and execution | Not "runner" |

**Time conventions:**
- `timestamp` = UTC epoch seconds (integer). Always UTC.
- `time` / `date` = Local time (US/Denver).

---

## Project Structure

```
arturo/
├── firmware/               # ESP32 C++ station firmware (Arduino/PlatformIO)
│   ├── src/
│   │   ├── main.cpp        # Entry point, FreeRTOS task creation
│   │   ├── config.h         # WiFi, Redis IP, instance ID, debug level
│   │   ├── network/         # wifi_manager, redis_client
│   │   ├── messaging/       # envelope, command_handler, heartbeat
│   │   ├── protocols/       # scpi, modbus, cti, ascii packetizers
│   │   ├── devices/         # tcp_device, serial_device, relay_controller, modbus_device, cti_onboard_device
│   │   └── safety/          # watchdog, estop, interlock, wifi_reconnect, power_recovery, ota_update
│   └── test/                # 16 Unity test suites (host-native, not on ESP32)
│
├── services/                # Go controller processes
│   ├── go.mod               # Module: github.com/holla2040/arturo
│   ├── cmd/
│   │   ├── controller/      # REST API, WebSocket, device registry, health, SQLite, E-stop
│   │   ├── console/         # Mock station spawner + web console
│   │   └── terminal/        # Operator web UI (reverse proxy to controller)
│   └── internal/            # 16 Go packages (see Services section)
│
├── tools/
│   ├── engine/              # Script parser + executor CLI
│   ├── monitor/             # Redis traffic monitor CLI
│   └── supervisor/          # Service process manager (separate Go module)
│
├── profiles/                # Device YAML profiles (command vocabulary per instrument)
│   ├── testequipment/       # Keysight 34461A, Fluke 8846A, Rigol DP832
│   ├── pumps/               # CTI On-Board cryopump
│   ├── controllers/         # Omega CN7500, Arduino UNO
│   ├── relays/              # USB 8-channel relay board
│   └── modbus/              # Generic Modbus TCP device
│
├── schemas/                 # JSON Schema message definitions (v1.0.0)
│   └── v1.0.0/              # 5 message types + envelope + error
│
├── scripts/                 # Test scripts (.art files)
│   ├── pump_cycle.art
│   ├── pump_status.art
│   └── regen_temp_monitor.art
│
├── tests/                   # Python tests (pytest)
│   ├── schemas/             # Schema validation tests
│   └── integration/         # Redis integration tests
│
├── redis/                   # Redis ACL config
├── docs/                    # Documentation
│   ├── architecture/        # ARCHITECTURE.md (core spec)
│   ├── SCRIPTING_HAL.md     # HAL reference for script authors
│   └── reference/           # Legacy reference docs (protocol, scripting, CTI)
│
├── CLAUDE.md                # Project instructions (build commands, guidelines)
├── README.md                # Project overview
├── SCRIPTING_DISCUSSION.md  # Scripting design decisions and open questions
└── Makefile                 # Test targets
```

---

## The Three Services

All three are Go binaries built from `services/cmd/`.

### Controller (`services/cmd/controller/`)

The headless backend. Runs always.

```bash
cd services && go build -o controller ./cmd/controller
./controller -redis localhost:6379 -listen :8002 -db arturo.db
```

Contains:
- **REST API + WebSocket hub** (`internal/api/`) — endpoints for device control, test management, real-time updates
- **Device registry** (`internal/registry/`) — maps device IDs to station instances, populated from heartbeats
- **E-stop coordinator** (`internal/estop/`) — listens for E-stop events, broadcasts to all, logs
- **Health monitor** (`internal/redishealth/`) — tracks Redis connection health
- **Test manager** (`internal/testmanager/`) — test lifecycle: start, pause, stop, artifacts
- **Data store** (`internal/store/`) — SQLite for test results, device events
- **Report generator** (`internal/report/`, `internal/artifact/`) — PDF reports, SMB file server integration
- **Protocol** (`internal/protocol/`) — envelope builder/parser, command handling, JSON schema validation

### Terminal (`services/cmd/terminal/`)

Operator web UI. Runs always. Serves HTML and reverse-proxies to the controller.

```bash
cd services && go build -o terminal ./cmd/terminal
./terminal -listen :8000 -controller http://localhost:8002
./terminal -listen :8000 -controller http://localhost:8002 -dev  # live reload
```

### Console (`services/cmd/console/`)

Development tool. Spawns mock stations with simulated pumps. Not for production.

```bash
cd services && go build -o console ./cmd/console
./console -stations 1,2,3,4                    # mock all four
./console -stations 2,3,4                      # mock 2-4, leave 1 for real hardware
./console -stations 1 -cooldown-hours 2.0      # faster cooling sim
./console -stations 1,2 -fail-rate 0.1         # 10% random command failure
```

Contains:
- **Mock pump simulator** (`internal/mockpump/`) — simulates CTI cryopump behavior (temperature curves, regen cycles, valve states)
- **Web console** (`internal/console/`) — browser UI for controlling mock stations

---

## The Script Engine

### Overview

Test definitions are `.art` script files. Scripts are the **single unit of orchestration** — there is no separate test configuration layer. If you want to know what a test does, you read the `.art` file.

The engine CLI (`tools/engine/`) has three modes:

```bash
engine validate <file.art>                                    # Parse-only, structured JSON errors
engine devices --profiles <dir>                               # List available devices/commands as JSON
engine run --redis <addr> --station <id> <file.art>           # Full execution via Redis
```

### Key Design Decisions

1. **Station-scoped execution** — a script runs on one station. The operator picks the station in the terminal, loads a script, hits run. Scripts never address stations by name — they just issue commands against whatever station they're bound to. `SEND "pump_on"`, not `SEND "PUMP-01" "pump_on"`.

2. **Profile-based abstraction** — scripts use logical command names (`"get_temp_1st_stage"`), not raw protocol commands (`"$P01J5C"`). Device profiles (YAML) map logical names to protocol calls. Script authors don't need to know the wire protocol.

3. **LLM-authoring first** — every interface the engine exposes (validation, error reporting, device introspection) is designed to be usable by both humans and LLMs. Structured JSON errors with line/column, queryable device vocabulary, no implicit context.

### Language Features

The parser is a complete recursive descent parser with 50+ token types and multi-error recovery.

**Fully working (parse + execute):**
- Variables: `SET`, `CONST`, `GLOBAL`, `DELETE`, `APPEND`, `EXTEND`
- Control flow: `IF`/`ELSEIF`/`ELSE`, `LOOP N TIMES`, `WHILE`, `FOREACH`, `BREAK`, `CONTINUE`
- Error handling: `TRY`/`CATCH`/`FINALLY`
- Functions: `FUNCTION`/`CALL`/`RETURN`
- Device I/O: `SEND`, `QUERY` (with `TIMEOUT`), `CONNECT`, `DISCONNECT`
- Testing: `TEST`, `SUITE`, `PASS`, `FAIL`, `SKIP`, `ASSERT`
- Utility: `LOG`, `DELAY`
- Expressions: arithmetic, comparison, logical, indexing, builtins (`FLOAT`, `INT`, `STRING`, `BOOL`, `LENGTH`, `TYPE`, `EXISTS`, `NOW`)

**Parse but don't execute yet:**
- `IMPORT` — parses but doesn't load files
- `LIBRARY` — parses but doesn't expose cross-file functions
- `PARALLEL` — parses but executes sequentially
- `RESERVE` — parses but doesn't enforce

### Engine Internals (12 packages under `services/internal/script/`)

```
script/
├── ast/           # AST node definitions
├── lexer/         # Tokenizer
├── token/         # Token type definitions
├── parser/        # Recursive descent parser → AST
├── executor/      # Walks AST, sends Redis commands, manages variables
├── redisrouter/   # Routes script commands to stations via Redis Streams
├── profile/       # Loads YAML device profiles, exposes command vocabulary
├── validate/      # Parse-only validation (no hardware)
├── result/        # Test result types (pass/fail/skip/assert)
└── variable/      # Scoped variable system
```

### Example Scripts

**Simple pump query** (`scripts/pump_status.art`):
```arturo
TEST "Pump Motor Status"
    QUERY "pump_status" status TIMEOUT 5000
    LOG INFO "Motor status: " + status
    PASS "Pump responded"
ENDTEST
```

**Regen temperature monitor** (`scripts/regen_temp_monitor.art`):
```arturo
CONST SAMPLE_INTERVAL 5000
TEST "Regen Temperature Monitor"
    QUERY "pump_status" status TIMEOUT 5000
    IF status == "0"
        SEND "pump_on"
        DELAY 2000
    ENDIF
    SEND "start_regen"
    SET regen_active true
    WHILE regen_active
        QUERY "get_regen_step" step TIMEOUT 5000
        IF step == 0
            SET regen_active false
        ELSE
            QUERY "get_temp_1st_stage" t1 TIMEOUT 5000
            QUERY "get_temp_2nd_stage" t2 TIMEOUT 5000
            LOG INFO "Step " + step + " — 1st: " + t1 + " K, 2nd: " + t2 + " K"
            DELAY SAMPLE_INTERVAL
        ENDIF
    ENDWHILE
    PASS "Regen completed"
ENDTEST
```

### Open Questions (from SCRIPTING_DISCUSSION.md)

1. How does the engine discover which profile a station uses? (CLI flag, Redis config, heartbeat metadata?)
2. IMPORT/LIBRARY need executor implementation before shared `.artlib` libraries work
3. Data recording strategy: explicit `RECORD` command vs. engine always records?
4. Should the engine subscribe to `events:emergency_stop` and abort scripts on E-stop?
5. INPUT() for operator prompts requires a new message flow engine→terminal
6. Where do test reports go, and does the terminal display results in real-time?

---

## Protocol v1.0.0

### Envelope

Every message wraps its payload in a standard envelope:

```json
{
  "envelope": {
    "id": "uuid-v4",
    "timestamp": 1752832800,
    "source": {
      "service": "controller",
      "instance": "ctrl-01",
      "version": "1.0.0"
    },
    "schema_version": "v1.0.0",
    "type": "device.command.request",
    "correlation_id": "uuid-v4",
    "reply_to": "responses:controller:ctrl-01"
  },
  "payload": { ... }
}
```

Key fields:
- `id` — UUIDv4, unique per message
- `timestamp` — UTC epoch seconds (integer)
- `type` — dot-notation message type
- `correlation_id` — links request to response (required for command request/response and OTA)
- `reply_to` — Redis Stream for the response (required for requests)

### Five Message Types

| Type | Transport | Direction | Purpose |
|------|-----------|-----------|---------|
| `device.command.request` | Redis Stream | Controller → Station | Execute a command on a device |
| `device.command.response` | Redis Stream | Station → Controller | Result of a device command |
| `service.heartbeat` | Redis Pub/Sub | Station → Controller | Periodic health report (every 30s) |
| `system.emergency_stop` | Redis Pub/Sub | Any → All | Emergency stop broadcast |
| `system.ota.request` | Redis Stream | Controller → Station | Firmware update request |

### Redis Channels

| Channel | Type | Direction | Purpose |
|---------|------|-----------|---------|
| `commands:{station-instance}` | Stream | Controller → Station | Per-station command delivery |
| `responses:{requester-instance}` | Stream | Station → Controller | Response delivery |
| `events:heartbeat` | Pub/Sub | Station → Controller | Fire-and-forget heartbeats |
| `events:emergency_stop` | Pub/Sub + Stream | Any → All | E-stop (Pub/Sub for speed + Stream for audit) |
| `device:{instance}:alive` | Key with 90s TTL | Station | Presence detection |

### Command Round-Trip

```
Controller:  XADD commands:dmm-station-01 * message <json>
Station:     XREAD BLOCK 0 STREAMS commands:dmm-station-01 $
Station:     Execute command on hardware
Station:     XADD responses:controller:ctrl-01 * message <json>
Station:     XACK commands:dmm-station-01
Controller:  XREAD responses:controller:ctrl-01, match correlation_id
```

### Error Codes

Errors use a standard error object with `code`, `message`, and optional `details`:

| Code | Meaning |
|------|---------|
| `E_DEVICE_TIMEOUT` | Device didn't respond within timeout_ms |
| `E_DEVICE_NOT_FOUND` | device_id doesn't match any connected device |
| `E_DEVICE_NOT_CONNECTED` | Device known but disconnected |
| `E_DEVICE_ERROR` | Device returned an error (SCPI error, Modbus exception) |
| `E_COMMAND_FAILED` | Command failed for non-device reason |
| `E_VALIDATION_FAILED` | Message failed schema validation |
| `E_INVALID_PARAMETER` | Parameter out of range or wrong type |
| `E_INTERNAL` | Unexpected internal error |

---

## Device Profiles

YAML files in `profiles/` defining each instrument's protocol and command vocabulary. The profile is the bridge between logical command names in scripts and raw protocol commands on the wire.

### Structure

```yaml
manufacturer: "Keysight Technologies"
model: "34461A"
type: "dmm"
protocol: "scpi"
packetizer:
  type: "scpi"
  line_ending: "\n"
commands:
  identify: "*IDN?"
  measure_dc_voltage: "MEAS:VOLT:DC?"
  measure_resistance: "MEAS:RES?"
  # ...
responses:
  success: "0,\"No error\""
  error: "ERR"
```

### Existing Profiles

| Profile | Type | Protocol | Commands |
|---------|------|----------|----------|
| `cti_onboard.yaml` | Cryopump | CTI (proprietary serial) | 42 commands — pump control, temps, pressure, regen, valves, PFR, diagnostics |
| `keysight_34461a.yaml` | DMM | SCPI (TCP) | 11 commands — voltage, current, resistance, frequency |
| `fluke_8846a.yaml` | DMM | SCPI (TCP) | 8 commands — voltage, current, resistance |
| `rigol_dp832.yaml` | Power supply | SCPI (TCP) | 12 commands — voltage/current set, output on/off, measure |
| `omega_cn7500.yaml` | Temp controller | Modbus RTU (RS485) | 3 commands — read temp, read/write setpoint |
| `usb_relay_8ch.yaml` | Relay board | ASCII (USB) | 8 commands — relay on/off/toggle, all on/off, status |

### HAL Reference

`docs/SCRIPTING_HAL.md` documents the abstract command vocabulary from the script author's perspective. It lists every logical command name, what it returns, units, and implementation status (Profile/Firmware/Mock) without exposing protocol details.

**Pump (CTI)** — fully documented with 42 commands across control, temperature, pressure, regen, valves, operating data, PFR, status bytes, and diagnostics. Each regen status value and error code is documented with human-readable names.

**Temperature controller, relay board, DMM, power supply** — placeholder sections with commands from their profiles.

---

## ESP32 Firmware

### Hardware

All stations use ESP32-S3: dual-core 240MHz, 512KB SRAM, WiFi. Memory budget: ~152KB used, ~360KB headroom.

Four firmware variants (same codebase, different compile-time config):

| Variant | Board | Connection | Instruments |
|---------|-------|-----------|-------------|
| TCP bridge | ESP32-S3 + W5500 | Ethernet to instrument, WiFi to Redis | SCPI (DMMs, PSUs) |
| Serial bridge | ESP32-S3 + MAX3232/MAX485 | UART to instrument, WiFi to Redis | CTI pumps, Modbus |
| Relay controller | ESP32-S3 + relay board | GPIO to relays, WiFi to Redis | Power switching |
| E-stop station | ESP32-S3 + button + LED | GPIO, WiFi to Redis | Safety only |

### FreeRTOS Tasks

| Task | Core | Priority | What it does |
|------|------|----------|-------------|
| `heartbeatTask` | 0 | 1 | Publish heartbeat every 30s, refresh presence key |
| `commandTask` | 0 | 2 | XREAD from Redis, dispatch commands |
| `deviceTask` | 1 | 2 | Execute commands on physical instruments |
| `watchdogTask` | 0 | 3 | Feed hardware watchdog, check E-stop GPIO |
| `loop()` | 1 | 1 | WiFi/Redis reconnection, status LED |

Core 0 = network tasks. Core 1 = hardware I/O. They don't block each other.

### Implementation Status

- **CTI protocol: fully implemented** — command lookup and execution work end-to-end
- **SCPI dispatch: stubbed** — returns "unsupported_protocol"
- **Modbus dispatch: stubbed** — same
- **Device registry: hardcoded**, not loaded from profiles

### Boot Sequence

1. Init GPIO (E-stop to safe state, relays off, status LED)
2. Init hardware watchdog (8s timeout)
3. Load config from NVS
4. Connect WiFi (retry with exponential backoff)
5. Connect Redis (retry with backoff)
6. Set presence key: `SET device:{instance}:alive EX 90`
7. Create FreeRTOS tasks
8. Publish first heartbeat (startup announcement)
9. Main loop: reconnection + status LED

### Safety

- **E-stop**: Physical button wired to GPIO interrupt. Local GPIO action happens **before** Redis publish. Hardware safety does not depend on the network.
- **Interlocks**: Local safety checks (temperature, over-current) that don't require network.
- **Watchdog**: 8-second hardware watchdog. If firmware hangs, ESP32 reboots.
- **OTA**: Dual-partition with automatic rollback. If new firmware can't connect to Redis within 30s, bootloader rolls back.

### Pin Assignments (Current)

| Function | GPIO | UART | Notes |
|----------|------|------|-------|
| CTI OnBoard RX (PUMP-01) | 17 | UART1 RXD | RS-232 via MAX3232, 2400 7E1 |
| CTI OnBoard TX (PUMP-01) | 18 | UART1 TXD | RS-232 via MAX3232, 2400 7E1 |
| USB Serial (debug) | — | UART0 | 115200 8N1, CDC on boot |

---

## Redis Security

Each station gets its own Redis ACL user. ACL config is in `redis/redis-acl.conf`.

**Station permissions (scoped per station):**
- Read own command stream (`commands:{instance}`)
- Write to any response stream (`responses:*`)
- Publish heartbeats and E-stop
- Manage own presence key (`device:{instance}:alive`)
- Cannot read other stations' commands or use admin commands

**Controller permissions:** full access to all keys and channels.

**Monitor tool:** read-only access to everything.

**Default user is disabled** — all connections require authentication.

Defined station users: `dmm-station-01`, `psu-station-01`, `relay-board-01`, `serial-bridge-01`, `estop-01`, `station-06` (spare).

---

## Tools

### Monitor (`tools/monitor/`)

Redis traffic viewer. Shows commands, responses, heartbeats, and presence in real-time with color coding and correlation tracking.

```bash
cd tools/monitor && go build -o monitor
./monitor                                   # Everything
./monitor --station dmm-station-01          # Filter to one station
./monitor --type device.command.*           # Filter by message type
./monitor --json                            # Raw JSON output
```

### Engine (`tools/engine/`)

Script parser and executor.

```bash
cd tools/engine && go build -o engine
./engine validate script.art                           # Parse-only
./engine devices --profiles ../profiles                # Device introspection
./engine run --redis localhost:6379 --station PUMP-01 script.art  # Execute
```

### Supervisor (`tools/supervisor/`)

Process manager for the controller and terminal services. Independent Go module. Uses `fsnotify` for file watching.

---

## Build Commands

### Go Services
```bash
cd services && go build -o controller ./cmd/controller
cd services && go build -o console ./cmd/console
cd services && go build -o terminal ./cmd/terminal
cd tools/engine && go build -o engine
cd tools/monitor && go build -o monitor
```

**Always rebuild after changing Go code.** Go is compiled — edits have no effect until you build.

### ESP32 Firmware
```bash
cd firmware && pio run -e esp32s3                     # compile
cd firmware && pio run -e esp32s3 -t upload            # flash
cd firmware && pio device monitor --baud 115200        # serial monitor
cd firmware && pio test -e native                      # run unit tests
```

Before flashing via USB, kill the serial monitor: `pkill -9 -f microcom`

### Tests
```bash
make test                    # Quick: schemas + Go
make test-schemas            # Schema validation only
make test-firmware           # PlatformIO native unit tests
make test-integration        # Redis integration tests (needs Redis running)
make test-all                # Everything except hardware
```

---

## Documentation Map

| File | What It Covers |
|------|---------------|
| `CLAUDE.md` | Build commands, development guidelines, key files |
| `README.md` | Project overview, architecture diagram, naming conventions |
| `SCRIPTING_DISCUSSION.md` | Scripting design decisions, engine status, open questions |
| `docs/architecture/ARCHITECTURE.md` | Core architecture spec: protocol, channels, phases, firmware, debugging |
| `docs/SCRIPTING_HAL.md` | HAL reference: abstract command vocabulary per device type |
| `docs/reference/SCRIPTING_LANGUAGE_ORIGINAL.md` | Full language reference (.art DSL syntax and features) |
| `docs/reference/PROTOCOL_ORIGINAL.md` | Original v2 protocol spec (reference, not current) |
| `docs/reference/CTI_COMMAND_REFERENCE.md` | CTI pump command reference |
| `docs/reference/CTI_BROOKS_PROTOCOL.md` | CTI Brooks protocol documentation |
| `services/README.md` | Service architecture, build/run commands, source structure |
| `firmware/README.md` | Firmware architecture: Arduino, FreeRTOS tasks, OTA, debug levels |
| `profiles/README.md` | Device profile directory layout |
| `schemas/README.md` | Schema directory layout and versioning |
| `schemas/v1.0.0/README.md` | Protocol v1.0.0 message type index |
| `schemas/v1.0.0/*/schema-definition.md` | Per-message-type JSON Schema + field docs + implementation examples |

---

## Current State Summary

**What works end-to-end:**
- CTI cryopump protocol: firmware can talk to real pumps via Redis
- Protocol v1.0.0 envelope: both Go and C++ build/parse correctly
- JSON Schemas: complete for all 5 message types + envelope + error
- Script engine: lex → parse → execute for the full language (minus IMPORT/LIBRARY/PARALLEL)
- Device profiles: 6 instruments defined, profile loading works in the engine
- Mock pump simulator: realistic temperature curves, regen cycles
- Console: spawns mock stations for development
- Terminal: serves operator UI, proxies to controller
- Redis ACL: per-station security configured

**What's stubbed or incomplete:**
- Firmware: SCPI and Modbus dispatch are stubbed (only CTI works)
- Firmware: Device registry is hardcoded, not loaded from profiles
- Engine: IMPORT/LIBRARY parse but don't execute
- Engine: PARALLEL parses but executes sequentially
- Engine: No E-stop subscription during script execution
- Engine: No INPUT() operator prompt flow
- Test reports: generation exists but storage/delivery TBD
- HAL reference: Only pump section is fully documented; others are placeholders

---

## Key Architectural Rules

1. **If it can be a function call inside the controller, it's not a service.** Keep service count low.
2. **Schemas are the single source of truth.** Code implements what the schemas define.
3. **Scripts are the unit of orchestration.** No GUI-only test builders, no database-stored test definitions.
4. **Station-scoped execution.** Scripts never address stations by name.
5. **Profile-based abstraction.** Scripts use logical command names, not raw protocol.
6. **Physical safety first.** E-stop GPIO acts locally before Redis notification.
7. **Build the monitor first.** Debugging infrastructure exists before the thing it debugs.
