package report

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"github.com/holla2040/arturo/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func seedTestData(t *testing.T, s *store.Store) {
	t.Helper()
	if err := s.CreateTestRun("run-1", "test.art"); err != nil {
		t.Fatalf("failed to create test run: %v", err)
	}
	if err := s.RecordCommandResult("run-1", "fluke-8846a", "measure_dc_voltage", true, "1.234", 150); err != nil {
		t.Fatalf("failed to record command result: %v", err)
	}
	if err := s.RecordCommandResult("run-1", "relay-board-01", "set_relay", false, "", 50); err != nil {
		t.Fatalf("failed to record command result: %v", err)
	}
}

func TestExportCSV_NoMeasurements(t *testing.T) {
	s := newTestStore(t)
	if err := s.CreateTestRun("run-empty", "empty.art"); err != nil {
		t.Fatalf("failed to create test run: %v", err)
	}

	var buf bytes.Buffer
	if err := ExportCSV(&buf, s, "run-empty"); err != nil {
		t.Fatalf("ExportCSV returned error: %v", err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line (header only), got %d", len(lines))
	}
	if lines[0] != "device_id,command_name,success,response,duration_ms,timestamp" {
		t.Errorf("unexpected header: %s", lines[0])
	}
}

func TestExportCSV_WithMeasurements(t *testing.T) {
	s := newTestStore(t)
	seedTestData(t, s)

	var buf bytes.Buffer
	if err := ExportCSV(&buf, s, "run-1"); err != nil {
		t.Fatalf("ExportCSV returned error: %v", err)
	}

	r := csv.NewReader(strings.NewReader(buf.String()))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	// header + 2 data rows
	if len(records) != 3 {
		t.Fatalf("expected 3 rows (1 header + 2 data), got %d", len(records))
	}

	// Check first data row
	row := records[1]
	if row[0] != "fluke-8846a" {
		t.Errorf("device_id: got %q, want %q", row[0], "fluke-8846a")
	}
	if row[1] != "measure_dc_voltage" {
		t.Errorf("command_name: got %q, want %q", row[1], "measure_dc_voltage")
	}
	if row[3] != "1.234" {
		t.Errorf("response: got %q, want %q", row[3], "1.234")
	}
	if row[4] != "150" {
		t.Errorf("duration_ms: got %q, want %q", row[4], "150")
	}
}

func TestExportCSV_RowCount(t *testing.T) {
	s := newTestStore(t)
	seedTestData(t, s)

	var buf bytes.Buffer
	if err := ExportCSV(&buf, s, "run-1"); err != nil {
		t.Fatalf("ExportCSV returned error: %v", err)
	}

	r := csv.NewReader(strings.NewReader(buf.String()))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	// 1 header + 2 data rows = 3 total
	if len(records) != 3 {
		t.Errorf("expected 3 rows, got %d", len(records))
	}
}

func TestExportCSV_SuccessValues(t *testing.T) {
	s := newTestStore(t)
	seedTestData(t, s)

	var buf bytes.Buffer
	if err := ExportCSV(&buf, s, "run-1"); err != nil {
		t.Fatalf("ExportCSV returned error: %v", err)
	}

	r := csv.NewReader(strings.NewReader(buf.String()))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse CSV: %v", err)
	}

	// First measurement: success=true
	if records[1][2] != "true" {
		t.Errorf("row 1 success: got %q, want %q", records[1][2], "true")
	}
	// Second measurement: success=false
	if records[2][2] != "false" {
		t.Errorf("row 2 success: got %q, want %q", records[2][2], "false")
	}
}

func TestExportJSON_NoMeasurements(t *testing.T) {
	s := newTestStore(t)
	if err := s.CreateTestRun("run-empty", "empty.art"); err != nil {
		t.Fatalf("failed to create test run: %v", err)
	}

	var buf bytes.Buffer
	if err := ExportJSON(&buf, s, "run-empty"); err != nil {
		t.Fatalf("ExportJSON returned error: %v", err)
	}

	output := strings.TrimSpace(buf.String())
	if output != "[]" {
		t.Errorf("expected %q, got %q", "[]", output)
	}
}

func TestExportJSON_WithMeasurements(t *testing.T) {
	s := newTestStore(t)
	seedTestData(t, s)

	var buf bytes.Buffer
	if err := ExportJSON(&buf, s, "run-1"); err != nil {
		t.Fatalf("ExportJSON returned error: %v", err)
	}

	var records []MeasurementJSON
	if err := json.Unmarshal(buf.Bytes(), &records); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
}

func TestExportJSON_FieldValues(t *testing.T) {
	s := newTestStore(t)
	seedTestData(t, s)

	var buf bytes.Buffer
	if err := ExportJSON(&buf, s, "run-1"); err != nil {
		t.Fatalf("ExportJSON returned error: %v", err)
	}

	var records []MeasurementJSON
	if err := json.Unmarshal(buf.Bytes(), &records); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	r := records[0]
	if r.DeviceID != "fluke-8846a" {
		t.Errorf("device_id: got %q, want %q", r.DeviceID, "fluke-8846a")
	}
	if r.CommandName != "measure_dc_voltage" {
		t.Errorf("command_name: got %q, want %q", r.CommandName, "measure_dc_voltage")
	}
	if r.Response != "1.234" {
		t.Errorf("response: got %q, want %q", r.Response, "1.234")
	}
	if r.DurationMs != 150 {
		t.Errorf("duration_ms: got %d, want %d", r.DurationMs, 150)
	}
}

func TestExportJSON_SuccessValues(t *testing.T) {
	s := newTestStore(t)
	seedTestData(t, s)

	var buf bytes.Buffer
	if err := ExportJSON(&buf, s, "run-1"); err != nil {
		t.Fatalf("ExportJSON returned error: %v", err)
	}

	var records []MeasurementJSON
	if err := json.Unmarshal(buf.Bytes(), &records); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if records[0].Success != true {
		t.Errorf("record 0 success: got %v, want true", records[0].Success)
	}
	if records[1].Success != false {
		t.Errorf("record 1 success: got %v, want false", records[1].Success)
	}
}
