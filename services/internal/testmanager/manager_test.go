package testmanager

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/holla2040/arturo/internal/script/executor"
	"github.com/holla2040/arturo/internal/store"
)

// mockRouter implements executor.DeviceRouter with canned responses.
type mockRouter struct {
	mu        sync.Mutex
	calls     []mockCall
	responses map[string]*executor.CommandResult
	delay     time.Duration
}

type mockCall struct {
	DeviceID string
	Command  string
}

func newMockRouter() *mockRouter {
	return &mockRouter{
		responses: map[string]*executor.CommandResult{
			"pump_status":         {Success: true, Response: "1", DurationMs: 50},
			"pump_on":             {Success: true, Response: "A", DurationMs: 50},
			"pump_off":            {Success: true, Response: "A", DurationMs: 50},
			"get_temp_1st_stage":  {Success: true, Response: "77.5", DurationMs: 50},
			"get_temp_2nd_stage":  {Success: true, Response: "15.2", DurationMs: 50},
		},
	}
}

func (m *mockRouter) SendCommand(ctx context.Context, deviceID, command string, params map[string]string, timeoutMs int) (*executor.CommandResult, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	m.mu.Lock()
	m.calls = append(m.calls, mockCall{DeviceID: deviceID, Command: command})
	m.mu.Unlock()

	if result, ok := m.responses[command]; ok {
		return result, nil
	}
	return &executor.CommandResult{Success: true, Response: "OK", DurationMs: 10}, nil
}

func (m *mockRouter) getCalls() []mockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := make([]mockCall, len(m.calls))
	copy(calls, m.calls)
	return calls
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func writeTestScript(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.art")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write test script: %v", err)
	}
	return path
}

func TestPausableRouterBlocksWhenPaused(t *testing.T) {
	inner := newMockRouter()
	pr := NewPausableRouter(inner)

	// Normal call should work
	result, err := pr.SendCommand(context.Background(), "PUMP-01", "pump_status", nil, 5000)
	if err != nil {
		t.Fatalf("SendCommand failed: %v", err)
	}
	if result.Response != "1" {
		t.Errorf("expected response '1', got %s", result.Response)
	}

	// Pause
	pr.Pause()
	if !pr.IsPaused() {
		t.Error("expected paused=true")
	}

	// Verify SendCommand blocks when paused
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = pr.SendCommand(ctx, "PUMP-01", "pump_status", nil, 5000)
	if err == nil {
		t.Error("expected timeout error when paused")
	}

	// Resume and verify normal operation
	pr.Resume()
	if pr.IsPaused() {
		t.Error("expected paused=false after resume")
	}

	result, err = pr.SendCommand(context.Background(), "PUMP-01", "pump_on", nil, 5000)
	if err != nil {
		t.Fatalf("SendCommand after resume failed: %v", err)
	}
	if result.Response != "A" {
		t.Errorf("expected response 'A', got %s", result.Response)
	}
}

func TestPausableRouterResumeUnblocks(t *testing.T) {
	inner := newMockRouter()
	pr := NewPausableRouter(inner)

	pr.Pause()

	var wg sync.WaitGroup
	var sendErr error
	var sendResult *executor.CommandResult

	wg.Add(1)
	go func() {
		defer wg.Done()
		sendResult, sendErr = pr.SendCommand(context.Background(), "PUMP-01", "pump_status", nil, 5000)
	}()

	// Give the goroutine time to block
	time.Sleep(50 * time.Millisecond)

	// Resume should unblock
	pr.Resume()
	wg.Wait()

	if sendErr != nil {
		t.Fatalf("SendCommand after resume failed: %v", sendErr)
	}
	if sendResult.Response != "1" {
		t.Errorf("expected response '1', got %s", sendResult.Response)
	}
}

func TestManagerStartAndComplete(t *testing.T) {
	st := newTestStore(t)
	router := newMockRouter()

	script := writeTestScript(t, `TEST "Simple Test"
    QUERY "PUMP-01" "pump_status" status TIMEOUT 5000
    PASS "pump responded"
ENDTEST`)

	st.CreateEmployee("emp-1", "Test User")
	st.CreateRMA("rma-1", "RMA-001", "SN1", "Customer", "CT-8", "emp-1", "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewWithFactory(ctx, st, nil, func(station string) executor.DeviceRouter {
		return router
	})

	err := mgr.StartTest("station-01", "PUMP-01", script, "rma-1", "run-1", "emp-1")
	if err != nil {
		t.Fatalf("StartTest failed: %v", err)
	}

	// Verify session exists
	session := mgr.GetSession("station-01")
	if session == nil {
		t.Fatal("expected active session")
	}
	if session.State != StateRunning {
		t.Errorf("expected state running, got %s", session.State)
	}

	// Wait for completion
	time.Sleep(500 * time.Millisecond)

	// Session should be cleaned up
	session = mgr.GetSession("station-01")
	if session != nil {
		t.Error("expected nil session after completion")
	}

	// Test run should be finished in store
	run, err := st.GetTestRun("run-1")
	if err != nil {
		t.Fatalf("GetTestRun failed: %v", err)
	}
	if run == nil {
		t.Fatal("expected non-nil test run")
	}
	if run.Status != "passed" {
		t.Errorf("expected status passed, got %s", run.Status)
	}
}

