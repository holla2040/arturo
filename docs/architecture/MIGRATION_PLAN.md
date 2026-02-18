# Arturo v2: Fresh Build Plan

**Date:** 2026-02-17
**Approach:** Start over. Carry forward architecture decisions as specs, not code.

---

## 1. What We're Building

| Layer | Count | Language | Role |
|-------|-------|---------|------|
| ESP32 field nodes | up to 6 | C++ (Arduino/PlatformIO) | Bridge hardware to Redis |
| Ubuntu server | 1 | Go | Orchestration, UI, data, scripting |
| Redis | 1 | - | Message backbone |

All on a single LAN. No cloud. No MQTT (for now).

---

## 2. What We're Carrying Forward (as specs, not code)

These architecture decisions from the original project are proven and worth keeping:

### 2.1 Protocol v1.0.0 Envelope Format

Every message between any component uses this structure:

```json
{
  "envelope": {
    "id": "uuid-v4",
    "timestamp": "2025-07-18T10:00:00.123Z",
    "source": {
      "service": "esp32_relay_controller",
      "instance": "relay-board-01",
      "version": "1.0.0"
    },
    "schema_version": "v1.0.0",
    "type": "device.command.response",
    "correlation_id": "corr-456",
    "reply_to": "responses/orchestrator/orch-01"
  },
  "payload": { }
}
```

**Trimmed for v1.** Only these envelope fields are required:

| Field | Required | Purpose |
|-------|----------|---------|
| `id` | Yes | UUIDv4, unique per message |
| `timestamp` | Yes | RFC3339 with milliseconds |
| `source.service` | Yes | Who sent this |
| `source.instance` | Yes | Which instance |
| `source.version` | Yes | Firmware/software version |
| `schema_version` | Yes | Always "v1" for now |
| `type` | Yes | Message type (dot notation) |
| `correlation_id` | Yes | Links request to response |
| `reply_to` | For requests | Where to send the response |
| `trace_id` | No | Deferred |
| `auth` | No | Deferred (LAN trust for v1) |

### 2.2 Four Initial Message Types

Nothing else until these four work end-to-end:

**1. `device.command.request`** (server -> ESP32)
```json
{
  "envelope": { "type": "device.command.request", "reply_to": "stream:responses/orchestrator/orch-01", "..." : "..." },
  "payload": {
    "device_id": "fluke-8846a",
    "command": "measure_dc_voltage",
    "parameters": {},
    "timeout_ms": 5000
  }
}
```

**2. `device.command.response`** (ESP32 -> server)
```json
{
  "envelope": { "type": "device.command.response", "correlation_id": "corr-456", "..." : "..." },
  "payload": {
    "device_id": "fluke-8846a",
    "success": true,
    "response": "1.23456789",
    "duration_ms": 45
  }
}
```

**3. `service.heartbeat`** (ESP32 -> server, every 30s)
```json
{
  "envelope": { "type": "service.heartbeat", "..." : "..." },
  "payload": {
    "status": "running",
    "uptime_seconds": 3600,
    "devices": ["fluke-8846a"],
    "free_heap": 245000,
    "wifi_rssi": -42
  }
}
```

**4. `system.emergency_stop`** (anyone -> everyone)
```json
{
  "envelope": { "type": "system.emergency_stop", "..." : "..." },
  "payload": {
    "reason": "physical_button",
    "triggered_by": "relay-board-01",
    "severity": "critical"
  }
}
```

### 2.3 Channel Architecture (Redis Streams + Pub/Sub)

**Critical distinction: Streams for reliable commands, Pub/Sub for fire-and-forget telemetry.**

| Channel | Redis Type | Direction | Purpose |
|---------|-----------|-----------|---------|
| `commands:{device-instance}` | **Stream** | Server -> ESP32 | Reliable command delivery |
| `responses:{requester-instance}` | **Stream** | ESP32 -> Server | Reliable response delivery |
| `events:heartbeat` | Pub/Sub | ESP32 -> Server | Heartbeat telemetry |
| `events:emergency_stop` | **Both** | Any -> All | E-stop (Pub/Sub for speed + Stream for audit) |

Why Streams for commands:
- At-least-once delivery (messages persist until acknowledged)
- Consumer groups for multiple readers
- Replay capability if server restarts
- Built-in message IDs and ordering

Why Pub/Sub for heartbeats:
- Missing a heartbeat isn't critical (next one comes in 30s)
- Lower overhead
- No acknowledgment needed

**Redis key for device presence:**
- `device:{instance-id}:alive` with 90-second TTL
- ESP32 refreshes on each heartbeat
- Server checks key existence for liveness

