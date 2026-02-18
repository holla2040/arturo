# Arturo

Industrial test automation system built on ESP32 field devices and a centralized Go server, connected via Redis.

## Architecture

```
ESP32 field nodes (up to 6)  <--Redis Streams/PubSub-->  Ubuntu Server (Go)
       C++ / Arduino                                          Go
  - SCPI instruments                                   - Test orchestration
  - Serial devices                                     - Script engine (.art DSL)
  - Relay control                                      - REST API + WebSocket
  - Modbus devices                                     - SQLite data storage
  - Safety interlocks                                  - Web dashboard
```

## Project Structure

```
arturo/
├── docs/
│   ├── architecture/
│   │   └── MIGRATION_PLAN.md           # Full build plan and architecture decisions
│   └── reference/                      # Reference material from arturo-go-archive
├── schemas/                            # Protocol v1.0.0 message schemas
│   └── v1.0.0/
│       ├── envelope/                   # Shared message envelope
│       ├── error/                      # Shared error object
│       ├── device-command-request/     # Server -> ESP32 command
│       ├── device-command-response/    # ESP32 -> Server result
│       ├── service-heartbeat/          # ESP32 health report (30s interval)
│       ├── system-emergency-stop/      # E-stop broadcast
│       └── system-ota-request/         # OTA firmware update
├── server/                             # Go server
│   └── cmd/
│       ├── arturo-server/              # Main server (API, device registry, data, health)
│       ├── arturo-engine/              # Script parser + executor
│       └── arturo-monitor/             # Redis traffic monitor (debugging)
├── firmware/                           # ESP32 Arduino project
│   ├── README.md                       # Firmware architecture decisions
│   ├── src/
│   │   ├── network/                    # WiFi, Redis client
│   │   ├── messaging/                  # Protocol v1.0.0 envelope, command handler, heartbeat
│   │   ├── protocols/                  # SCPI, Modbus, CTI, ASCII packetizers
│   │   ├── devices/                    # TCP, serial, relay, modbus device drivers
│   │   └── safety/                     # Watchdog, E-stop, interlocks
│   └── test/
└── profiles/                           # Device profile YAMLs
```

## Key Decisions

- **Go** on server, **C++** on ESP32, **Redis** in the middle
- **Arduino framework** with Arduino CLI (not IDE), FreeRTOS tasks for concurrency
- **Redis Streams** for reliable command/response delivery (per-device channels)
- **Redis Pub/Sub** for heartbeats and emergency stop (fire-and-forget)
- **Protocol v1.0.0 envelope** on every message (JSON, same format on ESP32 and server)
- **5 message types**: `device.command.request`, `device.command.response`, `service.heartbeat`, `system.emergency_stop`, `system.ota.request`
- **Direct ESP32-to-Redis** connection (no MQTT broker, no middleware)
- **2 server processes** (not 39)
- **OTA firmware updates** via ESP-IDF dual-partition with automatic rollback
- **Schemas as single source of truth** — code implements what the schemas define

## Message Flow

```
Server                          Redis                         ESP32
  │                               │                             │
  │── XADD commands:node-01 ─────>│                             │
  │                               │──── XREAD BLOCK ───────────>│
  │                               │                             │── execute on device
  │                               │<──── XADD responses:orch ───│
  │<── XREAD ────────────────────-│                             │
  │                               │                             │
  │                               │<── PUBLISH events:heartbeat │  (every 30s)
  │<── SUBSCRIBE ────────────────-│                             │
```

## Getting Started

1. Read [schemas/v1.0.0/README.md](schemas/v1.0.0/README.md) — the protocol definitions
2. Read [firmware/README.md](firmware/README.md) — ESP32 architecture decisions
3. Read [docs/architecture/MIGRATION_PLAN.md](docs/architecture/MIGRATION_PLAN.md) — full build plan

## Related

- [arturo-go-archive](https://github.com/holla2040/arturo-go-archive) — Original Go microservices implementation (reference only)
