package testmanager

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/holla2040/arturo/internal/protocol"
	"github.com/holla2040/arturo/internal/script/ast"
	"github.com/holla2040/arturo/internal/script/executor"
	"github.com/holla2040/arturo/internal/script/lexer"
	"github.com/holla2040/arturo/internal/script/parser"
	"github.com/holla2040/arturo/internal/script/result"
	"github.com/holla2040/arturo/internal/store"
	"github.com/redis/go-redis/v9"
)

// SessionState represents the state of a test session.
type SessionState string

const (
	StateRunning SessionState = "testing"
	StatePaused  SessionState = "paused"
)

// TestSession manages a single test execution on a station.
type TestSession struct {
	mu              sync.RWMutex
	testRunID       string
	rmaID           string
	rmaNumber       string
	stationInstance string
	deviceID        string
	scriptPath      string
	displayName     string
	state           SessionState
	startedAt       time.Time
	employeeID      string

	store           *store.Store
	hub             Broadcaster
	pausableRouter  *PausableRouter
	rawRouter       executor.DeviceRouter
	collector       *result.Collector
	rdb             *redis.Client
	source          protocol.Source

	cancel          context.CancelFunc
	tempCancel      context.CancelFunc
	doneCh          chan struct{}
}

// SessionInfo provides read-only info about a session.
type SessionInfo struct {
	TestRunID       string       `json:"test_run_id"`
	RMAID           string       `json:"rma_id"`
	RMANumber       string       `json:"rma_number"`
	StationInstance string       `json:"station_instance"`
	DeviceID        string       `json:"device_id"`
	ScriptPath      string       `json:"script_path"`
	State           SessionState `json:"state"`
	StartedAt       time.Time    `json:"started_at"`
	EmployeeID      string       `json:"employee_id"`
}

// StartSessionParams contains everything needed to start a test.
type StartSessionParams struct {
	TestRunID       string
	RMAID           string
	StationInstance string
	DeviceID        string
	ScriptPath      string
	EmployeeID      string
	RawRouter       executor.DeviceRouter // bypasses pause for temp monitor
	Store           *store.Store
	Hub             Broadcaster
	Rdb             *redis.Client
	Source          protocol.Source
}

