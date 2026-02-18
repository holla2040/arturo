package store

import (
	"testing"
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
