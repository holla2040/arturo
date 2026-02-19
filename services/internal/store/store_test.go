package store

import (
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New(:memory:) failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNewCreatesStore(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer s.Close()
}

func TestCreateAndQueryTestRun(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateTestRun("run-1", "test_script.art"); err != nil {
		t.Fatalf("CreateTestRun failed: %v", err)
	}

	runs, err := s.QueryTestRuns()
	if err != nil {
		t.Fatalf("QueryTestRuns failed: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].ID != "run-1" {
		t.Errorf("expected ID run-1, got %s", runs[0].ID)
	}
	if runs[0].ScriptName != "test_script.art" {
		t.Errorf("expected script test_script.art, got %s", runs[0].ScriptName)
	}
	if runs[0].Status != "running" {
		t.Errorf("expected status running, got %s", runs[0].Status)
	}
	if runs[0].FinishedAt != nil {
		t.Errorf("expected nil FinishedAt, got %v", runs[0].FinishedAt)
	}
}

func TestMultipleTestRunsReturnedInOrder(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateTestRun("run-a", "first.art"); err != nil {
		t.Fatalf("CreateTestRun failed: %v", err)
	}
	if err := s.CreateTestRun("run-b", "second.art"); err != nil {
		t.Fatalf("CreateTestRun failed: %v", err)
	}

	runs, err := s.QueryTestRuns()
	if err != nil {
		t.Fatalf("QueryTestRuns failed: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
	// ORDER BY started_at DESC, so most recent first
	// Both were created at nearly the same time, but run-b was inserted second
	// With identical timestamps SQLite preserves insertion order, so DESC gives run-b first
	if runs[0].ID != "run-b" {
		t.Errorf("expected first result run-b, got %s", runs[0].ID)
	}
	if runs[1].ID != "run-a" {
		t.Errorf("expected second result run-a, got %s", runs[1].ID)
	}
}

func TestFinishTestRun(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateTestRun("run-1", "test.art"); err != nil {
		t.Fatalf("CreateTestRun failed: %v", err)
	}
	if err := s.FinishTestRun("run-1", "passed", "all checks OK"); err != nil {
		t.Fatalf("FinishTestRun failed: %v", err)
	}

	run, err := s.GetTestRun("run-1")
	if err != nil {
		t.Fatalf("GetTestRun failed: %v", err)
	}
	if run.Status != "passed" {
		t.Errorf("expected status passed, got %s", run.Status)
	}
	if run.Summary != "all checks OK" {
		t.Errorf("expected summary 'all checks OK', got %s", run.Summary)
	}
	if run.FinishedAt == nil {
		t.Error("expected non-nil FinishedAt after finishing")
	}
}

func TestGetTestRunExists(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateTestRun("run-1", "test.art"); err != nil {
		t.Fatalf("CreateTestRun failed: %v", err)
	}

	run, err := s.GetTestRun("run-1")
	if err != nil {
		t.Fatalf("GetTestRun failed: %v", err)
	}
	if run == nil {
		t.Fatal("expected non-nil run")
	}
	if run.ID != "run-1" {
		t.Errorf("expected ID run-1, got %s", run.ID)
	}
}

func TestGetTestRunNotFound(t *testing.T) {
	s := newTestStore(t)

	run, err := s.GetTestRun("nonexistent")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if run != nil {
		t.Errorf("expected nil for unknown ID, got %+v", run)
	}
}

func TestRecordAndQueryMeasurements(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateTestRun("run-1", "test.art"); err != nil {
		t.Fatalf("CreateTestRun failed: %v", err)
	}
	if err := s.RecordCommandResult("run-1", "dmm-01", "MEAS:VOLT?", true, "12.345", 150); err != nil {
		t.Fatalf("RecordCommandResult failed: %v", err)
	}

	measurements, err := s.QueryMeasurements("run-1")
	if err != nil {
		t.Fatalf("QueryMeasurements failed: %v", err)
	}
	if len(measurements) != 1 {
		t.Fatalf("expected 1 measurement, got %d", len(measurements))
	}
	m := measurements[0]
	if m.TestRunID != "run-1" {
		t.Errorf("expected test_run_id run-1, got %s", m.TestRunID)
	}
	if m.DeviceID != "dmm-01" {
		t.Errorf("expected device_id dmm-01, got %s", m.DeviceID)
	}
	if m.CommandName != "MEAS:VOLT?" {
		t.Errorf("expected command MEAS:VOLT?, got %s", m.CommandName)
	}
	if !m.Success {
		t.Error("expected success=true")
	}
	if m.Response != "12.345" {
		t.Errorf("expected response 12.345, got %s", m.Response)
	}
	if m.DurationMs != 150 {
		t.Errorf("expected duration 150, got %d", m.DurationMs)
	}
}

func TestMultipleMeasurementsForSameRun(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateTestRun("run-1", "test.art"); err != nil {
		t.Fatalf("CreateTestRun failed: %v", err)
	}
	if err := s.RecordCommandResult("run-1", "dmm-01", "MEAS:VOLT?", true, "12.0", 100); err != nil {
		t.Fatalf("RecordCommandResult failed: %v", err)
	}
	if err := s.RecordCommandResult("run-1", "dmm-01", "MEAS:CURR?", true, "0.5", 200); err != nil {
		t.Fatalf("RecordCommandResult failed: %v", err)
	}

	measurements, err := s.QueryMeasurements("run-1")
	if err != nil {
		t.Fatalf("QueryMeasurements failed: %v", err)
	}
	if len(measurements) != 2 {
		t.Fatalf("expected 2 measurements, got %d", len(measurements))
	}
}

func TestQueryMeasurementsEmptyForUnknownRun(t *testing.T) {
	s := newTestStore(t)

	measurements, err := s.QueryMeasurements("nonexistent")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if measurements == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(measurements) != 0 {
		t.Errorf("expected 0 measurements, got %d", len(measurements))
	}
}

func TestRecordDeviceEvent(t *testing.T) {
	s := newTestStore(t)

	if err := s.RecordDeviceEvent("dmm-01", "station-1", "connected", "initial connection"); err != nil {
		t.Fatalf("RecordDeviceEvent failed: %v", err)
	}
}

func TestSuccessBoolStoredCorrectly(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateTestRun("run-1", "test.art"); err != nil {
		t.Fatalf("CreateTestRun failed: %v", err)
	}
	if err := s.RecordCommandResult("run-1", "dev-1", "CMD1", true, "", 0); err != nil {
		t.Fatalf("RecordCommandResult (true) failed: %v", err)
	}
	if err := s.RecordCommandResult("run-1", "dev-1", "CMD2", false, "", 0); err != nil {
		t.Fatalf("RecordCommandResult (false) failed: %v", err)
	}

	measurements, err := s.QueryMeasurements("run-1")
	if err != nil {
		t.Fatalf("QueryMeasurements failed: %v", err)
	}
	if len(measurements) != 2 {
		t.Fatalf("expected 2 measurements, got %d", len(measurements))
	}
	if !measurements[0].Success {
		t.Error("expected first measurement success=true")
	}
	if measurements[1].Success {
		t.Error("expected second measurement success=false")
	}
}

func TestDurationAndResponseStored(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateTestRun("run-1", "test.art"); err != nil {
		t.Fatalf("CreateTestRun failed: %v", err)
	}
	if err := s.RecordCommandResult("run-1", "dev-1", "LONG:CMD", true, "detailed response data", 5000); err != nil {
		t.Fatalf("RecordCommandResult failed: %v", err)
	}

	measurements, err := s.QueryMeasurements("run-1")
	if err != nil {
		t.Fatalf("QueryMeasurements failed: %v", err)
	}
	if len(measurements) != 1 {
		t.Fatalf("expected 1 measurement, got %d", len(measurements))
	}
	if measurements[0].Response != "detailed response data" {
		t.Errorf("expected response 'detailed response data', got %s", measurements[0].Response)
	}
	if measurements[0].DurationMs != 5000 {
		t.Errorf("expected duration 5000, got %d", measurements[0].DurationMs)
	}
}

func TestCloseSucceeds(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Employee tests
// ---------------------------------------------------------------------------

func TestCreateAndGetEmployee(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateEmployee("emp-1", "John Doe"); err != nil {
		t.Fatalf("CreateEmployee failed: %v", err)
	}

	emp, err := s.GetEmployee("emp-1")
	if err != nil {
		t.Fatalf("GetEmployee failed: %v", err)
	}
	if emp == nil {
		t.Fatal("expected non-nil employee")
	}
	if emp.ID != "emp-1" {
		t.Errorf("expected ID emp-1, got %s", emp.ID)
	}
	if emp.Name != "John Doe" {
		t.Errorf("expected name John Doe, got %s", emp.Name)
	}
}

func TestUpsertEmployee(t *testing.T) {
	s := newTestStore(t)

	if err := s.UpsertEmployee("emp-1", "John Doe"); err != nil {
		t.Fatalf("UpsertEmployee (create) failed: %v", err)
	}
	if err := s.UpsertEmployee("emp-1", "John Updated"); err != nil {
		t.Fatalf("UpsertEmployee (update) failed: %v", err)
	}

	emp, err := s.GetEmployee("emp-1")
	if err != nil {
		t.Fatalf("GetEmployee failed: %v", err)
	}
	if emp.Name != "John Updated" {
		t.Errorf("expected updated name, got %s", emp.Name)
	}
}

func TestGetEmployeeNotFound(t *testing.T) {
	s := newTestStore(t)

	emp, err := s.GetEmployee("nonexistent")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if emp != nil {
		t.Errorf("expected nil for unknown ID, got %+v", emp)
	}
}

func TestListEmployees(t *testing.T) {
	s := newTestStore(t)

	s.CreateEmployee("emp-2", "Bob Smith")
	s.CreateEmployee("emp-1", "Alice Jones")

	employees, err := s.ListEmployees()
	if err != nil {
		t.Fatalf("ListEmployees failed: %v", err)
	}
	if len(employees) != 2 {
		t.Fatalf("expected 2 employees, got %d", len(employees))
	}
	// Ordered by name ASC
	if employees[0].Name != "Alice Jones" {
		t.Errorf("expected first employee Alice Jones, got %s", employees[0].Name)
	}
}

// ---------------------------------------------------------------------------
// RMA tests
// ---------------------------------------------------------------------------

func TestCreateAndGetRMA(t *testing.T) {
	s := newTestStore(t)

	s.CreateEmployee("emp-1", "Test User")

	if err := s.CreateRMA("rma-1", "RMA-2024-001", "SN12345", "ACME Corp", "CT-8", "emp-1", "initial repair"); err != nil {
		t.Fatalf("CreateRMA failed: %v", err)
	}

	rma, err := s.GetRMA("rma-1")
	if err != nil {
		t.Fatalf("GetRMA failed: %v", err)
	}
	if rma == nil {
		t.Fatal("expected non-nil RMA")
	}
	if rma.RMANumber != "RMA-2024-001" {
		t.Errorf("expected RMA number RMA-2024-001, got %s", rma.RMANumber)
	}
	if rma.PumpSerialNumber != "SN12345" {
		t.Errorf("expected pump serial SN12345, got %s", rma.PumpSerialNumber)
	}
	if rma.CustomerName != "ACME Corp" {
		t.Errorf("expected customer ACME Corp, got %s", rma.CustomerName)
	}
	if rma.PumpModel != "CT-8" {
		t.Errorf("expected model CT-8, got %s", rma.PumpModel)
	}
	if rma.Status != "open" {
		t.Errorf("expected status open, got %s", rma.Status)
	}
	if rma.ClosedAt != nil {
		t.Error("expected nil ClosedAt for open RMA")
	}
}

func TestGetRMAByNumber(t *testing.T) {
	s := newTestStore(t)
	s.CreateEmployee("emp-1", "Test User")
	s.CreateRMA("rma-1", "RMA-2024-001", "SN12345", "ACME Corp", "CT-8", "emp-1", "")

	rma, err := s.GetRMAByNumber("RMA-2024-001")
	if err != nil {
		t.Fatalf("GetRMAByNumber failed: %v", err)
	}
	if rma == nil {
		t.Fatal("expected non-nil RMA")
	}
	if rma.ID != "rma-1" {
		t.Errorf("expected ID rma-1, got %s", rma.ID)
	}
}

func TestCloseRMA(t *testing.T) {
	s := newTestStore(t)
	s.CreateEmployee("emp-1", "Test User")
	s.CreateRMA("rma-1", "RMA-2024-001", "SN12345", "ACME Corp", "CT-8", "emp-1", "")

	if err := s.CloseRMA("rma-1"); err != nil {
		t.Fatalf("CloseRMA failed: %v", err)
	}

	rma, err := s.GetRMA("rma-1")
	if err != nil {
		t.Fatalf("GetRMA failed: %v", err)
	}
	if rma.Status != "closed" {
		t.Errorf("expected status closed, got %s", rma.Status)
	}
	if rma.ClosedAt == nil {
		t.Error("expected non-nil ClosedAt after closing")
	}
}

func TestListRMAsFilterByStatus(t *testing.T) {
	s := newTestStore(t)
	s.CreateEmployee("emp-1", "Test User")
	s.CreateRMA("rma-1", "RMA-001", "SN1", "Customer 1", "CT-8", "emp-1", "")
	s.CreateRMA("rma-2", "RMA-002", "SN2", "Customer 2", "CT-8", "emp-1", "")
	s.CloseRMA("rma-2")

	open, err := s.ListRMAs("open")
	if err != nil {
		t.Fatalf("ListRMAs(open) failed: %v", err)
	}
	if len(open) != 1 {
		t.Errorf("expected 1 open RMA, got %d", len(open))
	}

	closed, err := s.ListRMAs("closed")
	if err != nil {
		t.Fatalf("ListRMAs(closed) failed: %v", err)
	}
	if len(closed) != 1 {
		t.Errorf("expected 1 closed RMA, got %d", len(closed))
	}

	all, err := s.ListRMAs("")
	if err != nil {
		t.Fatalf("ListRMAs() failed: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 total RMAs, got %d", len(all))
	}
}

func TestSearchRMAs(t *testing.T) {
	s := newTestStore(t)
	s.CreateEmployee("emp-1", "Test User")
	s.CreateRMA("rma-1", "RMA-001", "SN-AAA", "ACME Corp", "CT-8", "emp-1", "")
	s.CreateRMA("rma-2", "RMA-002", "SN-BBB", "Beta Inc", "CT-10", "emp-1", "")

	results, err := s.SearchRMAs("ACME")
	if err != nil {
		t.Fatalf("SearchRMAs failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'ACME', got %d", len(results))
	}

	results, err = s.SearchRMAs("SN-")
	if err != nil {
		t.Fatalf("SearchRMAs failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'SN-', got %d", len(results))
	}
}

func TestDuplicateRMANumberRejected(t *testing.T) {
	s := newTestStore(t)
	s.CreateEmployee("emp-1", "Test User")
	s.CreateRMA("rma-1", "RMA-001", "SN1", "Customer", "CT-8", "emp-1", "")

	err := s.CreateRMA("rma-2", "RMA-001", "SN2", "Other", "CT-8", "emp-1", "")
	if err == nil {
		t.Fatal("expected error for duplicate RMA number")
	}
}

// ---------------------------------------------------------------------------
// Station State tests
// ---------------------------------------------------------------------------

func TestSetAndGetStationState(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetStationState("station-01", "idle", nil); err != nil {
		t.Fatalf("SetStationState failed: %v", err)
	}

	ss, err := s.GetStationState("station-01")
	if err != nil {
		t.Fatalf("GetStationState failed: %v", err)
	}
	if ss == nil {
		t.Fatal("expected non-nil station state")
	}
	if ss.State != "idle" {
		t.Errorf("expected state idle, got %s", ss.State)
	}
	if ss.CurrentTestRunID != nil {
		t.Error("expected nil test run ID")
	}
}

func TestStationStateUpsert(t *testing.T) {
	s := newTestStore(t)

	s.SetStationState("station-01", "idle", nil)

	runID := "run-1"
	s.SetStationState("station-01", "testing", &runID)

	ss, err := s.GetStationState("station-01")
	if err != nil {
		t.Fatalf("GetStationState failed: %v", err)
	}
	if ss.State != "testing" {
		t.Errorf("expected state testing, got %s", ss.State)
	}
	if ss.CurrentTestRunID == nil || *ss.CurrentTestRunID != "run-1" {
		t.Error("expected test run ID run-1")
	}
}

func TestListStationStates(t *testing.T) {
	s := newTestStore(t)

	s.SetStationState("station-01", "idle", nil)
	s.SetStationState("station-02", "testing", nil)

	states, err := s.ListStationStates()
	if err != nil {
		t.Fatalf("ListStationStates failed: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("expected 2 station states, got %d", len(states))
	}
}

// ---------------------------------------------------------------------------
// Temperature Sample tests
// ---------------------------------------------------------------------------

func TestRecordAndQueryTemperatures(t *testing.T) {
	s := newTestStore(t)
	s.CreateTestRun("run-1", "test.art")

	if err := s.RecordTemperature("run-1", "station-01", "PUMP-01", "first_stage", 77.5); err != nil {
		t.Fatalf("RecordTemperature failed: %v", err)
	}
	if err := s.RecordTemperature("run-1", "station-01", "PUMP-01", "second_stage", 15.2); err != nil {
		t.Fatalf("RecordTemperature failed: %v", err)
	}

	temps, err := s.QueryTemperatures("run-1")
	if err != nil {
		t.Fatalf("QueryTemperatures failed: %v", err)
	}
	if len(temps) != 2 {
		t.Fatalf("expected 2 temperature samples, got %d", len(temps))
	}
	if temps[0].Stage != "first_stage" {
		t.Errorf("expected first_stage, got %s", temps[0].Stage)
	}
	if temps[0].TemperatureK != 77.5 {
		t.Errorf("expected 77.5K, got %f", temps[0].TemperatureK)
	}
	if temps[1].Stage != "second_stage" {
		t.Errorf("expected second_stage, got %s", temps[1].Stage)
	}
}

func TestQueryTemperaturesSince(t *testing.T) {
	s := newTestStore(t)
	s.CreateTestRun("run-1", "test.art")

	s.RecordTemperature("run-1", "station-01", "PUMP-01", "first_stage", 200.0)
	time.Sleep(10 * time.Millisecond)
	cutoff := time.Now().UTC()
	time.Sleep(10 * time.Millisecond)
	s.RecordTemperature("run-1", "station-01", "PUMP-01", "first_stage", 150.0)

	temps, err := s.QueryTemperaturesSince("run-1", cutoff)
	if err != nil {
		t.Fatalf("QueryTemperaturesSince failed: %v", err)
	}
	if len(temps) != 1 {
		t.Fatalf("expected 1 temperature sample since cutoff, got %d", len(temps))
	}
	if temps[0].TemperatureK != 150.0 {
		t.Errorf("expected 150.0K, got %f", temps[0].TemperatureK)
	}
}

// ---------------------------------------------------------------------------
// Test Event tests
// ---------------------------------------------------------------------------

func TestRecordAndQueryTestEvents(t *testing.T) {
	s := newTestStore(t)
	s.CreateTestRun("run-1", "test.art")

	if err := s.RecordTestEvent("run-1", "started", "emp-1", ""); err != nil {
		t.Fatalf("RecordTestEvent failed: %v", err)
	}
	if err := s.RecordTestEvent("run-1", "paused", "emp-1", "checking something"); err != nil {
		t.Fatalf("RecordTestEvent failed: %v", err)
	}

	events, err := s.QueryTestEvents("run-1")
	if err != nil {
		t.Fatalf("QueryTestEvents failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 test events, got %d", len(events))
	}
	if events[0].EventType != "started" {
		t.Errorf("expected event type started, got %s", events[0].EventType)
	}
	if events[1].Reason != "checking something" {
		t.Errorf("expected reason 'checking something', got %s", events[1].Reason)
	}
}

// ---------------------------------------------------------------------------
// CreateTestRunWithRMA tests
// ---------------------------------------------------------------------------

func TestCreateTestRunWithRMA(t *testing.T) {
	s := newTestStore(t)
	s.CreateEmployee("emp-1", "Test User")
	s.CreateRMA("rma-1", "RMA-001", "SN1", "Customer", "CT-8", "emp-1", "")

	if err := s.CreateTestRunWithRMA("run-1", "test.art", "rma-1", "station-01", "abc123", "TEST \"hello\"\nENDTEST"); err != nil {
		t.Fatalf("CreateTestRunWithRMA failed: %v", err)
	}

	run, err := s.GetTestRun("run-1")
	if err != nil {
		t.Fatalf("GetTestRun failed: %v", err)
	}
	if run.RMAID == nil || *run.RMAID != "rma-1" {
		t.Error("expected RMAID rma-1")
	}
	if run.StationInstance == nil || *run.StationInstance != "station-01" {
		t.Error("expected StationInstance station-01")
	}
	if run.ScriptSHA256 == nil || *run.ScriptSHA256 != "abc123" {
		t.Error("expected ScriptSHA256 abc123")
	}
}

func TestQueryTestRunsByRMA(t *testing.T) {
	s := newTestStore(t)
	s.CreateEmployee("emp-1", "Test User")
	s.CreateRMA("rma-1", "RMA-001", "SN1", "Customer", "CT-8", "emp-1", "")

	s.CreateTestRunWithRMA("run-1", "test1.art", "rma-1", "station-01", "hash1", "")
	s.CreateTestRunWithRMA("run-2", "test2.art", "rma-1", "station-01", "hash2", "")
	s.CreateTestRun("run-3", "unrelated.art") // not linked to RMA

	runs, err := s.QueryTestRunsByRMA("rma-1")
	if err != nil {
		t.Fatalf("QueryTestRunsByRMA failed: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs for RMA, got %d", len(runs))
	}
}

func TestDeleteTestRun(t *testing.T) {
	s := newTestStore(t)
	s.CreateTestRun("run-1", "test.art")
	s.RecordCommandResult("run-1", "dev-1", "CMD", true, "ok", 100)
	s.RecordTemperature("run-1", "station-01", "PUMP-01", "first_stage", 77.0)
	s.RecordTestEvent("run-1", "started", "emp-1", "")

	if err := s.DeleteTestRun("run-1"); err != nil {
		t.Fatalf("DeleteTestRun failed: %v", err)
	}

	run, err := s.GetTestRun("run-1")
	if err != nil {
		t.Fatalf("GetTestRun failed: %v", err)
	}
	if run != nil {
		t.Error("expected nil run after delete")
	}

	temps, _ := s.QueryTemperatures("run-1")
	if len(temps) != 0 {
		t.Error("expected no temperatures after delete")
	}

	events, _ := s.QueryTestEvents("run-1")
	if len(events) != 0 {
		t.Error("expected no events after delete")
	}

	measurements, _ := s.QueryMeasurements("run-1")
	if len(measurements) != 0 {
		t.Error("expected no measurements after delete")
	}
}

func TestMigrationIdempotent(t *testing.T) {
	// Creating a store twice on the same DB should not error
	s1, err := New(":memory:")
	if err != nil {
		t.Fatalf("first New failed: %v", err)
	}

	// Run migration again on same DB
	if err := migrateTestRuns(s1.db); err != nil {
		t.Fatalf("second migration failed: %v", err)
	}
	s1.Close()
}
