# Arturo TODO

## Agent Instructions

You are picking up work on the Arturo project. Follow these steps:

1. Read this entire file to understand project state
2. Find the first task marked `[ ]` (not started) that is NOT in a blocked section
3. Mark it `[-]` (in progress) before you start working
4. Read the "Read first" files listed for that phase before writing any code
5. Do the work — write code, write tests, verify it builds
6. When done, mark the task `[x]` and move to the next `[ ]` task
7. If you finish all available tasks or get blocked, stop and report what you accomplished

Do NOT skip ahead to blocked phases. Do NOT start a task someone else marked `[-]`.

Key project files:
- `CLAUDE.md` — project overview, architecture, build commands, conventions
- `docs/architecture/MIGRATION_PLAN.md` — full architecture spec and design decisions
- `docs/reference/SCRIPTING_LANGUAGE_ORIGINAL.md` — .art DSL language reference

---

## Completed

- [x] Phase 0: Freeze Contracts — 7 JSON schemas, 8 device profiles, 69 schema tests pass
- [x] Phase 1: Heartbeat + Monitor — arturo-monitor working, ESP32 heartbeats, presence keys
- [x] Phase 2: Command Round-Trip — SCPI client, Redis streams, CLI sender, 39 Go tests pass
- [x] Phase 3: Relay and Serial Variants — 9 firmware modules (devices, protocols, safety), 133 new tests, 172 total pass
- [x] Phase 4: Controller Core — device registry, health monitor, REST API, WebSocket, SQLite, E-stop coordinator, report generator, arturo-server rewrite

---

## Phase 3: Relay and Serial Variants (firmware/, C++) — COMPLETE

Read first: `firmware/README.md`, `firmware/src/protocols/scpi_client.cpp` (pattern to follow), `firmware/src/commands/command_handler.cpp`

- [x] TCP device client — `firmware/src/devices/tcp_device.cpp` — TCP socket client for SCPI instruments over Ethernet. Follow scpi_client.cpp pattern. Needs connect/disconnect, send/receive with timeout, reconnect logic.
- [x] Serial device bridge — `firmware/src/devices/serial_device.cpp` — HardwareSerial wrapper for UART instruments (CTI pumps, Modbus). Configure baud, parity, stop bits from device profile.
- [x] Relay controller — `firmware/src/devices/relay_controller.cpp` — GPIO output for relay channels. Safe-state on boot (all off). Channel validation against profile.
- [x] Modbus device client — `firmware/src/devices/modbus_device.cpp` — RS485 Modbus RTU. Function codes 03 (read holding), 06 (write single), 16 (write multiple). CRC16 calculation. Reference: `profiles/controllers/omega_cn7500.yaml`
- [x] CTI packetizer — `firmware/src/protocols/cti.cpp` — Port from Go reference in arturo-go-archive `src/common/protocols/cti.go`. Checksum calculation, response code parsing. Reference: `docs/reference/CTI_BROOKS_PROTOCOL.md`
- [x] Modbus packetizer — `firmware/src/protocols/modbus.cpp` — Port from Go reference. CRC16, slave addressing, function code dispatch.
- [x] Watchdog timer — `firmware/src/safety/watchdog.cpp` — Hardware watchdog, 8s timeout, feed from main loop.
- [x] E-stop handler — `firmware/src/safety/estop.cpp` — GPIO button input, immediate local relay shutoff, publish `system.emergency_stop` to Redis.
- [x] Safety interlocks — `firmware/src/safety/interlock.cpp` — Local checks (temperature, over-current) that don't depend on network.

## Phase 4: Controller Core (server/, Go) — COMPLETE

Read first: `server/cmd/arturo-server/main.go` (current stub), `server/internal/protocol/` (envelope/command builders), `docs/architecture/MIGRATION_PLAN.md` section 5

- [x] Device registry — `server/internal/registry/` — Map device IDs to station instances. Populate from heartbeat payloads (devices field). Lookup for command routing. 20 tests.
- [x] Health monitor — Track heartbeat timestamps per station. Flag OFFLINE after 90s silence. Expose status via registry. (integrated in registry package)
- [x] REST API — `server/internal/api/` — Endpoints: GET /devices, GET /devices/{id}, POST /devices/{id}/command, GET /system/status, GET /reports/{id}/csv, GET /reports/{id}/json. Go 1.22 net/http.ServeMux. 19 tests.
- [x] WebSocket — `server/internal/api/websocket.go` — Push real-time heartbeats, command results, e-stop events to browser clients. 6 tests.
- [x] SQLite storage — `server/internal/store/` — Tables: test_runs, measurements, device_history. Pure Go SQLite (modernc.org/sqlite). 13 tests.
- [x] E-stop coordinator — `server/internal/estop/` — State management + callbacks. Redis subscription wired in main.go. 13 tests.
- [x] Report generator — `server/internal/report/` — CSV/JSON export of test results from SQLite. 8 tests.
- [x] arturo-server main.go rewrite — HTTP server mode (default) + legacy `send` subcommand. Heartbeat/estop/response listeners, health check ticker, graceful shutdown.

## Phase 5: Script Engine (server/, Go) — UNBLOCKED

Read first: `docs/reference/SCRIPTING_LANGUAGE_ORIGINAL.md`, `docs/architecture/MIGRATION_PLAN.md` sections 2.6, 2.8, 5.3

- [ ] Lexer — Tokenize .art source files. Port token types from arturo-go-archive `src/domains/automation/19_script_parser/token.go`
- [ ] Parser — Recursive descent parser producing AST. Port grammar from arturo-go-archive `src/domains/automation/19_script_parser/parser.go`
- [ ] Executor — Walk AST, route device commands through Redis Streams (not direct calls). See MIGRATION_PLAN.md section 5.3.
- [ ] Variable system — Scoped variables, constants, loop counters.
- [ ] Validate mode — `arturo-engine validate <script.art>` — parse-only, structured JSON errors (line/col/message). See MIGRATION_PLAN.md section 2.8.
- [ ] Device introspection — `arturo-engine devices` — dump profiles as JSON for LLM consumption.
- [ ] Test result aggregation — Pass/fail per TEST block, SUITE rollup, structured report output.

## Phase 6: Dashboard and Reports — BLOCKED until Phase 4 REST API and Phase 5 are [x]

- [ ] Web dashboard — Station status, active test progress, recent measurements, E-stop status
- [ ] PDF report generation
- [ ] CSV export

## Phase 7: Hardening — BLOCKED until all above phases are [x]

- [ ] WiFi disconnect/reconnect under load
- [ ] Redis connection loss and recovery
- [ ] Power failure recovery
- [ ] Watchdog timer verification
- [ ] E-stop end-to-end test
- [ ] OTA firmware update mechanism

---

## Parallelism Guide

Phase 3 (firmware/C++) and Phase 4 (server/Go) are independent — different languages, different directories. Two agents can work these simultaneously without conflicts.

All other phases are sequential due to dependencies.