// NewSession creates and starts a test session. It launches the executor
// and temperature monitor as goroutines.
func NewSession(ctx context.Context, params StartSessionParams) (*TestSession, error) {
	// Read and parse the script
	source, err := os.ReadFile(params.ScriptPath)
	if err != nil {
		return nil, fmt.Errorf("read script: %w", err)
	}

	scriptContent := string(source)
	scriptHash := fmt.Sprintf("%x", sha256.Sum256(source))

	tokens, lexErrors := lexer.New(scriptContent).Tokenize()
	if len(lexErrors) > 0 {
		return nil, fmt.Errorf("script lex errors: %v", lexErrors[0].Error())
	}

	program, parseErrors := parser.New(tokens).Parse()
	if len(parseErrors) > 0 {
		return nil, fmt.Errorf("script parse errors: %v", parseErrors[0].Error())
	}

	// Extract descriptive test title from the AST
	testTitle := extractTestTitle(program)
	displayName := testTitle
	if displayName == "" {
		displayName = filepath.Base(params.ScriptPath)
	}

	// Extract and enforce report metadata
	reportType, reportVersion := extractScriptMeta(program)
	if reportType == "" || reportVersion == "" {
		return nil, fmt.Errorf("script missing required CONST: REPORT_TYPE and REPORT_VERSION")
	}

	// Create test run in SQLite (store display name, not full path)
	if err := params.Store.CreateTestRunWithRMA(
		params.TestRunID, displayName, params.RMAID,
		params.StationInstance, scriptHash, scriptContent,
		reportType, reportVersion,
	); err != nil {
		return nil, fmt.Errorf("create test run: %w", err)
	}

	// Record started event with both filename and title
	startedDesc := filepath.Base(params.ScriptPath)
	if testTitle != "" {
		startedDesc += " - " + testTitle
	}
	params.Store.RecordTestEvent(params.TestRunID, "started", params.EmployeeID, startedDesc)

	// Create pausable router wrapping the raw router
	pausable := NewPausableRouter(params.RawRouter)

	// Create result collector
	collector := result.NewCollector(params.ScriptPath)

	// Create cancellable contexts
	execCtx, execCancel := context.WithCancel(ctx)
	tempCtx, tempCancel := context.WithCancel(ctx)

	// Resolve human-readable RMA number
	var rmaNumber string
	if rma, err := params.Store.GetRMA(params.RMAID); err == nil && rma != nil {
		rmaNumber = rma.RMANumber
	}

	session := &TestSession{
		testRunID:       params.TestRunID,
		rmaID:           params.RMAID,
		rmaNumber:       rmaNumber,
		stationInstance: params.StationInstance,
		deviceID:        params.DeviceID,
		scriptPath:      params.ScriptPath,
		displayName:     displayName,
		state:           StateRunning,
		startedAt:       time.Now(),
		employeeID:      params.EmployeeID,
		store:           params.Store,
		hub:             params.Hub,
		pausableRouter:  pausable,
		rawRouter:       params.RawRouter,
		collector:       collector,
		rdb:             params.Rdb,
		source:          params.Source,
		cancel:          execCancel,
		tempCancel:      tempCancel,
		doneCh:          make(chan struct{}),
	}

	// Update station state
	params.Store.SetStationState(params.StationInstance, "testing", &params.TestRunID)
	if params.Hub != nil {
		params.Hub.BroadcastEvent("station_state", map[string]interface{}{
			"station_instance": params.StationInstance,
			"state":            "testing",
			"test_run_id":      params.TestRunID,
			"rma_id":           params.RMAID,
			"rma_number":       rmaNumber,
			"script_name":      params.ScriptPath,
			"started_at":       session.startedAt.Format(time.RFC3339Nano),
		})
	}

	// Notify station display: test is running
	session.notifyStation("running")

	// Start temperature monitor
	tempMon := NewTempMonitor(params.RawRouter, params.Store, params.Hub,
		params.TestRunID, params.StationInstance, params.DeviceID)
	go tempMon.Run(tempCtx)

	// Start executor in background
	go session.runExecutor(execCtx, string(source))

	return session, nil
}

// sessionEventEmitter bridges executor events to the store and WebSocket hub.
type sessionEventEmitter struct {
	testRunID       string
	stationInstance string
	employeeID      string
	store           *store.Store
	hub             Broadcaster
}

func (se *sessionEventEmitter) EmitEvent(eventType, detail string) {
	se.store.RecordTestEvent(se.testRunID, eventType, se.employeeID, detail)

	if se.hub != nil {
		se.hub.BroadcastEvent("test_event", map[string]interface{}{
			"test_run_id":      se.testRunID,
			"event_type":       eventType,
			"station_instance": se.stationInstance,
			"employee_id":      se.employeeID,
			"reason":           detail,
			"timestamp":        time.Now().UTC().Format(time.RFC3339Nano),
		})
	}
}

// extractScriptMeta walks the AST to find CONST REPORT_TYPE and REPORT_VERSION.
func extractScriptMeta(program *ast.Program) (reportType, reportVersion string) {
	for _, stmt := range program.Statements {
		if cs, ok := stmt.(*ast.ConstStmt); ok {
			if lit, ok := cs.Value.(*ast.StringLit); ok {
				switch cs.Name {
				case "REPORT_TYPE":
					reportType = lit.Value
				case "REPORT_VERSION":
					reportVersion = lit.Value
				}
			}
		}
	}
	return
}

// extractTestTitle walks the AST to find the first TEST name.
func extractTestTitle(program *ast.Program) string {
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.TestDef:
			if lit, ok := s.Name.(*ast.StringLit); ok {
				return lit.Value
			}
		case *ast.SuiteDef:
			for _, t := range s.Tests {
				if lit, ok := t.Name.(*ast.StringLit); ok {
					return lit.Value
				}
			}
		}
	}
	return ""
}

