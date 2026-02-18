# Arturo

Industrial test automation system built on ESP32 field devices and a centralized Go server, connected via Redis.

## Architecture

```
ESP32 field nodes (up to 6)  <--Redis Streams/PubSub-->  Ubuntu Server (Go)
       C++                                                    Go
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
│   │   └── MIGRATION_PLAN.md       # Full build plan and architecture decisions
│   └── reference/                  # Reference material from arturo-go-archive
├── schemas/                        # JSON Schema for message types (Phase 0)
├── server/                         # Go server (arturo-server, arturo-engine, arturo-monitor)
│   └── cmd/
│       ├── arturo-server/          # Main server (API, device registry, data, health)
│       ├── arturo-engine/          # Script parser + executor
│       └── arturo-monitor/         # Redis traffic monitor (debugging)
├── firmware/                       # ESP32 PlatformIO project
│   ├── src/
│   │   ├── network/                # WiFi, Redis client
│   │   ├── messaging/              # Protocol v2 envelope, command handler, heartbeat
│   │   ├── protocols/              # SCPI, Modbus, CTI, ASCII packetizers
│   │   ├── devices/                # TCP, serial, relay, modbus device drivers
│   │   └── safety/                 # Watchdog, E-stop, interlocks
│   └── test/
└── profiles/                       # Device profile YAMLs
```

## Key Decisions

- **Redis Streams** for reliable command/response delivery
- **Redis Pub/Sub** for heartbeats and emergency stop
- **Protocol v2 envelope** on every message (JSON, same format on ESP32 and server)
- **4 initial message types**: `device.command.request`, `device.command.response`, `service.heartbeat`, `system.emergency_stop`
- **Direct ESP32-to-Redis** connection (no MQTT broker, no middleware)
- **2 server processes** (not 39)

## Getting Started

See [docs/architecture/MIGRATION_PLAN.md](docs/architecture/MIGRATION_PLAN.md) for the full build plan, phasing, and architecture details.

## Related

- [arturo-go-archive](https://github.com/holla2040/arturo-go-archive) - Original Go microservices implementation (reference only)
