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

---

## Phase 3: Relay and Serial Variants (firmware/, C++)

Read first: `firmware/README.md`, `firmware/src/protocols/scpi_client.cpp` (pattern to follow), `firmware/src/commands/command_handler.cpp`

- [ ] TCP device client — `firmware/src/devices/tcp_device.cpp` — TCP socket client for SCPI instruments over Ethernet. Follow scpi_client.cpp pattern. Needs connect/disconnect, send/receive with timeout, reconnect logic.
- [ ] Serial device bridge — `firmware/src/devices/serial_device.cpp` — HardwareSerial wrapper for UART instruments (CTI pumps, Modbus). Configure baud, parity, stop bits from device profile.
- [ ] Relay controller — `firmware/src/devices/relay_controller.cpp` — GPIO output for relay channels. Safe-state on boot (all off). Channel validation against profile.
- [ ] Modbus device client — `firmware/src/devices/modbus_device.cpp` — RS485 Modbus RTU. Function codes 03 (read holding), 06 (write single), 16 (write multiple). CRC16 calculation. Reference: `profiles/controllers/omega_cn7500.yaml`
- [ ] CTI packetizer — `firmware/src/protocols/cti.cpp` — Port from Go reference in arturo-go-archive `src/common/protocols/cti.go`. Checksum calculation, response code parsing. Reference: `docs/reference/CTI_BROOKS_PROTOCOL.md`
- [ ] Modbus packetizer — `firmware/src/protocols/modbus.cpp` — Port from Go reference. CRC16, slave addressing, function code dispatch.
- [ ] Watchdog timer — `firmware/src/safety/watchdog.cpp` — Hardware watchdog, 8s timeout, feed from main loop.
- [ ] E-stop handler — `firmware/src/safety/estop.cpp` — GPIO button input, immediate local relay shutoff, publish `system.emergency_stop` to Redis.
- [ ] Safety interlocks — `firmware/src/safety/interlock.cpp` — Local checks (temperature, over-current) that don't depend on network.

## Phase 4: Controller Core (server/, Go)

Read first: `server/cmd/arturo-server/main.go` (current stub), `server/internal/protocol/` (envelope/command builders), `docs/architecture/MIGRATION_PLAN.md` section 5

- [ ] Device registry — `server/internal/registry/` — Map device IDs to station instances. Populate from heartbeat payloads (devices field). Lookup for command routing. Reference: MIGRATION_PLAN.md section 5.2
- [ ] Health monitor — Track heartbeat timestamps per station. Flag OFFLINE after 90s silence. Expose status via registry.
- [ ] REST API — `server/cmd/arturo-server/` — Endpoints: GET /devices, GET /devices/:id, POST /devices/:id/command, GET /system/status. Use net/http or chi router.
- [ ] WebSocket — Push real-time heartbeats and command results to browser clients.
- [ ] SQLite storage — `server/internal/store/` — Tables: test_runs, measurements, device_history. Store command results and test outcomes.
- [ ] E-stop coordinator — Subscribe to `events:emergency_stop`. Broadcast to all stations. Log to stream. Expose status via API.
- [ ] Report generator — CSV/JSON export of test results from SQLite.

## Phase 5: Script Engine (server/, Go) — BLOCKED until Phase 4 device registry is [x]

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