### 2.4 Device Profiles

YAML definitions describing each instrument's protocol, connection parameters, and command vocabulary. These get compiled into ESP32 firmware or served from the server.

Profiles to carry forward from the original project:

| Profile | Type | Protocol | Connection |
|---------|------|----------|------------|
| `fluke_8846a` | DMM | SCPI | TCP (Ethernet) |
| `keysight_34461a` | DMM | SCPI | TCP (Ethernet) |
| `rigol_dp832` | PSU | SCPI | TCP (Ethernet) |
| `cti_onboard` | Cryopump | CTI (proprietary) | UART (RS232) |
| `omega_cn7500` | Temp Controller | Modbus RTU | UART (RS485) |
| `usb_relay_8ch` | Relay Board | GPIO | Direct GPIO |

### 2.5 Protocol Packetizers

Abstract interface for formatting commands and parsing responses per device protocol:

| Packetizer | Devices | Key Behavior |
|------------|---------|-------------|
| **SCPI** | DMMs, PSUs, Oscilloscopes | Append `\n`, validate response for ERR |
| **ASCII** | Simple devices | Configurable terminator, parameter substitution |
| **Modbus RTU** | Temp controllers, PLCs | Slave address, function codes, CRC16 |
| **CTI** | Cryopumps | Checksum calculation, response code parsing |
| **Binary** | Embedded sensors | STX/ETX framing, optional checksum |

These must be reimplemented in C++ for ESP32. The Go implementations in `src/common/protocols/` serve as reference.

### 2.6 Arturo Scripting Language (.art)

DSL for test automation. Grammar and semantics are proven:

```
TEST "Measure DC Voltage"
  CONNECT fluke-8846a
  SEND "MEAS:VOLT:DC?"
  SET result = RECEIVE
  ASSERT result > 0.0
  ASSERT result < 10.0
  DISCONNECT fluke-8846a
  PASS
END TEST
```

The parser and executor are the most complex server components. Carry forward the grammar from `src/domains/automation/19_script_parser/` as the language spec. Rewrite the implementation in Go.

### 2.7 Safety Interlock Concepts

- Physical E-stop button on ESP32 -> instant local power cut via GPIO
- E-stop message broadcast to all nodes via Redis
- Local interlocks that don't depend on network (temperature, over-current)
- Recovery requires explicit operator acknowledgment
- All E-stop events are logged to persistent Stream

---

## 3. Redis Security (Even on LAN)

Each ESP32 gets its own Redis ACL user:

```
# Redis ACL for relay-board-01
user relay-board-01 on >password123
  ~commands:relay-board-01      # Read own command stream
  ~responses:*                   # Write to response streams
  ~events:heartbeat              # Publish heartbeats
  ~events:emergency_stop         # Publish/subscribe E-stop
  ~device:relay-board-01:*       # Own presence keys
```

Server gets broader permissions:

```
user orchestrator on >serverpass
  ~commands:*      # Write commands to any device
  ~responses:*     # Read all responses
  ~events:*        # Read all events
  ~device:*        # Read all presence keys
  allcommands
```

This prevents a compromised ESP32 from reading other devices' commands or writing to arbitrary keys.

---

## 4. ESP32 Firmware Architecture

### 4.1 Hardware

| Variant | Board | Use Case | Interfaces |
|---------|-------|----------|-----------|
| TCP bridge | ESP32-S3 + W5500 Ethernet | SCPI instruments | Ethernet to instrument, WiFi to Redis |
| Serial bridge | ESP32-S3 + MAX3232 or MAX485 | Serial devices | UART to instrument, WiFi to Redis |
| Relay controller | ESP32-S3 + relay board | Power switching | GPIO to relays, WiFi to Redis |
| E-stop node | ESP32-S3 + button + LED | Safety | GPIO button input, LED output, WiFi to Redis |

The ESP32-S3 is recommended for all variants: dual-core 240MHz, 512KB SRAM, WiFi, plenty of GPIO.

### 4.2 Firmware Module Structure

