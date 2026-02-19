package artifact

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/holla2040/arturo/internal/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New failed: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func setupTestData(t *testing.T, st *store.Store) {
	t.Helper()

	st.CreateEmployee("emp-1", "John Doe")
	st.CreateRMA("rma-1", "RMA-2024-001", "SN12345", "ACME Corp", "CT-8", "emp-1", "repair notes")
	st.CreateTestRunWithRMA("run-1", "pump_test.art", "rma-1", "station-01", "abc123def", "TEST \"test\"\nENDTEST")
	st.RecordTestEvent("run-1", "started", "emp-1", "")
	st.RecordTemperature("run-1", "station-01", "PUMP-01", "first_stage", 77.5)
	st.RecordTemperature("run-1", "station-01", "PUMP-01", "second_stage", 15.2)
	st.RecordCommandResult("run-1", "PUMP-01", "pump_status", true, "1", 50)
	st.RecordTestEvent("run-1", "completed", "emp-1", "all passed")
	st.FinishTestRun("run-1", "passed", "all passed")
}

func TestGenerateArtifact(t *testing.T) {
	st := newTestStore(t)
	setupTestData(t, st)

	artifact, err := Generate(st, "rma-1")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if artifact == nil {
		t.Fatal("expected non-nil artifact")
	}

	if artifact.RMANumber != "RMA-2024-001" {
		t.Errorf("expected RMA number RMA-2024-001, got %s", artifact.RMANumber)
	}
	if artifact.PumpSerialNumber != "SN12345" {
		t.Errorf("expected pump serial SN12345, got %s", artifact.PumpSerialNumber)
	}
	if artifact.CustomerName != "ACME Corp" {
		t.Errorf("expected customer ACME Corp, got %s", artifact.CustomerName)
	}
	if artifact.Employee.Name != "John Doe" {
		t.Errorf("expected employee John Doe, got %s", artifact.Employee.Name)
	}
	if len(artifact.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(artifact.Runs))
	}

	run := artifact.Runs[0]
	if run.Status != "passed" {
		t.Errorf("expected status passed, got %s", run.Status)
	}
	if run.ScriptSHA256 != "abc123def" {
		t.Errorf("expected SHA256 abc123def, got %s", run.ScriptSHA256)
	}
	if len(run.Events) != 2 {
		t.Errorf("expected 2 events, got %d", len(run.Events))
	}
	if len(run.Temperatures) != 2 {
		t.Errorf("expected 2 temperature samples, got %d", len(run.Temperatures))
	}
	if len(run.Measurements) != 1 {
		t.Errorf("expected 1 measurement, got %d", len(run.Measurements))
	}
}

func TestGenerateJSON(t *testing.T) {
	st := newTestStore(t)
	setupTestData(t, st)

	var buf bytes.Buffer
	if err := GenerateJSON(&buf, st, "rma-1"); err != nil {
		t.Fatalf("GenerateJSON failed: %v", err)
	}

	// Verify it's valid JSON
	var parsed TestArtifact
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("generated JSON is invalid: %v", err)
	}
	if parsed.RMANumber != "RMA-2024-001" {
		t.Errorf("expected RMA number in JSON, got %s", parsed.RMANumber)
	}
}

func TestGeneratePDF(t *testing.T) {
	st := newTestStore(t)
	setupTestData(t, st)

	var buf bytes.Buffer
	if err := GeneratePDF(&buf, st, "rma-1"); err != nil {
		t.Fatalf("GeneratePDF failed: %v", err)
	}

	// Verify it starts with PDF magic bytes
	if buf.Len() < 4 {
		t.Fatal("PDF output too small")
	}
	if string(buf.Bytes()[:4]) != "%PDF" {
		t.Error("PDF output does not start with %PDF magic bytes")
	}
}

func TestExportToShare(t *testing.T) {
	dir := t.TempDir()

	jsonData := []byte(`{"rma_number":"RMA-001"}`)
	pdfData := []byte("%PDF-1.4 test")

	if err := ExportToShare(jsonData, pdfData, "RMA-001", dir); err != nil {
		t.Fatalf("ExportToShare failed: %v", err)
	}

	// Verify files exist
	jsonPath := filepath.Join(dir, "RMA-001", "RMA-001.json")
	if data, err := os.ReadFile(jsonPath); err != nil {
		t.Fatalf("JSON file not found: %v", err)
	} else if string(data) != string(jsonData) {
		t.Error("JSON content mismatch")
	}

	pdfPath := filepath.Join(dir, "RMA-001", "RMA-001.pdf")
	if data, err := os.ReadFile(pdfPath); err != nil {
		t.Fatalf("PDF file not found: %v", err)
	} else if string(data) != string(pdfData) {
		t.Error("PDF content mismatch")
	}
}

func TestGenerateNotFound(t *testing.T) {
	st := newTestStore(t)

	artifact, err := Generate(st, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if artifact != nil {
		t.Error("expected nil artifact for nonexistent RMA")
	}
}
