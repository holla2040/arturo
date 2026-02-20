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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/holla2040/arturo/internal/console"
	"github.com/holla2040/arturo/internal/mockpump"
	"github.com/holla2040/arturo/internal/protocol"
	"github.com/redis/go-redis/v9"
)

const firmwareVersion = "1.0.0-mock"

func main() {
	redisAddr := flag.String("redis", "localhost:6379", "Redis address")
	stationsFlag := flag.String("stations", "1,2,3,4", "Comma-separated station numbers to mock (e.g. 2,3,4)")
	failRate := flag.Float64("fail-rate", 0.0, "Probability of random command failure (0.0-1.0)")
	cooldownHours := flag.Float64("cooldown-hours", 4.0, "Simulated hours to reach base temperature")
	httpAddr := flag.String("http", ":8001", "HTTP address for web UI")
	flag.Parse()

	stationNums, err := parseStations(*stationsFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid -stations value: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	rdb := redis.NewClient(&redis.Options{Addr: *redisAddr})
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to Redis at %s: %v\n", *redisAddr, err)
		os.Exit(1)
	}
	log.Printf("Connected to Redis at %s", *redisAddr)

	// Create mock stations
	var wg sync.WaitGroup
	stationInfos := make([]*console.StationInfo, 0, len(stationNums))

	for _, num := range stationNums {
		inst := fmt.Sprintf("station-%02d", num)
		dev := fmt.Sprintf("PUMP-%02d", num)

		pump := mockpump.NewPump(*cooldownHours, *failRate)
		online := true
		s := &mockStation{
			rdb:      rdb,
			instance: inst,
			deviceID: dev,
			pump:     pump,
			online:   &online,
		}

		stationInfos = append(stationInfos, &console.StationInfo{
			Instance: inst,
			DeviceID: dev,
			Pump:     pump,
			Online:   &online,
		})

		wg.Add(1)
		go func() {
			defer wg.Done()
			s.run(ctx)
		}()

		log.Printf("Started mock station %s with device %s", inst, dev)
	}

	// Build console handler
	handler, runMonitor := console.Handler(stationInfos, rdb)
	runMonitor(ctx)

	// Start HTTP server
	srv := &http.Server{Addr: *httpAddr, Handler: handler}
	go func() {
		log.Printf("Web UI at http://localhost%s", *httpAddr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)

	wg.Wait()
	log.Println("All mock stations stopped")
}

func parseStations(s string) ([]int, error) {
	parts := strings.Split(s, ",")
	nums := make([]int, 0, len(parts))
	seen := make(map[int]bool)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid station number %q", p)
		}
		if n < 1 || n > 6 {
			return nil, fmt.Errorf("station number %d out of range (1-6)", n)
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		nums = append(nums, n)
	}
	if len(nums) == 0 {
		return nil, fmt.Errorf("no station numbers specified")
	}
	return nums, nil
}

// ── Mock Station (moved from arturo-mock-station) ──

type mockStation struct {
	rdb      *redis.Client
	instance string
	deviceID string
	pump     *mockpump.Pump
	online   *bool
	startUp  time.Time
}

func (s *mockStation) run(ctx context.Context) {
	s.startUp = time.Now()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		s.heartbeatLoop(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		s.presenceLoop(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		s.commandLoop(ctx)
	}()

	wg.Wait()
}

func (s *mockStation) source() protocol.Source {
	return protocol.Source{
		Service:  "arturo_station",
		Instance: s.instance,
		Version:  firmwareVersion,
	}
}

func (s *mockStation) heartbeatLoop(ctx context.Context) {
	if *s.online {
		s.sendHeartbeat(ctx)
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if *s.online {
				s.sendHeartbeat(ctx)
			}
		}
	}
}