```
arturo-esp32/
├── platformio.ini
├── src/
│   ├── main.cpp
│   ├── config.h                    # WiFi SSID, Redis IP, device profile, instance ID
│   │
│   ├── network/
│   │   ├── wifi_manager.cpp        # Connect, reconnect, exponential backoff
│   │   └── redis_client.cpp        # XREAD, XADD, PUBLISH, SUBSCRIBE (minimal RESP)
│   │
│   ├── messaging/
│   │   ├── envelope.cpp            # Build/parse Protocol v1.0.0 JSON envelopes
│   │   ├── command_handler.cpp     # Dispatch incoming commands to device driver
│   │   └── heartbeat.cpp           # 30-second heartbeat publisher
│   │
│   ├── protocols/                  # Ported from Go src/common/protocols/
│   │   ├── packetizer.h            # Abstract interface
│   │   ├── scpi.cpp
│   │   ├── modbus.cpp
│   │   ├── cti.cpp
│   │   └── ascii.cpp
│   │
│   ├── devices/
│   │   ├── tcp_device.cpp          # TCP socket client for SCPI instruments
│   │   ├── serial_device.cpp       # HardwareSerial for UART instruments
│   │   ├── relay_controller.cpp    # GPIO output for relay channels
│   │   └── modbus_device.cpp       # RS485 Modbus RTU
│   │
│   └── safety/
│       ├── watchdog.cpp            # Hardware watchdog timer
│       ├── estop.cpp               # GPIO E-stop button, local power cut
│       └── interlock.cpp           # Local safety checks (temp, current)
│
└── test/
    ├── test_envelope.cpp
    └── test_scpi.cpp
```

### 4.3 Memory Budget (ESP32-S3, 512KB SRAM)

| Component | Estimate | Notes |
|-----------|----------|-------|
| FreeRTOS + WiFi | ~80 KB | Fixed |
| Redis client + buffers | ~20 KB | RESP protocol, 2KB tx/rx buffers |
| ArduinoJson | ~10 KB | Static document, 4KB buffer |
| Packetizer | ~5 KB | One active protocol |
| Application logic | ~20 KB | Command dispatch, state |
| **Total** | **~135 KB** | Leaves 375KB headroom |

### 4.4 Boot Sequence

```
1. Init GPIO (E-stop button, relays to safe state, status LED)
2. Init watchdog timer (8-second timeout)
3. Load config from NVS (WiFi creds, Redis IP, instance ID, device profile)
4. Connect WiFi (retry with backoff, status LED blinks)
5. Connect Redis (retry with backoff)
6. Set presence key: SET device:{instance}:alive EX 90
7. Publish to events:heartbeat (service.started equivalent)
8. Start reading command stream: XREAD BLOCK 1000 STREAMS commands:{instance} $
9. Start heartbeat task (30s interval, refreshes presence key)
10. Start watchdog feed task
11. Main loop: read command -> execute on device -> publish response -> XACK
```

### 4.5 Command Execution Flow on ESP32

```
XREAD command from commands:{instance}
  |
  v
Parse JSON envelope (ArduinoJson)
  |
  v
Validate: correct device_id? known command? timeout sane?
  |
  v
Packetizer.pack(command, parameters) -> raw bytes
  |
  v
Send to device (TCP write / UART write / GPIO set)
  |
  v
Wait for response (with timeout from payload.timeout_ms)
  |
  v
Packetizer.unpack(raw_response) -> string
  |
  v
Build response envelope (correlation_id from request)
  |
  v
XADD to reply_to stream
  |
  v
XACK the command (mark as processed)
```

---

## 5. Go Server Architecture

### 5.1 Two Processes, Not Thirty-Nine

```
┌──────────────────────────────────────────┐
│ arturo-server                            │
│                                          │
│  Device Registry     REST API            │
│  Health Monitor      WebSocket           │
│  Config Manager      Data Store (SQLite) │
│  E-Stop Coordinator  Report Generator    │
│  Metrics Collector                       │
└──────────────────────────────────────────┘

┌──────────────────────────────────────────┐
│ arturo-engine                            │
│                                          │
│  Script Parser (.art DSL)                │
│  Script Executor (sends commands via     │
│    Redis Streams to ESP32 nodes)         │
│  Variable System                         │
│  Test Result Aggregator                  │
└──────────────────────────────────────────┘

┌──────────────────────────────────────────┐
│ Redis                                    │
└──────────────────────────────────────────┘
```

Optional standalone tools (keep small, run when needed):
- `arturo-repl` - Redis message monitor (from original component 01 concept)
- `arturo-console` - Interactive device command CLI (from component 40 concept)

### 5.2 Device Registry (core new concept)

The server needs to know which ESP32 node owns which device:

```go
// Device registry maps device IDs to ESP32 instances
type DeviceEntry struct {
    DeviceID    string    // "fluke-8846a"
    NodeInstance string   // "dmm-station-01"
    CommandStream string  // "commands:dmm-station-01"
    Protocol    string    // "scpi"
    LastSeen    time.Time
    Status      string    // "online", "offline", "error"
}
```

