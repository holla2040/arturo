# Arturo Services

Go services that run on the controller machine. These are the processes that must be running for the system to operate.

## Services

| Service | What it does | Runs |
|---------|-------------|------|
| **controller** | REST API, WebSocket, device registry, health monitor, E-stop coordinator, SQLite data store | Always |
| **console** | Spawns mock stations with simulated pumps, serves web UI for control and Redis monitoring | Development |
| **terminal** | Operator web UI; serves HTML and reverse-proxies API/WebSocket to the controller | Always |

## Build

```bash
cd services
go build -o controller ./cmd/arturo-controller
go build -o console ./cmd/arturo-console
go build -o terminal ./cmd/arturo-terminal
```

## Run

```bash
# Controller (required)
./controller -redis localhost:6379 -listen :8002 -db arturo.db

# Terminal (operator UI, proxies to controller)
./terminal -listen :8000 -controller http://localhost:8002
./terminal -listen :8000 -controller http://localhost:8002 -dev  # live reload from disk

# Console (mock stations for development)
./console -stations 1,2,3,4                    # mock all four stations (default)
./console -stations 2,3,4                      # mock 2-4, leave 1 for real hardware
./console -stations 1 -cooldown-hours 2.0      # single station, faster cooling
./console -stations 1,2 -fail-rate 0.1         # 10% random command failure
```

## Go Module

Module path: `github.com/holla2040/arturo`

The module root is this directory. All import paths use `github.com/holla2040/arturo/internal/...`.

## Source Structure

```
services/
├── cmd/
│   ├── arturo-controller/       # Controller entry point
│   ├── arturo-console/          # Console entry point
│   └── arturo-terminal/         # Terminal entry point
├── internal/
│   ├── api/                     # REST API handlers, WebSocket hub, response dispatcher
│   ├── artifact/                # Test artifact storage (PDF reports, data files)
│   ├── console/                 # Mock station web console
│   ├── estop/                   # Emergency stop coordinator
│   ├── mockpump/                # Mock CTI cryopump simulator
│   ├── poller/                  # Polling utilities
│   ├── protocol/                # Protocol v1.0.0 envelope builder/parser
│   ├── redishealth/             # Redis connection health monitor
│   ├── registry/                # Device registry (maps devices to stations)
│   ├── report/                  # Test report generation
│   ├── script/                  # Script engine
│   │   ├── ast/                 #   Abstract syntax tree
│   │   ├── executor/            #   Script executor (sends commands via Redis)
│   │   ├── lexer/               #   Tokenizer
│   │   ├── parser/              #   .art DSL parser
│   │   ├── profile/             #   Device profile loader
│   │   ├── redisrouter/         #   Routes script commands to stations via Redis
│   │   ├── result/              #   Test result types
│   │   ├── token/               #   Token types
│   │   ├── validate/            #   Parse-only validation (no hardware)
│   │   └── variable/            #   Script variable handling
│   ├── store/                   # SQLite data store (test results, device events)
│   ├── terminal/                # Operator web UI (reverse proxy to controller)
│   └── testmanager/             # Test lifecycle (start, pause, stop, artifacts)
│
├── go.mod
└── go.sum
```

## Test

```bash
cd services
go test ./...
```
