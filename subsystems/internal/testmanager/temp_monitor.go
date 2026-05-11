package testmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/holla2040/arturo/internal/regen"
	"github.com/holla2040/arturo/internal/script/executor"
	"github.com/holla2040/arturo/internal/store"
)

// tempTelemetry is the subset of the firmware's get_telemetry response this
// monitor needs. Matches docs/SCRIPTING_HAL.md "Telemetry Snapshot".
type tempTelemetry struct {
	Stage1TempK float64 `json:"stage1_temp_k"`
	Stage2TempK float64 `json:"stage2_temp_k"`
	RegenChar   string  `json:"regen_char"`
}

// TempMonitor queries cached pump telemetry every 5 seconds and stores the
// temperatures in SQLite. It bypasses the PausableRouter (uses raw router)
// so temperatures keep recording during pause.
//
// It also tracks the regen state character and emits a single `regen_state`
// test event on each change — that's the operator-facing signal that
// replaces the per-query spam the script executor used to emit. See
// docs/architecture/TEST_EVENTS.md.
type TempMonitor struct {
	router          executor.DeviceRouter // raw router, NOT PausableRouter
	store           *store.Store
	hub             Broadcaster
	testRunID       string
	stationInstance string
	deviceID        string
	employeeID      string
	startedAt       time.Time
	interval        time.Duration

	lastRegenChar string
}

// NewTempMonitor creates a temperature monitor. startedAt is used to format
// the elapsed-time field in emitted regen_state events.
func NewTempMonitor(router executor.DeviceRouter, st *store.Store, hub Broadcaster, testRunID, stationInstance, deviceID, employeeID string, startedAt time.Time) *TempMonitor {
	return &TempMonitor{
		router:          router,
		store:           st,
		hub:             hub,
		testRunID:       testRunID,
		stationInstance: stationInstance,
		deviceID:        deviceID,
		employeeID:      employeeID,
		startedAt:       startedAt,
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

	// Skip non-positive temperatures. A cryopump can't read 0 K (or below);
	// when we see one it's a partial-poll/comm-glitch artifact in the
	// firmware cache — the firmware resets stale_count on ANY successful
	// poll command, so a persistently failing J/K query still produces a
	// "fresh-looking" snapshot with an un-updated stage temp. Better to
	// skip the sample than inject a misleading zero into the plot.
	if snap.Stage1TempK > 0 {
		tm.recordStage("first_stage", snap.Stage1TempK)
	} else {
		log.Printf("temp_monitor: %s skipping first_stage sample, T1=%.3f (suspect data)",
			tm.stationInstance, snap.Stage1TempK)
	}
	if snap.Stage2TempK > 0 {
		tm.recordStage("second_stage", snap.Stage2TempK)
	} else {
		log.Printf("temp_monitor: %s skipping second_stage sample, T2=%.3f (suspect data)",
			tm.stationInstance, snap.Stage2TempK)
	}
	tm.recordRegenChange(snap)
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

// recordRegenChange emits a `regen_state` test event when the regen character
// changes (or on the very first sample). The reason string carries the
// single-character code, the human-readable state name, both stage
// temperatures, and the elapsed test-run time.
func (tm *TempMonitor) recordRegenChange(snap tempTelemetry) {
	if snap.RegenChar == tm.lastRegenChar {
		return
	}
	tm.lastRegenChar = snap.RegenChar

	reason := fmt.Sprintf("regen=%s (%s) • 1st=%.1fK • 2nd=%.1fK • elapsed=%s",
		snap.RegenChar,
		regen.StateName(snap.RegenChar),
		snap.Stage1TempK,
		snap.Stage2TempK,
		formatElapsed(time.Since(tm.startedAt)),
	)

	if err := tm.store.RecordTestEvent(tm.testRunID, "regen_state", tm.employeeID, reason); err != nil {
		log.Printf("temp_monitor: %s record regen_state: %v", tm.stationInstance, err)
		return
	}

	if tm.hub != nil {
		tm.hub.BroadcastEvent("test_event", map[string]interface{}{
			"test_run_id":      tm.testRunID,
			"event_type":       "regen_state",
			"station_instance": tm.stationInstance,
			"employee_id":      tm.employeeID,
			"reason":           reason,
			"timestamp":        time.Now().UTC().Format(time.RFC3339Nano),
		})
	}
}

// formatElapsed formats a duration the same way the terminal's detail-page
// elapsed field does (commit 70f466c): m:ss under one hour, h:mm:ss above.
func formatElapsed(d time.Duration) string {
	secs := int(d.Seconds())
	if secs < 0 {
		secs = 0
	}
	h := secs / 3600
	m := (secs % 3600) / 60
	s := secs % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
