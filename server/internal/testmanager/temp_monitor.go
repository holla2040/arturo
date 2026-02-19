package testmanager

import (
	"context"
	"log"
	"strconv"
	"time"


	"github.com/holla2040/arturo/internal/script/executor"
	"github.com/holla2040/arturo/internal/store"
)

// TempMonitor queries 1st and 2nd stage temperatures every 5 seconds
// and stores them in SQLite. It bypasses the PausableRouter (uses raw router)
// so temperatures keep recording during pause.
type TempMonitor struct {
	router          executor.DeviceRouter // raw router, NOT PausableRouter
	store           *store.Store
	hub             Broadcaster
	testRunID       string
	stationInstance string
	deviceID        string
	interval        time.Duration
}

// NewTempMonitor creates a temperature monitor.
func NewTempMonitor(router executor.DeviceRouter, st *store.Store, hub Broadcaster, testRunID, stationInstance, deviceID string) *TempMonitor {
	return &TempMonitor{
		router:          router,
		store:           st,
		hub:             hub,
		testRunID:       testRunID,
		stationInstance: stationInstance,
		deviceID:        deviceID,
		interval:        5 * time.Second,
	}
}

// Run starts the temperature monitoring loop. It blocks until ctx is cancelled.
func (tm *TempMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(tm.interval)
	defer ticker.Stop()

	// Take an immediate reading
	tm.sample(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tm.sample(ctx)
		}
	}
}

func (tm *TempMonitor) sample(ctx context.Context) {
	// Query 1st stage temperature
	tm.queryAndRecord(ctx, "get_temp_1st_stage", "first_stage")
	// Query 2nd stage temperature
	tm.queryAndRecord(ctx, "get_temp_2nd_stage", "second_stage")
}

func (tm *TempMonitor) queryAndRecord(ctx context.Context, command, stage string) {
	result, err := tm.router.SendCommand(ctx, tm.deviceID, command, nil, 5000)
	if err != nil {
		if ctx.Err() != nil {
			return // Context cancelled, shutting down
		}
		log.Printf("temp_monitor: %s %s query failed: %v", tm.stationInstance, stage, err)
		return
	}

	if !result.Success {
		log.Printf("temp_monitor: %s %s query unsuccessful", tm.stationInstance, stage)
		return
	}

	tempK, err := strconv.ParseFloat(result.Response, 64)
	if err != nil {
		log.Printf("temp_monitor: %s %s parse error: %v (response: %q)", tm.stationInstance, stage, err, result.Response)
		return
	}

	if err := tm.store.RecordTemperature(tm.testRunID, tm.stationInstance, tm.deviceID, stage, tempK); err != nil {
		log.Printf("temp_monitor: %s store error: %v", tm.stationInstance, err)
		return
	}

	// Broadcast temperature update via WebSocket
	if tm.hub != nil {
		tm.hub.BroadcastEvent("temperature", map[string]interface{}{
			"test_run_id":      tm.testRunID,
			"station_instance": tm.stationInstance,
			"device_id":        tm.deviceID,
			"stage":            stage,
			"temperature_k":    tempK,
			"timestamp":        time.Now().UTC().Format(time.RFC3339Nano),
		})
	}
}
