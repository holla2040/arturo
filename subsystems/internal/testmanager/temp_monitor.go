package testmanager

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/holla2040/arturo/internal/script/executor"
	"github.com/holla2040/arturo/internal/store"
)

// tempTelemetry is the subset of the firmware's get_telemetry response this
// monitor needs. Matches docs/SCRIPTING_HAL.md "Telemetry Snapshot".
type tempTelemetry struct {
	Stage1TempK float64 `json:"stage1_temp_k"`
	Stage2TempK float64 `json:"stage2_temp_k"`
}

// TempMonitor queries cached pump telemetry every 5 seconds and stores the
// temperatures in SQLite. It bypasses the PausableRouter (uses raw router)
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
	// One cache-served get_telemetry query replaces two individual temp
	// queries. See docs/architecture/ARCHITECTURE.md §4.6.
	result, err := tm.router.SendCommand(ctx, tm.deviceID, "get_telemetry", nil, 5000)
	if err != nil {
		if ctx.Err() != nil {
			return // Context cancelled, shutting down
		}
		log.Printf("temp_monitor: %s telemetry query failed: %v", tm.stationInstance, err)
		return
	}
	if !result.Success {
		log.Printf("temp_monitor: %s telemetry query unsuccessful", tm.stationInstance)
		return
	}

	var snap tempTelemetry
	if err := json.Unmarshal([]byte(result.Response), &snap); err != nil {
		log.Printf("temp_monitor: %s telemetry parse error: %v (response: %q)",
			tm.stationInstance, err, result.Response)
		return
	}

	tm.recordStage("first_stage", snap.Stage1TempK)
	tm.recordStage("second_stage", snap.Stage2TempK)
}

func (tm *TempMonitor) recordStage(stage string, tempK float64) {
	if err := tm.store.RecordTemperature(tm.testRunID, tm.stationInstance, tm.deviceID, stage, tempK); err != nil {
		log.Printf("temp_monitor: %s store error: %v", tm.stationInstance, err)
		return
	}
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