func TestManagerDuplicateStartRejected(t *testing.T) {
	st := newTestStore(t)
	router := newMockRouter()
	router.delay = 2 * time.Second // Keep test running

	script := writeTestScript(t, `TEST "Long Test"
    QUERY "PUMP-01" "pump_status" status TIMEOUT 5000
    DELAY 5000
    PASS "done"
ENDTEST`)

	st.CreateEmployee("emp-1", "Test User")
	st.CreateRMA("rma-1", "RMA-001", "SN1", "Customer", "CT-8", "emp-1", "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewWithFactory(ctx, st, nil, func(station string) executor.DeviceRouter {
		return router
	})

	err := mgr.StartTest("station-01", "PUMP-01", script, "rma-1", "run-1", "emp-1")
	if err != nil {
		t.Fatalf("first StartTest failed: %v", err)
	}

	err = mgr.StartTest("station-01", "PUMP-01", script, "rma-1", "run-2", "emp-1")
	if err == nil {
		t.Error("expected error for duplicate start")
	}
}

func TestManagerPauseResume(t *testing.T) {
	st := newTestStore(t)
	router := newMockRouter()
	router.delay = 200 * time.Millisecond // Slow down commands

	script := writeTestScript(t, `TEST "Slow Test"
    QUERY "PUMP-01" "pump_status" status TIMEOUT 5000
    QUERY "PUMP-01" "pump_status" status TIMEOUT 5000
    QUERY "PUMP-01" "pump_status" status TIMEOUT 5000
    PASS "done"
ENDTEST`)

	st.CreateEmployee("emp-1", "Test User")
	st.CreateRMA("rma-1", "RMA-001", "SN1", "Customer", "CT-8", "emp-1", "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewWithFactory(ctx, st, nil, func(station string) executor.DeviceRouter {
		return router
	})

	mgr.StartTest("station-01", "PUMP-01", script, "rma-1", "run-1", "emp-1")

	// Wait for test to start executing
	time.Sleep(100 * time.Millisecond)

	// Pause
	err := mgr.PauseTest("station-01", "emp-1")
	if err != nil {
		t.Fatalf("PauseTest failed: %v", err)
	}

	session := mgr.GetSession("station-01")
	if session == nil {
		t.Fatal("expected session after pause")
	}
	if session.State != StatePaused {
		t.Errorf("expected paused, got %s", session.State)
	}

	// Verify pause event recorded
	events, _ := st.QueryTestEvents("run-1")
	found := false
	for _, e := range events {
		if e.EventType == "paused" {
			found = true
		}
	}
	if !found {
		t.Error("expected paused event in store")
	}

	// Resume
	err = mgr.ResumeTest("station-01", "emp-1")
	if err != nil {
		t.Fatalf("ResumeTest failed: %v", err)
	}

	session = mgr.GetSession("station-01")
	if session != nil && session.State != StateRunning {
		t.Errorf("expected running after resume, got %s", session.State)
	}
}

func TestManagerTerminate(t *testing.T) {
	st := newTestStore(t)
	router := newMockRouter()
	router.delay = 500 * time.Millisecond

	script := writeTestScript(t, `TEST "Long Test"
    QUERY "PUMP-01" "pump_status" status TIMEOUT 5000
    DELAY 10000
    PASS "done"
ENDTEST`)

	st.CreateEmployee("emp-1", "Test User")
	st.CreateRMA("rma-1", "RMA-001", "SN1", "Customer", "CT-8", "emp-1", "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewWithFactory(ctx, st, nil, func(station string) executor.DeviceRouter {
		return router
	})

	mgr.StartTest("station-01", "PUMP-01", script, "rma-1", "run-1", "emp-1")
	time.Sleep(100 * time.Millisecond)

	err := mgr.TerminateTest("station-01", "emp-1", "operator decision")
	if err != nil {
		t.Fatalf("TerminateTest failed: %v", err)
	}

	// Test run should be terminated with data preserved
	run, _ := st.GetTestRun("run-1")
	if run == nil {
		t.Fatal("expected non-nil test run (data should be preserved)")
	}
	if run.Status != "terminated" {
		t.Errorf("expected status terminated, got %s", run.Status)
	}

	// Events should include terminated
	events, _ := st.QueryTestEvents("run-1")
	found := false
	for _, e := range events {
		if e.EventType == "terminated" && e.Reason == "operator decision" {
			found = true
		}
	}
	if !found {
		t.Error("expected terminated event with reason")
	}
}