When the script executor needs to send a command to `fluke-8846a`:
1. Look up `fluke-8846a` in registry -> `dmm-station-01`
2. XADD to `commands:dmm-station-01`
3. XREAD from `responses:orchestrator:orch-01` with correlation filter
4. Return result to script

ESP32 nodes self-register by including their device list in heartbeats.

### 5.3 Script Executor Adaptation

The script executor's device interaction changes from "call local Go function" to "send Redis Stream message and wait for response":

```go
// Old (direct local call):
result, err := tcpManager.SendCommand(deviceID, "MEAS:VOLT:DC?")

// New (via Redis to ESP32):
corrID := uuid.New()
cmd := buildCommandRequest(deviceID, "measure_dc_voltage", corrID)
node := registry.GetNode(deviceID)
redis.XAdd(ctx, node.CommandStream, cmd)
response := redis.XReadBlock(ctx, myResponseStream, corrID, timeout)
```

This is the single biggest change in the server code. Everything else (parser, variables, data storage, API) works the same.

---

## 6. Debugging and Observability (Built From Day One)

Debugging is not a phase. It's infrastructure that exists before the first command is sent.

### 6.1 `arturo-monitor` - The Eyes on the System

A standalone Go tool that shows everything flowing through Redis in real-time. Build this in Phase 1 alongside the first heartbeat.

```
$ arturo-monitor

┌─ STREAMS ──────────────────────────────────────────────────────────────────┐
│ 12:04:01.123 commands:relay-board-01     → device.command.request         │
│              corr=a1b2c3  cmd=set_relay  params={channel:3,state:on}      │
│ 12:04:01.187 responses:orchestrator:o1   ← device.command.response        │
│              corr=a1b2c3  success=true   duration=64ms                    │
│ 12:04:05.001 commands:dmm-station-01     → device.command.request         │
│              corr=d4e5f6  cmd=measure_dc_voltage  timeout=5000ms          │
│ 12:04:05.048 responses:orchestrator:o1   ← device.command.response        │
│              corr=d4e5f6  success=true   response="1.23456789" dur=47ms   │
├─ PUB/SUB ──────────────────────────────────────────────────────────────────┤
│ 12:04:00.000 events:heartbeat            relay-board-01    up=3600s       │
│              devices=[relay-8ch] heap=245KB rssi=-42dBm                   │
│ 12:04:00.012 events:heartbeat            dmm-station-01    up=7200s       │
│              devices=[fluke-8846a] heap=310KB rssi=-38dBm                 │
│ 12:04:30.001 events:heartbeat            relay-board-01    up=3630s       │
├─ PRESENCE ─────────────────────────────────────────────────────────────────┤
│ device:relay-board-01:alive    TTL=87s  ✓ online                          │
│ device:dmm-station-01:alive    TTL=62s  ✓ online                          │
│ device:serial-bridge-01:alive  TTL=0s   ✗ OFFLINE (last seen 92s ago)    │
└────────────────────────────────────────────────────────────────────────────┘
```

**Features:**
- Tails all command/response Streams simultaneously (`XREAD BLOCK` on all `commands:*` and `responses:*`)
- Subscribes to all Pub/Sub channels (`PSUBSCRIBE events:*`)
- Polls presence keys (`SCAN` + `TTL` on `device:*:alive`)
- Color-coded: green=success, red=error/offline, yellow=warning/slow, cyan=heartbeat
- Correlates requests with responses (matches `correlation_id`, shows round-trip time)
- Flags anomalies: missing responses (timeout), unexpected message types, malformed JSON
- Filterable: `arturo-monitor --node relay-board-01` or `arturo-monitor --type heartbeat`

**Modes:**
```bash
arturo-monitor                          # Everything, default view
arturo-monitor --streams                # Command/response streams only
arturo-monitor --pubsub                 # Pub/Sub only
arturo-monitor --presence               # Presence keys only
arturo-monitor --node dmm-station-01    # Filter to one ESP32
arturo-monitor --type device.command.*  # Filter by message type
arturo-monitor --corr a1b2c3            # Track one correlation chain
arturo-monitor --json                   # Raw JSON output (pipe to jq)
arturo-monitor --log /tmp/debug.jsonl   # Log all messages to file (JSONL)
```

**This is ~300-400 lines of Go.** It's the single most valuable debugging tool in the system.

### 6.2 ESP32 Serial Debug Output

Every ESP32 prints structured debug logs to its USB serial port (115200 baud). Connect a USB cable, open a terminal, see everything:

