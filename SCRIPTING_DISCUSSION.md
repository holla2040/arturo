# Scripting Discussion

Captured from conversation on 2026-02-21. Not a spec — a working doc for when we start building scripts.

## Key Design Decision: Station-Scoped Execution

Scripts run **on** a station, not **across** stations. The operator picks a station in the terminal, loads a script, hits run. The script never addresses a station by name — it just issues commands against whatever station it's bound to.

This means:
- No `SEND "PUMP-01" "pump_on"` — just `SEND "pump_on"`
- No `QUERY "THERMO-01" "first_stage_temp"` — just `QUERY "first_stage_temp"`
- No CONNECT/DISCONNECT in scripts — the station is already connected
- Same script works on any station with the same instrument capabilities
- **The existing pump scripts need updating** to drop the device ID argument

## Key Design Decision: Profile-Based Abstraction

Scripts use **logical command names**, not raw protocol commands. The script says *what* it wants; the device profile knows *how* to get it.

```arturo
QUERY "first_stage_temp" t1 TIMEOUT 5000
```

The profile YAML maps this to the actual protocol:
- CTI pump → register read
- SCPI instrument → `MEAS:TEMP1?`
- Modbus device → register address

Script authors don't need to know the protocol. This is the "how" layer.

## Where We Are Today

### Engine (tools/engine/) — Solid

The engine has three modes:
- `engine validate <file.art>` — parse-only with structured JSON errors
- `engine devices --profiles <dir>` — introspection, outputs available devices/commands as JSON
- `engine run --redis <addr> --station <id> <file.art>` — full execution: lex → parse → execute via Redis

**Parser:** Complete recursive descent. 50+ token types. Full error recovery with multi-error reporting.

**Executor:** Walks the AST with scoped variables. Routes commands through Redis to stations, correlates responses by UUID.

**Language features that parse AND execute:**
- SET, CONST, GLOBAL, DELETE, APPEND, EXTEND
- IF/ELSEIF/ELSE, LOOP N TIMES, WHILE, FOREACH, BREAK, CONTINUE
- TRY/CATCH/FINALLY
- FUNCTION/CALL/RETURN
- SEND, QUERY, CONNECT, DISCONNECT
- TEST, SUITE, PASS, FAIL, SKIP, ASSERT
- LOG, DELAY
- Expressions: arithmetic, comparison, logical, indexing, builtins (FLOAT, INT, STRING, BOOL, LENGTH, TYPE, EXISTS, NOW)

**Language features that parse but DON'T execute yet:**
- IMPORT — parses but doesn't load files
- LIBRARY — parses but doesn't expose cross-file functions
- PARALLEL — parses but executes sequentially
- RESERVE — parses but doesn't enforce

### HAL Reference (docs/SCRIPTING_HAL.md) — Complete for Pump

The HAL reference defines the abstract command vocabulary for script authors. It documents every logical command name, what it returns, units, and which layer implements it (profile, firmware, mock). No protocol details — just "here's what you can do with a pump."

- **Pump:** Fully documented — 42 profile commands covering control, temperature, pressure, regen (with phase/error descriptions), valves, operating data, PFR, status bytes, diagnostics. Implementation status tracked per command.
- **Temperature controller, relay board, DMM, power supply:** Placeholder sections with commands from their profiles.

### Profiles (profiles/) — Well-Designed

YAML files with manufacturer, model, protocol type, packetizer config, and command maps.

Existing profiles:
- `cti_onboard.yaml` — 42 commands (pump control, temps, pressure, regen, valves, PFR, diagnostics)
- `keysight_34461a.yaml` — SCPI DMM, 11 commands
- `fluke_8846a.yaml` — SCPI DMM, 8 commands
- `rigol_dp832.yaml` — SCPI power supply, 12 commands
- `omega_cn7500.yaml` — Modbus temperature controller, 3 commands
- `usb_relay_8ch.yaml` — ASCII relay board, 8 commands

Profile loading and introspection already work in the engine.

### Firmware — Partial

CommandHandler polls Redis, parses protocol v1.0.0 envelopes, dispatches to devices.

- **CTI protocol: fully implemented** — command lookup and execution work
- **SCPI dispatch: stubbed** — returns "unsupported_protocol"
- **Modbus dispatch: stubbed** — same
- Device registry is hardcoded, not loaded from profiles

### Schemas (schemas/v1.0.0/) — Complete

JSON Schema for all message types:
- `device.command.request` — device_id, command_name, parameters, timeout_ms
- `device.command.response` — success, response, error enum, duration_ms
- `service.heartbeat`, `system.emergency_stop`, `system.ota.request`

## Talking Points for When We Start

### 1. Script SEND/QUERY Syntax Needs to Change

Current scripts use `QUERY "PUMP-01" "pump_status" status`. With station-scoped execution and profile abstraction, this becomes `QUERY "pump_status" status`. The parser and executor need to handle this simpler form — one argument (the logical command name), not two (device + command).

### 2. How Does the Engine Know the Station's Profile?

When the engine runs a script on a station, it needs to know what profile that station has so it can validate command names. Options:
- Station config in Redis that maps station ID → profile name
- Passed as a CLI flag: `engine run --profile cti_onboard ...`
- Discovered from the station's heartbeat metadata

### 3. IMPORT/LIBRARY Need Implementation

The parser handles them, the executor doesn't. Before we write real scripts with shared utilities (e.g. `test_utilities.artlib`), we need the executor to resolve imports and load library functions into scope.

### 4. Should Scripts Record Data or Just Log It?

The reference doc mentions SAVE_JSON and GENERATE_REPORT. For temperature recording use cases, we need to decide:
- Does the script explicitly save data, or does the engine always record everything?
- Where does data go — file on the controller? Redis? Both?
- Is there a structured data collection command (like `RECORD "first_stage_temp" t1`) vs just LOG?

### 5. How Do Regions Work?

The temperature recording example assumes "region_start" and "region_status" are commands the station understands. Questions:
- Is a "region" a concept the station firmware manages?
- Or is the script itself defining the region (start recording → do stuff → stop recording)?
- What triggers region completion — a station event, a threshold, operator input?

### 6. PARALLEL — Do We Actually Need It?

With single-station execution, parallel is less critical. You're querying one station's instruments, which are behind one firmware instance. The station firmware could handle concurrent queries internally, but the script might not need to express that. Worth discussing whether to implement or defer.

### 7. Firmware Protocol Gaps

SCPI and Modbus dispatch are stubbed. Before scripts can talk to non-CTI instruments, firmware needs:
- SCPI client (TCP socket, send command string, read response)
- Modbus client (TCP or RTU, register read/write)
- Both need to be wired into the CommandHandler dispatch

### 8. Error Handling Strategy

TRY/CATCH works in the engine. But what happens when:
- A station goes offline mid-script?
- A query times out repeatedly?
- An emergency stop fires?

The engine already listens on Redis — does it subscribe to `events:emergency_stop` and abort? This needs to be wired up.

### 9. Operator Interaction

INPUT() for prompting the operator (e.g. "enter serial number") requires the engine to communicate back to the terminal UI. This is a new message flow that doesn't exist yet.

### 10. Test Reports

The engine already records pass/fail/skip/assert results to JSON. But:
- Where does the report go?
- Does the terminal display results in real-time or after completion?
- How does this integrate with the file server for report storage?