func TestManagerAbort(t *testing.T) {
	st := newTestStore(t)
	router := newMockRouter()
	router.delay = 500 * time.Millisecond

	script := writeTestScript(t, `TEST "Long Test"
    QUERY "PUMP-01" "pump_status" status TIMEOUT 5000
    DELAY 10000
    PASS "done"
ENDTEST`)

	st.CreateEmployee("emp-1", "Test User")
	st.CreateRMA("rma-1", "RMA-001", "SN1", "Customer", "CT-8", "emp-1", "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewWithFactory(ctx, st, nil, func(station string) executor.DeviceRouter {
		return router
	})

	mgr.StartTest("station-01", "PUMP-01", script, "rma-1", "run-1", "emp-1")
	time.Sleep(100 * time.Millisecond)

	err := mgr.AbortTest("station-01", "emp-1")
	if err != nil {
		t.Fatalf("AbortTest failed: %v", err)
	}

	// Test run should be deleted
	run, _ := st.GetTestRun("run-1")
	if run != nil {
		t.Error("expected nil test run after abort (data should be discarded)")
	}
}

func TestManagerEmergencyStopAll(t *testing.T) {
	st := newTestStore(t)
	router := newMockRouter()
	router.delay = 500 * time.Millisecond

	script := writeTestScript(t, `TEST "Long Test"
    QUERY "PUMP-01" "pump_status" status TIMEOUT 5000
    DELAY 10000
    PASS "done"
ENDTEST`)

	st.CreateEmployee("emp-1", "Test User")
	st.CreateRMA("rma-1", "RMA-001", "SN1", "Customer", "CT-8", "emp-1", "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewWithFactory(ctx, st, nil, func(station string) executor.DeviceRouter {
		return router
	})

	mgr.StartTest("station-01", "PUMP-01", script, "rma-1", "run-1", "emp-1")
	time.Sleep(100 * time.Millisecond)

	mgr.EmergencyStopAll()
	time.Sleep(200 * time.Millisecond)

	// All sessions should be cleaned up
	sessions := mgr.ListSessions()
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions after e-stop, got %d", len(sessions))
	}
}

func TestManagerNoSessionErrors(t *testing.T) {
	st := newTestStore(t)

	ctx := context.Background()
	mgr := NewWithFactory(ctx, st, nil, func(station string) executor.DeviceRouter {
		return newMockRouter()
	})

	if err := mgr.PauseTest("station-01", "emp-1"); err == nil {
		t.Error("expected error pausing non-existent session")
	}
	if err := mgr.ResumeTest("station-01", "emp-1"); err == nil {
		t.Error("expected error resuming non-existent session")
	}
	if err := mgr.TerminateTest("station-01", "emp-1", "reason"); err == nil {
		t.Error("expected error terminating non-existent session")
	}
	if err := mgr.AbortTest("station-01", "emp-1"); err == nil {
		t.Error("expected error aborting non-existent session")
	}
}

func TestManagerHasActiveTestForRMA(t *testing.T) {
	st := newTestStore(t)
	router := newMockRouter()
	router.delay = 500 * time.Millisecond

	script := writeTestScript(t, `TEST "Long Test"
    QUERY "PUMP-01" "pump_status" status TIMEOUT 5000
    DELAY 10000
    PASS "done"
ENDTEST`)

	st.CreateEmployee("emp-1", "Test User")
	st.CreateRMA("rma-1", "RMA-001", "SN1", "Customer", "CT-8", "emp-1", "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewWithFactory(ctx, st, nil, func(station string) executor.DeviceRouter {
		return router
	})

	if mgr.HasActiveTestForRMA("rma-1") {
		t.Error("expected no active test before start")
	}

	mgr.StartTest("station-01", "PUMP-01", script, "rma-1", "run-1", "emp-1")
	time.Sleep(100 * time.Millisecond)

	if !mgr.HasActiveTestForRMA("rma-1") {
		t.Error("expected active test for rma-1")
	}
	if mgr.HasActiveTestForRMA("rma-2") {
		t.Error("expected no active test for rma-2")
	}
}