```
[12:04:00.000] [WIFI] Connected to SSID "arturo-lab" rssi=-42
[12:04:00.150] [REDIS] Connected to 192.168.1.10:6379 as relay-board-01
[12:04:00.155] [REDIS] SET device:relay-board-01:alive EX 90
[12:04:00.160] [REDIS] XREAD BLOCK 1000 STREAMS commands:relay-board-01 $
[12:04:00.170] [HEARTBEAT] Published heartbeat #1 heap=245KB
[12:04:01.123] [CMD] Received: device.command.request corr=a1b2c3
[12:04:01.124] [CMD]   device=relay-8ch command=set_relay params={ch:3,on}
[12:04:01.125] [RELAY] GPIO 17 -> HIGH (channel 3 ON)
[12:04:01.126] [CMD] Response: success=true duration=2ms
[12:04:01.130] [REDIS] XADD responses:orchestrator:o1 ...
[12:04:01.135] [REDIS] XACK commands:relay-board-01 ...
[12:04:30.000] [HEARTBEAT] Published heartbeat #2 heap=244KB
[12:04:45.000] [WIFI] *** DISCONNECTED *** reason=beacon_timeout
[12:04:45.500] [WIFI] Reconnecting attempt 1/10...
[12:04:46.200] [WIFI] Connected rssi=-51
[12:04:46.350] [REDIS] Reconnected
```

**Debug levels (compile-time flag in config.h):**
```cpp
#define DEBUG_LEVEL_NONE   0   // Production: no serial output
#define DEBUG_LEVEL_ERROR  1   // Errors only
#define DEBUG_LEVEL_INFO   2   // Lifecycle + commands + errors
#define DEBUG_LEVEL_DEBUG  3   // Everything including raw bytes
#define DEBUG_LEVEL_TRACE  4   // Hex dumps of SCPI/Modbus traffic

#define DEBUG_LEVEL DEBUG_LEVEL_INFO  // Set per build
```

At `DEBUG_LEVEL_TRACE`, you see the raw wire traffic:

```
[12:04:05.001] [CMD] Received: device.command.request corr=d4e5f6
[12:04:05.002] [SCPI] TX >>> "MEAS:VOLT:DC?\n" (15 bytes)
[12:04:05.002] [SCPI] TX hex: 4d 45 41 53 3a 56 4f 4c 54 3a 44 43 3f 0a
[12:04:05.045] [SCPI] RX <<< "+1.23456789E+00\n" (17 bytes)
[12:04:05.045] [SCPI] RX hex: 2b 31 2e 32 33 34 35 36 37 38 39 45 2b 30 30 0a
[12:04:05.046] [SCPI] Parsed: "1.23456789"
```

### 6.3 Redis CLI Cheat Sheet

No custom tools needed for quick checks:

```bash
# === LIVE MONITORING ===

# Watch all Pub/Sub messages
redis-cli PSUBSCRIBE "events:*"

# Tail a specific command stream (from the beginning)
redis-cli XREAD BLOCK 0 STREAMS commands:relay-board-01 0

# Tail a stream from "now" (only new messages)
redis-cli XREAD BLOCK 0 STREAMS commands:relay-board-01 $

# Tail ALL command streams at once
redis-cli XREAD BLOCK 0 STREAMS \
  commands:relay-board-01 \
  commands:dmm-station-01 \
  commands:serial-bridge-01 \
  $ $ $

# Watch all responses
redis-cli XREAD BLOCK 0 STREAMS responses:orchestrator:orch-01 $

# Nuclear option: see EVERY Redis command from EVERY client
redis-cli MONITOR

# === INSPECTION ===

# Check which ESP32s are alive
redis-cli KEYS "device:*:alive"

# Check TTL on a presence key
redis-cli TTL device:relay-board-01:alive

# Read last 10 commands sent to a node
redis-cli XREVRANGE commands:relay-board-01 + - COUNT 10

# Read last 10 responses
redis-cli XREVRANGE responses:orchestrator:orch-01 + - COUNT 10

# Count messages in a stream
redis-cli XLEN commands:relay-board-01

# Get stream info (first/last ID, consumer groups)
redis-cli XINFO STREAM commands:relay-board-01

# List all consumer groups on a stream
redis-cli XINFO GROUPS commands:relay-board-01

# Check pending (unacknowledged) messages
redis-cli XPENDING commands:relay-board-01 esp32-group - + 10

# === MANUAL TESTING (inject commands by hand) ===

# Send a command to an ESP32 manually
redis-cli XADD commands:relay-board-01 '*' \
  message '{"envelope":{"id":"test-001","timestamp":"2026-02-17T12:00:00.000Z","source":{"service":"manual","instance":"cli","version":"0.0.0"},"schema_version":"v1","type":"device.command.request","correlation_id":"manual-test-001","reply_to":"responses:manual:cli"},"payload":{"device_id":"relay-8ch","command":"set_relay","parameters":{"channel":"3","state":"on"},"timeout_ms":5000}}'

# Then read the response
redis-cli XREAD BLOCK 5000 STREAMS responses:manual:cli $

# Trigger a manual emergency stop
redis-cli PUBLISH events:emergency_stop \
  '{"envelope":{"id":"estop-manual","timestamp":"2026-02-17T12:00:00.000Z","source":{"service":"manual","instance":"cli","version":"0.0.0"},"schema_version":"v1","type":"system.emergency_stop"},"payload":{"reason":"manual_test","triggered_by":"cli","severity":"critical"}}'
```

