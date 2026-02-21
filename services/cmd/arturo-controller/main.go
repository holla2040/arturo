package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/holla2040/arturo/internal/api"
	"github.com/holla2040/arturo/internal/estop"
	"github.com/holla2040/arturo/internal/poller"
	"github.com/holla2040/arturo/internal/protocol"
	"github.com/holla2040/arturo/internal/redishealth"
	"github.com/holla2040/arturo/internal/registry"
	"github.com/holla2040/arturo/internal/store"
	"github.com/holla2040/arturo/internal/testmanager"
	"github.com/redis/go-redis/v9"
)

const serverVersion = "1.0.0"

var serverSource = protocol.Source{
	Service:  "arturo_controller",
	Instance: "ctrl-01",
	Version:  serverVersion,
}

func main() {
	// If first arg is "send", run the legacy CLI sender
	if len(os.Args) > 1 && os.Args[1] == "send" {
		runSendCommand()
		return
	}

	// Server mode
	redisAddr := flag.String("redis", "localhost:6379", "Redis address")
	listenAddr := flag.String("listen", ":8002", "HTTP listen address")
	dbPath := flag.String("db", "arturo.db", "SQLite database path")
	flag.Parse()

	// Context for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Initialize Redis
	rdb := redis.NewClient(&redis.Options{Addr: *redisAddr})
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis at %s: %v", *redisAddr, err)
	}
	log.Printf("Connected to Redis at %s", *redisAddr)

	// Initialize SQLite store
	db, err := store.New(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open database at %s: %v", *dbPath, err)
	}
	defer db.Close()
	log.Printf("Opened database at %s", *dbPath)

	// Initialize components
	reg := registry.New()
	dispatcher := api.NewResponseDispatcher()
	wsHub := api.NewHub()

	// Test manager for test lifecycle
	testMgr := testmanager.New(ctx, db, wsHub, rdb, serverSource)
	log.Println("Test manager initialized")

	// E-stop coordinator with callback to broadcast via WebSocket and stop all tests
	estopCoord := estop.New(func(state estop.State) {
		wsHub.BroadcastEvent("estop", state)
		db.RecordDeviceEvent("system", "controller", "emergency_stop", state.Reason)
		testMgr.EmergencyStopAll()
	})

	// Redis command sender
	sender := &redisCommandSender{rdb: rdb}

	// Redis health monitor
	redisMon := redishealth.New(rdb,
		redishealth.WithInterval(5*time.Second),
		redishealth.WithOnDown(func() {
			log.Println("Redis connection lost — API commands will return 503")
			wsHub.BroadcastEvent("redis_health", map[string]string{"status": "disconnected"})
		}),
		redishealth.WithOnUp(func() {
			log.Println("Redis connection restored — API commands available")
			wsHub.BroadcastEvent("redis_health", map[string]string{"status": "connected"})
		}),
	)

	// HTTP handler
	handler := &api.Handler{
		Registry:    reg,
		Store:       db,
		Estop:       estopCoord,
		Dispatcher:  dispatcher,
		Sender:      sender,
		Source:      serverSource,
		RedisHealth: redisMon,
		TestMgr:     testMgr,
	}

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	mux.HandleFunc("GET /ws", wsHub.HandleWebSocket)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"service":"arturo-controller","version":"` + serverVersion + `"}`))
	})

	server := &http.Server{
		Addr:    *listenAddr,
		Handler: mux,
	}

	var wg sync.WaitGroup

	// 1. Heartbeat listener
	wg.Add(1)
	go func() {
		defer wg.Done()
		runHeartbeatListener(ctx, rdb, reg, wsHub, testMgr)
	}()

	// 2. E-stop listener
	wg.Add(1)
	go func() {
		defer wg.Done()
		runEstopListener(ctx, rdb, estopCoord, wsHub)
	}()

	// 3. Response listener
	wg.Add(1)
	go func() {
		defer wg.Done()
		runResponseListener(ctx, rdb, dispatcher, wsHub)
	}()

	// 4. Health check ticker
	wg.Add(1)
	go func() {
		defer wg.Done()
		runHealthChecker(ctx, reg, wsHub, testMgr)
	}()

	// 5. WebSocket hub
	wg.Add(1)
	go func() {
		defer wg.Done()
		wsHub.Run(ctx)
	}()

	// 6. Redis health monitor
	wg.Add(1)
	go func() {
		defer wg.Done()
		redisMon.Run(ctx)
	}()

	// 7. HTTP server
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Printf("HTTP server listening on %s", *listenAddr)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// 8. Station poller (background status + temperature queries)
	wg.Add(1)
	go func() {
		defer wg.Done()
		stationPoller := poller.New(serverSource, sender, dispatcher, reg, wsHub, db)
		stationPoller.Run(ctx)
	}()

	// 9. Temperature log pruner (12-hour rolling window)
	wg.Add(1)
	go func() {
		defer wg.Done()
		cutoff := 12 * time.Hour

		// Prune immediately on startup
		if count, err := db.PruneTemperatureLog(time.Now().Add(-cutoff)); err != nil {
			log.Printf("temperature pruner: startup error: %v", err)
		} else if count > 0 {
			log.Printf("temperature pruner: startup pruned %d rows", count)
		}

		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if count, err := db.PruneTemperatureLog(time.Now().Add(-cutoff)); err != nil {
					log.Printf("temperature pruner: %v", err)
				} else if count > 0 {
					log.Printf("temperature pruner: pruned %d rows", count)
				}
			}
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	log.Println("Shutting down...")

	// Graceful HTTP shutdown with 5s timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(shutdownCtx)

	wg.Wait()
	log.Println("Shutdown complete")
}

// runHeartbeatListener subscribes to heartbeat events and updates the registry.
// It automatically re-subscribes if the connection drops.
func runHeartbeatListener(ctx context.Context, rdb *redis.Client, reg *registry.Registry, hub *api.Hub, testMgr *testmanager.TestManager) {
	for {
		if ctx.Err() != nil {
			return
		}

		sub := rdb.Subscribe(ctx, "events:heartbeat")
		ch := sub.Channel()

		func() {
			defer sub.Close()
			for {
				select {
				case <-ctx.Done():
					return
				case msg, ok := <-ch:
					if !ok {
						log.Println("heartbeat: subscription channel closed, reconnecting...")
						return
					}
					parsed, err := protocol.Parse([]byte(msg.Payload))
					if err != nil {
						log.Printf("heartbeat: parse error: %v", err)
						continue
					}
					payload, err := protocol.ParseHeartbeat(parsed)
					if err != nil {
						log.Printf("heartbeat: payload error: %v", err)
						continue
					}
					reg.UpdateFromHeartbeat(parsed.Envelope.Source.Instance, payload)
					testMgr.HandleHeartbeat(parsed.Envelope.Source.Instance)
					hub.BroadcastEvent("heartbeat", payload)
				}
			}
		}()

		// Back off before retrying
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// runEstopListener subscribes to emergency stop events.
// It automatically re-subscribes if the connection drops.
func runEstopListener(ctx context.Context, rdb *redis.Client, coord *estop.Coordinator, hub *api.Hub) {
	for {
		if ctx.Err() != nil {
			return
		}

		sub := rdb.Subscribe(ctx, "events:emergency_stop")
		ch := sub.Channel()

		func() {
			defer sub.Close()
			for {
				select {
				case <-ctx.Done():
					return
				case msg, ok := <-ch:
					if !ok {
						log.Println("estop: subscription channel closed, reconnecting...")
						return
					}
					parsed, err := protocol.Parse([]byte(msg.Payload))
					if err != nil {
						log.Printf("estop: parse error: %v", err)
						continue
					}
					if err := coord.HandleMessage(parsed); err != nil {
						log.Printf("estop: handle error: %v", err)
					}
				}
			}
		}()

		// Back off before retrying
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// runResponseListener subscribes to the response channel for command responses.
// It automatically re-subscribes if the connection drops.
func runResponseListener(ctx context.Context, rdb *redis.Client, dispatcher *api.ResponseDispatcher, hub *api.Hub) {
	channel := "responses:" + serverSource.Instance

	for {
		if ctx.Err() != nil {
			return
		}

		sub := rdb.Subscribe(ctx, channel)
		ch := sub.Channel()

		func() {
			defer sub.Close()
			for {
				select {
				case <-ctx.Done():
					return
				case msg, ok := <-ch:
					if !ok {
						log.Println("response listener: subscription channel closed, reconnecting...")
						return
					}
					parsed, err := protocol.Parse([]byte(msg.Payload))
					if err != nil {
						log.Printf("response listener: parse error: %v", err)
						continue
					}
					dispatched := dispatcher.Dispatch(parsed)
					hub.BroadcastEvent("command_response", parsed.Payload)
					if !dispatched {
						log.Printf("response listener: no waiter for correlation_id=%s", parsed.Envelope.CorrelationID)
					}
				}
			}
		}()

		// Back off before retrying
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// runHealthChecker periodically checks station health.
func runHealthChecker(ctx context.Context, reg *registry.Registry, hub *api.Hub, testMgr *testmanager.TestManager) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			// Get stations that were online before health check
			stationsBefore := reg.ListStations()
			onlineBefore := make(map[string]bool)
			for _, s := range stationsBefore {
				if s.Status == "online" || s.Status == "stale" {
					onlineBefore[s.Instance] = true
				}
			}

			reg.RunHealthCheck(now)

			// Check which stations went offline
			stationsAfter := reg.ListStations()
			for _, s := range stationsAfter {
				if s.Status == "offline" && onlineBefore[s.Instance] {
					testMgr.HandleOffline(s.Instance)
				}
			}
		}
	}
}

// redisCommandSender implements api.CommandSender using Redis PUBLISH.
type redisCommandSender struct {
	rdb *redis.Client
}

func (s *redisCommandSender) SendCommand(ctx context.Context, channel string, msg *protocol.Message) error {
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal command: %w", err)
	}
	return s.rdb.Publish(ctx, channel, string(msgJSON)).Err()
}

// --- Legacy "send" subcommand ---

func runSendCommand() {
	sendFlags := flag.NewFlagSet("send", flag.ExitOnError)
	station := sendFlags.String("station", "", "station instance ID (required)")
	device := sendFlags.String("device", "", "device ID (required)")
	cmd := sendFlags.String("cmd", "", "command string (required)")
	timeout := sendFlags.Int("timeout", 5000, "timeout in milliseconds")
	redisAddr := sendFlags.String("redis", "localhost:6379", "Redis address")

	sendFlags.Parse(os.Args[2:])

	if *station == "" || *device == "" || *cmd == "" {
		fmt.Fprintf(os.Stderr, "Error: --station, --device, and --cmd are required\n")
		sendFlags.Usage()
		os.Exit(1)
	}

	source := protocol.Source{
		Service:  "arturo_controller",
		Instance: "server-01",
		Version:  serverVersion,
	}

	msg, err := protocol.BuildCommandRequest(source, *device, *cmd, nil, *timeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building command request: %v\n", err)
		os.Exit(1)
	}

	msgJSON, err := json.Marshal(msg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling message: %v\n", err)
		os.Exit(1)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: *redisAddr,
	})
	defer rdb.Close()

	ctx := context.Background()

	if err := rdb.Ping(ctx).Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to Redis at %s: %v\n", *redisAddr, err)
		os.Exit(1)
	}

	cmdChannel := "commands:" + *station
	if err := rdb.Publish(ctx, cmdChannel, string(msgJSON)).Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error sending command to %s: %v\n", cmdChannel, err)
		os.Exit(1)
	}

	fmt.Printf("Sent command to %s\n", cmdChannel)
	fmt.Printf("  device:         %s\n", *device)
	fmt.Printf("  command:        %s\n", *cmd)
	fmt.Printf("  correlation_id: %s\n", msg.Envelope.CorrelationID)
	fmt.Printf("  timeout:        %dms\n", *timeout)

	responseStream := msg.Envelope.ReplyTo
	timeoutDur := time.Duration(*timeout) * time.Millisecond

	resp, err := waitForResponse(ctx, rdb, responseStream, msg.Envelope.CorrelationID, timeoutDur)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}

	payload, err := protocol.ParseCommandResponse(resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError parsing response: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nResponse from %s\n", resp.Envelope.Source.Instance)
	fmt.Printf("  device:    %s\n", payload.DeviceID)
	fmt.Printf("  command:   %s\n", payload.CommandName)
	fmt.Printf("  success:   %v\n", payload.Success)

	if payload.Response != nil {
		fmt.Printf("  response:  %s\n", *payload.Response)
	}
	if payload.DurationMs != nil {
		fmt.Printf("  duration:  %dms\n", *payload.DurationMs)
	}
	if payload.Error != nil {
		fmt.Printf("  error:     [%s] %s\n", payload.Error.Code, payload.Error.Message)
	}

	if !payload.Success {
		os.Exit(1)
	}
}

func waitForResponse(ctx context.Context, rdb *redis.Client, channel, correlationID string, timeout time.Duration) (*protocol.Message, error) {
	sub := rdb.Subscribe(ctx, channel)
	defer sub.Close()

	ch := sub.Channel()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return nil, fmt.Errorf("subscription channel closed")
			}
			parsed, err := protocol.Parse([]byte(msg.Payload))
			if err != nil {
				continue
			}
			if parsed.Envelope.CorrelationID == correlationID {
				return parsed, nil
			}
		case <-timer.C:
			return nil, fmt.Errorf("timeout waiting for response (correlation_id=%s)", correlationID)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