// runExecutor creates and runs an executor with the given script source.
func (s *TestSession) runExecutor(ctx context.Context, scriptSource string) {
	defer close(s.doneCh)
	defer s.tempCancel()

	tokens, _ := lexer.New(scriptSource).Tokenize()
	program, _ := parser.New(tokens).Parse()

	emitter := &sessionEventEmitter{
		testRunID:       s.testRunID,
		stationInstance: s.stationInstance,
		employeeID:      s.employeeID,
		store:           s.store,
		hub:             s.hub,
	}

	exec := executor.New(ctx,
		executor.WithRouter(s.pausableRouter),
		executor.WithCollector(s.collector),
		executor.WithEmitter(emitter),
		executor.WithDeviceID(s.deviceID),
	)

	execErr := exec.Execute(program)
	report := s.collector.Finalize()

	if ctx.Err() != nil {
		// Context was cancelled — either terminate or abort
		// The calling code handles the state transition
		return
	}

	// Test completed normally
	status := "passed"
	summary := fmt.Sprintf("%d tests, %d passed, %d failed",
		report.Summary.Total, report.Summary.Passed, report.Summary.Failed)
	if report.Summary.Failed > 0 || report.Summary.Errors > 0 {
		status = "failed"
	}
	if execErr != nil {
		status = "error"
		summary = execErr.Error()
	}

	s.finish(status, summary)
}

// finish completes the session and updates the store.
func (s *TestSession) finish(status, summary string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.store.FinishTestRun(s.testRunID, status, summary); err != nil {
		log.Printf("testmanager: finish test run %s: %v", s.testRunID, err)
	}

	s.store.RecordTestEvent(s.testRunID, "completed", s.employeeID, summary)
	s.store.SetStationState(s.stationInstance, "idle", nil)

	if s.hub != nil {
		s.hub.BroadcastEvent("test_event", map[string]interface{}{
			"test_run_id":      s.testRunID,
			"event_type":       "completed",
			"station_instance": s.stationInstance,
			"employee_id":      s.employeeID,
			"status":           status,
			"summary":          summary,
			"timestamp":        time.Now().UTC().Format(time.RFC3339Nano),
		})
		s.hub.BroadcastEvent("station_state", map[string]interface{}{
			"station_instance": s.stationInstance,
			"state":            "idle",
			"test_run_id":      nil,
		})
	}

	s.notifyStation("completed")
}

// Info returns a snapshot of the session state.
func (s *TestSession) Info() SessionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return SessionInfo{
		TestRunID:       s.testRunID,
		RMAID:           s.rmaID,
		RMANumber:       s.rmaNumber,
		StationInstance: s.stationInstance,
		DeviceID:        s.deviceID,
		ScriptPath:      s.scriptPath,
		State:           s.state,
		StartedAt:       s.startedAt,
		EmployeeID:      s.employeeID,
	}
}

// Pause pauses the test execution (temperature monitoring continues).
func (s *TestSession) Pause(employeeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateRunning {
		return fmt.Errorf("cannot pause: session is %s", s.state)
	}

	s.pausableRouter.Pause()
	s.state = StatePaused

	s.store.RecordTestEvent(s.testRunID, "paused", employeeID, "")
	s.store.SetStationState(s.stationInstance, "paused", &s.testRunID)

	if s.hub != nil {
		s.hub.BroadcastEvent("test_event", map[string]interface{}{
			"test_run_id":      s.testRunID,
			"event_type":       "paused",
			"station_instance": s.stationInstance,
			"timestamp":        time.Now().UTC().Format(time.RFC3339Nano),
		})
		s.hub.BroadcastEvent("station_state", map[string]interface{}{
			"station_instance": s.stationInstance,
			"state":            "paused",
			"test_run_id":      s.testRunID,
			"rma_id":           s.rmaID,
			"rma_number":       s.rmaNumber,
			"script_name":      s.scriptPath,
			"started_at":       s.startedAt.Format(time.RFC3339Nano),
		})
	}

	s.notifyStation("paused")

	return nil
}

// Resume resumes a paused test.
func (s *TestSession) Resume(employeeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StatePaused {
		return fmt.Errorf("cannot resume: session is %s", s.state)
	}

	s.pausableRouter.Resume()
	s.state = StateRunning

	s.store.RecordTestEvent(s.testRunID, "resumed", employeeID, "")
	s.store.SetStationState(s.stationInstance, "testing", &s.testRunID)

	if s.hub != nil {
		s.hub.BroadcastEvent("test_event", map[string]interface{}{
			"test_run_id":      s.testRunID,
			"event_type":       "resumed",
			"station_instance": s.stationInstance,
			"timestamp":        time.Now().UTC().Format(time.RFC3339Nano),
		})
		s.hub.BroadcastEvent("station_state", map[string]interface{}{
			"station_instance": s.stationInstance,
			"state":            "testing",
			"test_run_id":      s.testRunID,
			"rma_id":           s.rmaID,
			"rma_number":       s.rmaNumber,
			"script_name":      s.scriptPath,
			"started_at":       s.startedAt.Format(time.RFC3339Nano),
		})
	}

	s.notifyStation("running")

	return nil
}