### 6.4 Message Logging to Disk

`arturo-monitor --log /var/log/arturo/messages.jsonl` writes every message as one JSON object per line:

```json
{"ts":"2026-02-17T12:04:01.123Z","channel":"commands:relay-board-01","redis_type":"stream","stream_id":"1708171441123-0","message":{...}}
{"ts":"2026-02-17T12:04:01.187Z","channel":"responses:orchestrator:o1","redis_type":"stream","stream_id":"1708171441187-0","message":{...}}
{"ts":"2026-02-17T12:04:30.001Z","channel":"events:heartbeat","redis_type":"pubsub","message":{...}}
```

JSONL format so you can:
```bash
# Search for a correlation chain
grep "a1b2c3" /var/log/arturo/messages.jsonl

# Find all errors
grep '"success":false' /var/log/arturo/messages.jsonl | jq .

# Find all commands to a specific device
grep '"device_id":"fluke-8846a"' /var/log/arturo/messages.jsonl | jq .

# Count messages per ESP32 node per hour
grep "commands:" /var/log/arturo/messages.jsonl | \
  jq -r '.channel' | sort | uniq -c | sort -rn

# Replay a command (copy from log, pipe to redis-cli)
grep "corr-456" /var/log/arturo/messages.jsonl | head -1 | \
  jq -r '.message' | redis-cli -x XADD commands:relay-board-01 '*' message
```

### 6.5 ESP32 Health Diagnostics

Each ESP32 heartbeat includes diagnostic fields the server and monitor display:

```json
{
  "payload": {
    "status": "running",
    "uptime_seconds": 3600,
    "devices": ["fluke-8846a"],
    "free_heap": 245000,
    "min_free_heap": 180000,
    "wifi_rssi": -42,
    "wifi_reconnects": 0,
    "redis_reconnects": 0,
    "commands_processed": 1547,
    "commands_failed": 3,
    "last_error": "E_DEVICE_TIMEOUT on fluke-8846a at 12:03:45",
    "watchdog_resets": 0
  }
}
```

`arturo-monitor` flags warning thresholds:
- `free_heap < 50KB` -> yellow warning
- `free_heap < 20KB` -> red alert
- `wifi_rssi < -70dBm` -> yellow
- `wifi_rssi < -80dBm` -> red
- `wifi_reconnects > 0` since last heartbeat -> yellow
- `commands_failed > 0` since last heartbeat -> yellow
- `watchdog_resets > 0` -> red alert
- Heartbeat missing for > 60s -> red OFFLINE
- Heartbeat missing for > 90s -> presence key expires, removed from registry

### 6.6 Debug Build vs Production Build

| Feature | Debug Build | Production Build |
|---------|-------------|-----------------|
| Serial debug output | Yes (configurable level) | Disabled |
| WiFi/Redis event logging | Verbose | Errors only |
| Raw protocol hex dumps | Available at TRACE level | Disabled |
| Heartbeat diagnostics | Full (heap, RSSI, counters, last_error) | Full (same - always useful) |
| OTA update | Enabled | Enabled |
| Watchdog | 8s timeout | 8s timeout |
| ArduinoJson buffer | 4KB (with overflow check + serial warning) | 4KB (silent truncate) |

### 6.7 When Things Go Wrong - Diagnostic Checklist

**ESP32 not appearing in monitor:**
1. Check USB serial output - is WiFi connected?
2. `redis-cli KEYS "device:*:alive"` - is presence key set?
3. `redis-cli PSUBSCRIBE "events:heartbeat"` - are heartbeats arriving?
4. Check Redis ACL - can this user PUBLISH to `events:heartbeat`?

