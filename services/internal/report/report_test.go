package report

import (
	"bytes"
	"compress/zlib"
	"encoding/csv"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/holla2040/arturo/internal/store"
)

// extractPDFText decompresses all zlib-compressed streams in raw PDF bytes
// and returns the concatenated decompressed content for text searching.
func extractPDFText(data []byte) []byte {
	var result []byte
	streamTag := []byte("stream\n")
	endTag := []byte("\nendstream")
	for {
		start := bytes.Index(data, streamTag)
		if start == -1 {
			break
		}
		data = data[start+len(streamTag):]
		end := bytes.Index(data, endTag)
		if end == -1 {
			break
		}
		compressed := bytes.TrimRight(data[:end], "\r\n ")
		r, err := zlib.NewReader(bytes.NewReader(compressed))
		if err == nil {
			decompressed, err := io.ReadAll(r)
			r.Close()
			if err == nil {
				result = append(result, decompressed...)
			}
		}
		data = data[end+len(endTag):]
	}
	return result
}

// seedFinishedTestData creates a test run and marks it finished with measurements.
func seedFinishedTestData(t *testing.T, s *store.Store) {
	t.Helper()
	seedTestData(t, s)
	if err := s.FinishTestRun("run-1", "passed", "All tests passed"); err != nil {
		t.Fatalf("failed to finish test run: %v", err)
	}
}

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

func TestExportPDF_NoMeasurements(t *testing.T) {
	s := newTestStore(t)
	if err := s.CreateTestRun("run-empty", "empty.art"); err != nil {
		t.Fatalf("failed to create test run: %v", err)
	}

	var buf bytes.Buffer
	if err := ExportPDF(&buf, s, "run-empty"); err != nil {
		t.Fatalf("ExportPDF returned error: %v", err)
	}

	data := buf.Bytes()
	if len(data) < 5 || string(data[:5]) != "%PDF-" {
		t.Fatalf("output does not start with PDF header, got %q", string(data[:min(20, len(data))]))
	}
}

func TestExportPDF_WithMeasurements(t *testing.T) {
	s := newTestStore(t)
	seedFinishedTestData(t, s)

	var buf bytes.Buffer
	if err := ExportPDF(&buf, s, "run-1"); err != nil {
		t.Fatalf("ExportPDF returned error: %v", err)
	}

	data := buf.Bytes()
	if len(data) < 5 || string(data[:5]) != "%PDF-" {
		t.Fatalf("output does not start with PDF header, got %q", string(data[:min(20, len(data))]))
	}
	if len(data) < 100 {
		t.Errorf("PDF output seems too small: %d bytes", len(data))
	}
}

func TestExportPDF_ContainsTestRunID(t *testing.T) {
	s := newTestStore(t)
	seedFinishedTestData(t, s)

	var buf bytes.Buffer
	if err := ExportPDF(&buf, s, "run-1"); err != nil {
		t.Fatalf("ExportPDF returned error: %v", err)
	}

	text := extractPDFText(buf.Bytes())
	if !bytes.Contains(text, []byte("run-1")) {
		t.Error("PDF does not contain test run ID 'run-1'")
	}
}

func TestExportPDF_ContainsDeviceIDs(t *testing.T) {
	s := newTestStore(t)
	seedFinishedTestData(t, s)

	var buf bytes.Buffer
	if err := ExportPDF(&buf, s, "run-1"); err != nil {
		t.Fatalf("ExportPDF returned error: %v", err)
	}

	text := extractPDFText(buf.Bytes())
	if !bytes.Contains(text, []byte("fluke-8846a")) {
		t.Error("PDF does not contain device ID 'fluke-8846a'")
	}
	if !bytes.Contains(text, []byte("relay-board-01")) {
		t.Error("PDF does not contain device ID 'relay-board-01'")
	}
}

func TestExportPDF_ContainsPassFail(t *testing.T) {
	s := newTestStore(t)
	seedFinishedTestData(t, s)

	var buf bytes.Buffer
	if err := ExportPDF(&buf, s, "run-1"); err != nil {
		t.Fatalf("ExportPDF returned error: %v", err)
	}

	text := extractPDFText(buf.Bytes())
	if !bytes.Contains(text, []byte("[PASS]")) {
		t.Error("PDF does not contain '[PASS]'")
	}
	if !bytes.Contains(text, []byte("[FAIL]")) {
		t.Error("PDF does not contain '[FAIL]'")
	}
}

func TestExportPDF_NonexistentTestRun(t *testing.T) {
	s := newTestStore(t)

	var buf bytes.Buffer
	err := ExportPDF(&buf, s, "no-such-run")
	if err == nil {
		t.Fatal("expected error for nonexistent test run, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestExportPDF_SizeLargerWithData(t *testing.T) {
	s := newTestStore(t)
	if err := s.CreateTestRun("run-empty", "empty.art"); err != nil {
		t.Fatalf("failed to create test run: %v", err)
	}
	seedFinishedTestData(t, s)

	var emptyBuf bytes.Buffer
	if err := ExportPDF(&emptyBuf, s, "run-empty"); err != nil {
		t.Fatalf("ExportPDF (empty) returned error: %v", err)
	}

	var dataBuf bytes.Buffer
	if err := ExportPDF(&dataBuf, s, "run-1"); err != nil {
		t.Fatalf("ExportPDF (data) returned error: %v", err)
	}

	if dataBuf.Len() <= emptyBuf.Len() {
		t.Errorf("PDF with data (%d bytes) should be larger than empty PDF (%d bytes)", dataBuf.Len(), emptyBuf.Len())
	}
}