// Terminate stops the test but preserves all data.
func (s *TestSession) Terminate(employeeID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateRunning && s.state != StatePaused {
		return fmt.Errorf("cannot terminate: session is %s", s.state)
	}

	// If paused, resume first so the executor can exit cleanly
	if s.state == StatePaused {
		s.pausableRouter.Resume()
	}

	// Cancel the executor context
	s.cancel()

	// Wait for executor to finish
	<-s.doneCh

	// Record terminate event and update state
	s.store.RecordTestEvent(s.testRunID, "terminated", employeeID, reason)
	s.store.FinishTestRun(s.testRunID, "terminated", reason)
	s.store.SetStationState(s.stationInstance, "idle", nil)

	if s.hub != nil {
		s.hub.BroadcastEvent("test_event", map[string]interface{}{
			"test_run_id":      s.testRunID,
			"event_type":       "terminated",
			"station_instance": s.stationInstance,
			"reason":           reason,
			"timestamp":        time.Now().UTC().Format(time.RFC3339Nano),
		})
		s.hub.BroadcastEvent("station_state", map[string]interface{}{
			"station_instance": s.stationInstance,
			"state":            "idle",
			"test_run_id":      nil,
		})
	}

	s.notifyStation("completed")

	return nil
}

// Abort stops the test and discards all data.
func (s *TestSession) Abort(employeeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateRunning && s.state != StatePaused {
		return fmt.Errorf("cannot abort: session is %s", s.state)
	}

	// If paused, resume first
	if s.state == StatePaused {
		s.pausableRouter.Resume()
	}

	// Cancel the executor context
	s.cancel()

	// Wait for executor to finish
	<-s.doneCh

	// Delete test run data
	if err := s.store.DeleteTestRun(s.testRunID); err != nil {
		log.Printf("testmanager: abort delete test run %s: %v", s.testRunID, err)
	}

	s.store.SetStationState(s.stationInstance, "idle", nil)

	if s.hub != nil {
		s.hub.BroadcastEvent("test_event", map[string]interface{}{
			"test_run_id":      s.testRunID,
			"event_type":       "aborted",
			"station_instance": s.stationInstance,
			"timestamp":        time.Now().UTC().Format(time.RFC3339Nano),
		})
		s.hub.BroadcastEvent("station_state", map[string]interface{}{
			"station_instance": s.stationInstance,
			"state":            "idle",
			"test_run_id":      nil,
		})
	}

	s.notifyStation("aborted")

	return nil
}

// Done returns a channel that's closed when the session completes.
func (s *TestSession) Done() <-chan struct{} {
	return s.doneCh
}

// TestRunID returns the test run ID.
func (s *TestSession) TestRunID() string {
	return s.testRunID
}

// notifyStation publishes a test.state.update message to the station's command
// channel so the station display can show test status and lock out manual controls.
func (s *TestSession) notifyStation(state string) {
	if s.rdb == nil {
		return
	}

	elapsed := uint32(time.Since(s.startedAt).Seconds())

	payload := map[string]interface{}{
		"state":           state,
		"test_id":         s.testRunID,
		"test_name":       s.displayName,
		"elapsed_seconds": elapsed,
	}

	msg, err := protocol.NewMessage(s.source, protocol.TypeTestStateUpdate, payload)
	if err != nil {
		log.Printf("testmanager: build test.state.update: %v", err)
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("testmanager: marshal test.state.update: %v", err)
		return
	}

	channel := "commands:" + s.stationInstance
	if err := s.rdb.Publish(context.Background(), channel, string(data)).Err(); err != nil {
		log.Printf("testmanager: publish test.state.update to %s: %v", channel, err)
	}
}