func (s *mockStation) sendHeartbeat(ctx context.Context) {
	uptime := int64(time.Since(s.startUp).Seconds())
	cmdProcessed := 0
	cmdFailed := 0

	payload := protocol.HeartbeatPayload{
		Status:            "online",
		UptimeSeconds:     uptime,
		Devices:           []string{s.deviceID},
		DeviceTypes:       map[string]string{s.deviceID: "mock"},
		FreeHeap:          245760,
		WifiRSSI:          -55,
		CommandsProcessed: &cmdProcessed,
		CommandsFailed:    &cmdFailed,
		LastError:         nil,
		FirmwareVersion:   firmwareVersion,
	}

	msg, err := protocol.NewMessage(s.source(), protocol.TypeServiceHeartbeat, payload)
	if err != nil {
		log.Printf("[%s] heartbeat build error: %v", s.instance, err)
		return
	}

	msgJSON, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[%s] heartbeat marshal error: %v", s.instance, err)
		return
	}

	if err := s.rdb.Publish(ctx, "events:heartbeat", string(msgJSON)).Err(); err != nil {
		if ctx.Err() == nil {
			log.Printf("[%s] heartbeat publish error: %v", s.instance, err)
		}
	}
}

func (s *mockStation) presenceLoop(ctx context.Context) {
	key := "device:" + s.instance + ":alive"
	wasOnline := *s.online
	if wasOnline {
		s.rdb.Set(ctx, key, "1", 15*time.Second)
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			on := *s.online
			if on {
				s.rdb.Set(ctx, key, "1", 15*time.Second)
			} else if wasOnline {
				// Transitioning to offline — remove alive key immediately
				s.rdb.Del(ctx, key)
			}
			wasOnline = on
		}
	}
}

func (s *mockStation) commandLoop(ctx context.Context) {
	channel := "commands:" + s.instance

	for {
		if ctx.Err() != nil {
			return
		}

		sub := s.rdb.Subscribe(ctx, channel)
		ch := sub.Channel()

		func() {
			defer sub.Close()
			for {
				select {
				case <-ctx.Done():
					return
				case msg, ok := <-ch:
					if !ok {
						log.Printf("[%s] command subscription closed, reconnecting...", s.instance)
						return
					}
					if !*s.online {
						continue
					}
					s.handleCommand(ctx, msg.Payload)
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

func (s *mockStation) handleCommand(ctx context.Context, msgJSON string) {
	parsed, err := protocol.Parse([]byte(msgJSON))
	if err != nil {
		log.Printf("[%s] parse error: %v", s.instance, err)
		return
	}

	cmdPayload, err := protocol.ParseCommandRequest(parsed)
	if err != nil {
		log.Printf("[%s] command parse error: %v", s.instance, err)
		return
	}

	start := time.Now()

	if cmdPayload.DeviceID != s.deviceID {
		s.sendResponse(ctx, parsed, cmdPayload, false, nil,
			&protocol.Error{Code: "DEVICE_NOT_FOUND", Message: fmt.Sprintf("unknown device: %s", cmdPayload.DeviceID)},
			time.Since(start))
		return
	}

	response, success := s.pump.HandleCommand(cmdPayload.CommandName)
	duration := time.Since(start)

	if success {
		s.sendResponse(ctx, parsed, cmdPayload, true, &response, nil, duration)
	} else {
		respErr := &protocol.Error{Code: "COMMAND_FAILED", Message: response}
		s.sendResponse(ctx, parsed, cmdPayload, false, nil, respErr, duration)
	}
}

func (s *mockStation) sendResponse(ctx context.Context, req *protocol.Message, cmdPayload *protocol.CommandRequestPayload, success bool, response *string, respErr *protocol.Error, duration time.Duration) {
	durationMs := int(duration.Milliseconds())

	payload := protocol.CommandResponsePayload{
		DeviceID:    cmdPayload.DeviceID,
		CommandName: cmdPayload.CommandName,
		Success:     success,
		Response:    response,
		Error:       respErr,
		DurationMs:  &durationMs,
	}

	msg, err := protocol.NewMessage(s.source(), protocol.TypeDeviceCommandResponse, payload)
	if err != nil {
		log.Printf("[%s] response build error: %v", s.instance, err)
		return
	}

	msg.Envelope.CorrelationID = req.Envelope.CorrelationID

	msgJSON, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[%s] response marshal error: %v", s.instance, err)
		return
	}

	replyTo := req.Envelope.ReplyTo
	if replyTo == "" {
		log.Printf("[%s] no reply_to in request", s.instance)
		return
	}

	if err := s.rdb.Publish(ctx, replyTo, string(msgJSON)).Err(); err != nil {
		if ctx.Err() == nil {
			log.Printf("[%s] response PUBLISH error: %v", s.instance, err)
		}
	}
}
