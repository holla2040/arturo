# Arturo Services

Go services that run on the controller machine. These are the processes that must be running for the system to operate.

## Services

| Service | What it does | Runs |
|---------|-------------|------|
| **arturo-controller** | REST API, WebSocket, device registry, health monitor, E-stop coordinator, SQLite data store | Always |
| **arturo-console** | Spawns mock stations with simulated pumps, serves web UI for control and Redis monitoring | Development |
| **arturo-terminal** | Operator web UI; serves HTML and reverse-proxies API/WebSocket to the controller | Always |

## Build

```bash
cd services
go build -o arturo-controller ./cmd/arturo-controller
go build -o arturo-console ./cmd/arturo-console
go build -o arturo-terminal ./cmd/arturo-terminal
```

## Run

```bash
# Controller (required)
./arturo-controller -redis localhost:6379 -listen :8080 -db arturo.db

# Monitor (debugging)
./arturo-monitor                            # everything
./arturo-monitor --station dmm-station-01   # filter to one station
./arturo-monitor --json                     # raw JSON output
```

## Go Module

Module path: `github.com/holla2040/arturo`

The module root is this directory. All import paths use `github.com/holla2040/arturo/internal/...`.

## Source Structure

```
services/
├── cmd/
│   ├── arturo-controller/       # Main service entry point
│   ├── arturo-console/          # Mock stations + web console entry point
│   └── arturo-terminal/         # Operator web UI entry point
├── internal/
│   ├── api/                     # REST API handlers, WebSocket hub, response dispatcher
│   ├── artifact/                # Test artifact storage (PDF reports, data files)
│   ├── console/                 # Mock station web console
│   ├── terminal/                # Operator web UI (reverse proxy to controller)
│   ├── estop/                   # Emergency stop coordinator
│   ├── mockpump/                # Mock CTI cryopump simulator
│   ├── protocol/                # Protocol v1.0.0 envelope builder/parser
│   ├── redishealth/             # Redis connection health monitor
│   ├── registry/                # Device registry (maps devices to stations)
│   ├── report/                  # Test report generation
│   ├── script/                  # Script engine
│   │   ├── ast/                 #   Abstract syntax tree
│   │   ├── executor/            #   Script executor (sends commands via Redis)
│   │   ├── lexer/               #   Tokenizer
│   │   ├── parser/              #   .art DSL parser
│   │   ├── redisrouter/         #   Routes script commands to stations via Redis
│   │   └── validate/            #   Parse-only validation (no hardware)
│   ├── store/                   # SQLite data store (test results, device events)
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