**Command sent but no response:**
1. `arturo-monitor --corr <id>` - did the command reach the stream?
2. `redis-cli XLEN commands:<node>` - is the stream growing?
3. `redis-cli XPENDING commands:<node> esp32-group` - is the message pending (read but not ACKed)?
4. Check ESP32 serial output - did it receive and parse the command?
5. Check ESP32 serial output - did the device respond?
6. Check ESP32 serial output at TRACE level - what went over the wire?

**ESP32 going offline intermittently:**
1. Check `wifi_rssi` in heartbeats - signal strength trending down?
2. Check `wifi_reconnects` counter - how often?
3. Check `free_heap` trend - memory leak?
4. Check `watchdog_resets` - firmware hanging?
5. Check Redis `XPENDING` - commands piling up unacknowledged?

**Messages arriving but malformed:**
1. `arturo-monitor --json | jq .` - parse the raw JSON
2. Validate against schema: `check-jsonschema --schemafile schemas/device.command.request.v1.json message.json`
3. Check ESP32 `DEBUG_LEVEL_TRACE` for raw bytes - encoding issue?

---

## 7. Build Phases

### Phase 0: Freeze Contracts (Week 1)

**Do not write any code until this is done.**

Deliverables:
1. JSON Schema files for the 4 message types
2. Redis channel/stream naming document
3. Redis ACL definitions for each ESP32 role
4. Device profile YAML for your first instrument

Validation: Review schemas, manually craft sample messages, verify they parse correctly.

### Phase 1: First Heartbeat + Monitor Tool (Weeks 2-3)

**Goal:** ESP32 connects to WiFi, connects to Redis, sends heartbeats. You can see everything.

ESP32:
- WiFi manager with reconnect
- Minimal Redis client (PUBLISH, SET with EX)
- Heartbeat publisher with full diagnostics (heap, RSSI, uptime)
- Status LED (blinking = connecting, solid = connected)
- Serial debug output at INFO level from day one

Server:
- **Build `arturo-monitor` first** (before any other server code)
- Verify ESP32 heartbeats appear in monitor with color coding
- Verify presence key TTL works, OFFLINE detection triggers

**The monitor is your "hello world", not the heartbeat.** If you can't see what's happening, you can't debug anything that comes after.

### Phase 2: First Command Round-Trip (Weeks 3-5)

**Goal:** Server sends a command, ESP32 executes it on a real instrument, response comes back. You see the full round-trip in the monitor.

ESP32:
- Redis Stream reader (XREAD BLOCK on `commands:{instance}`)
- Command handler + dispatcher
- SCPI packetizer (port from Go reference)
- TCP device client (connect to SCPI instrument)
- Response publisher (XADD to reply_to stream)
- Serial debug at TRACE level showing raw SCPI bytes on the wire

Server:
- Device registry (populated from heartbeats)
- Command sender (XADD to device's command stream)
- Response reader (XREAD on own response stream, match correlation_id)
- Simple CLI or HTTP endpoint to trigger a command
- `arturo-monitor` now shows: command sent -> response received, with correlation tracking and round-trip time
- Enable `--log` to JSONL file for post-mortem analysis

**Validate with a real DMM:** send `*IDN?`, get back the instrument identification string. Watch the entire flow in `arturo-monitor --corr <id>` while simultaneously watching raw SCPI bytes on the ESP32 serial console.

### Phase 3: Relay and Serial Variants (Weeks 5-7)

**Goal:** Additional ESP32 firmware variants for relay control and serial devices.

- Port relay controller (GPIO output, safety interlocks)
- Port serial device bridge (HardwareSerial + CTI/Modbus packetizer)
- Test with real hardware
- All three variants use the same messaging/heartbeat/safety modules

### Phase 4: Server Core (Weeks 7-10)

**Goal:** Consolidated Go server with REST API, WebSocket, data storage.

- REST API: device list, device status, send command, system status
- WebSocket: push heartbeats and command results to browser
- SQLite: store test results, measurements, device history
- Health monitor: track ESP32 heartbeats, alert on missing nodes
- E-stop coordinator: listen for E-stop events, broadcast to all, log

### Phase 5: Script Engine (Weeks 10-13)

**Goal:** Parse and execute .art scripts that control devices via ESP32 nodes.

- Reimplement Arturo DSL parser in Go (use original grammar as spec)
- Script executor routes device commands through Redis Streams
- Variable system for test parameters
- Test result aggregation and pass/fail reporting
- Run a real multi-step test script end-to-end

### Phase 6: Dashboard and Reports (Weeks 13-15)

**Goal:** Operator-facing UI.

- Vue.js or simple HTML dashboard showing:
  - ESP32 node status (online/offline, connected devices)
  - Active test progress
  - Recent measurements
  - E-stop status
- CSV export of test results
- PDF test report generation

### Phase 7: Hardening (Weeks 15-17)

- WiFi disconnect/reconnect under load
- Redis connection loss and recovery
- Power failure recovery (ESP32 boots to safe state)
- Watchdog timer verification
- E-stop end-to-end test (button press -> all nodes stop)
- OTA firmware update mechanism
- Documentation

---

## 7. What NOT to Build

Lessons from the original 39-component system:

| Don't Build | Why |
|-------------|-----|
| Process supervisor (systemg) | Use systemd for 2 Go processes |
| Separate WebSocket service | Build into arturo-server |
| Separate REST API service | Build into arturo-server |
| Separate health check service | Build into arturo-server |
| Separate config management service | Build into arturo-server |
| Separate auth service | Simple middleware, not a service |
| Separate metrics service | Prometheus endpoint, not a service |
| Separate file operations service | Standard library calls |
| Separate backup service | Cron job + sqlite3 .backup |
| Discord/Slack integration | Add later if actually needed |
| MCP server | Add later if actually needed |
| Plugin manager | YAGNI |
| Script synchronization service | cp + rsync |
| Communication hub | Redis IS the communication hub |
| 14 other services | Just don't |

**Rule: If it can be a function call inside arturo-server, it's not a service.**

---

## 8. Risk Register

| Risk | Impact | Likelihood | Mitigation |
|------|--------|-----------|------------|
| ESP32 JSON parsing too slow/big | Medium | Low | ArduinoJson v7 with static allocation; messages are small (~500 bytes) |
| WiFi drops during test | High | Medium | Streams persist; ESP32 reconnects and resumes reading; server detects timeout and pauses test |
| Redis single point of failure | Medium | Low | Redis persistence (AOF); for v1 this is acceptable |
| Learning Go slows progress | Medium | Medium | Go is small and C-like; budget 1-2 weeks for ramp-up |
| Scope creep back to 39 services | High | Medium | Enforce "function, not service" rule; resist premature abstraction |
| Serial timing issues on ESP32 | Low | Low | ESP32 has hardware UART with FIFO; better than USB-serial adapters |
| E-stop latency over WiFi | High | Low | Local GPIO E-stop acts instantly; Redis notification is secondary |

---

## 9. Concrete First Week

Day 1-2:
- Write JSON Schema for `device.command.request` and `device.command.response`
- Write JSON Schema for `service.heartbeat` and `system.emergency_stop`
- Define Redis Stream and Pub/Sub channel names
- Write Redis ACL config file

Day 3:
- Set up PlatformIO project for ESP32-S3
- Implement WiFi manager with reconnect
- Blink an LED

Day 4-5:
- Implement minimal Redis client on ESP32 (PUBLISH + SET + SUBSCRIBE)
- Implement serial debug output on ESP32 (structured, leveled, timestamped)
- Publish first heartbeat message
- Build `arturo-monitor` v0.1 (subscribe to `events:*`, print with timestamps + color)
- Verify heartbeat appears in monitor AND in ESP32 serial output
- Verify the JSON matches your schema
- Test: unplug ESP32, watch monitor show OFFLINE after TTL expires. Replug, watch it come back.

**If you can see the heartbeat in the monitor and the serial console simultaneously, you have the foundation. Everything else is building on top of it.**

---

## 10. Reference Material from Original Codebase

These files from the original Arturo repo are worth reading as design references (not code to copy):

| File | What to Extract |
|------|----------------|
| `docs/architecture/MESSAGING_PROTOCOL_V2.md` | Full protocol spec (trim for v1) |
| `src/common/protocols/scpi.go` | SCPI packetizer logic to port to C++ |
| `src/common/protocols/modbus.go` | Modbus RTU logic to port to C++ |
| `src/common/protocols/cti.go` | CTI pump protocol to port to C++ |
| `src/common/profiles/devices/*.yaml` | Device profile definitions |
| `src/domains/automation/19_script_parser/` | Arturo language grammar reference |
| `src/domains/devices/05_tcp_manager_service/main.go` | TCP device communication patterns |
| `src/domains/devices/06_serial_manager_service/main.go` | Serial device communication patterns |
| `src/domains/devices/07_relay_manager_service/` | Relay control + safety interlock logic |
| `src/domains/system/27_emergency_stop_system/` | E-stop architecture |
| `schemas/*.json` | Message schema definitions |

---

*This is a build plan, not a migration plan. Start clean. Build small. Validate each layer before adding the next.*
