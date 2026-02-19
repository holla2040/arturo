package report

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/holla2040/arturo/internal/store"
)

// MeasurementJSON is the JSON representation of a measurement for export.
type MeasurementJSON struct {
	DeviceID    string `json:"device_id"`
	CommandName string `json:"command_name"`
	Success     bool   `json:"success"`
	Response    string `json:"response"`
	DurationMs  int    `json:"duration_ms"`
	Timestamp   string `json:"timestamp"`
}

// ExportCSV writes measurement data as CSV to w.
// Headers: device_id,command_name,success,response,duration_ms,timestamp
func ExportCSV(w io.Writer, s *store.Store, testRunID string) error {
	measurements, err := s.QueryMeasurements(testRunID)
	if err != nil {
		return err
	}

	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"device_id", "command_name", "success", "response", "duration_ms", "timestamp"}); err != nil {
		return err
	}

	for _, m := range measurements {
		record := []string{
			m.DeviceID,
			m.CommandName,
			strconv.FormatBool(m.Success),
			m.Response,
			strconv.Itoa(m.DurationMs),
			m.Timestamp.Format(time.RFC3339),
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}

	cw.Flush()
	return cw.Error()
}

// ExportJSON writes measurement data as a JSON array to w.
func ExportJSON(w io.Writer, s *store.Store, testRunID string) error {
	measurements, err := s.QueryMeasurements(testRunID)
	if err != nil {
		return err
	}

	records := make([]MeasurementJSON, len(measurements))
	for i, m := range measurements {
		records[i] = MeasurementJSON{
			DeviceID:    m.DeviceID,
			CommandName: m.CommandName,
			Success:     m.Success,
			Response:    m.Response,
			DurationMs:  m.DurationMs,
			Timestamp:   m.Timestamp.Format(time.RFC3339),
		}
	}

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}

	_, err = w.Write(data)
	return err
}

// ExportPDF writes a formatted PDF test report to w.
func ExportPDF(w io.Writer, s *store.Store, testRunID string) error {
	run, err := s.GetTestRun(testRunID)
	if err != nil {
		return fmt.Errorf("failed to get test run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("test run %q not found", testRunID)
	}

	measurements, err := s.QueryMeasurements(testRunID)
	if err != nil {
		return fmt.Errorf("failed to query measurements: %w", err)
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.AddPage()

	pdfHeader(pdf, run)
	pdfSummary(pdf, run)
	pdfMeasurements(pdf, measurements)
	pdfFooter(pdf)

	if pdf.Err() {
		return fmt.Errorf("PDF generation error: %w", pdf.Error())
	}
	return pdf.Output(w)
}

func pdfHeader(pdf *fpdf.Fpdf, run *store.TestRun) {
	// Dark banner
	pdf.SetFillColor(33, 37, 41)
	pdf.Rect(15, 15, 180, 20, "F")
	pdf.SetFont("Helvetica", "B", 16)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetXY(20, 18)
	pdf.CellFormat(170, 14, "ARTURO TEST REPORT", "", 0, "L", false, 0, "")

	pdf.Ln(25)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(30, 6, "Test Run ID:", "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "B", 10)
	pdf.CellFormat(0, 6, run.ID, "", 1, "L", false, 0, "")

	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(30, 6, "Script:", "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "B", 10)
	pdf.CellFormat(0, 6, run.ScriptName, "", 1, "L", false, 0, "")

	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(30, 6, "Generated:", "", 0, "L", false, 0, "")
	pdf.CellFormat(0, 6, time.Now().UTC().Format("2006-01-02 15:04:05 UTC"), "", 1, "L", false, 0, "")

	pdf.Ln(4)
}

func pdfSummary(pdf *fpdf.Fpdf, run *store.TestRun) {
	pdf.SetFont("Helvetica", "B", 12)
	pdf.CellFormat(0, 8, "Summary", "", 1, "L", false, 0, "")
	pdf.SetDrawColor(200, 200, 200)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(3)

	// Status with color tag
	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(30, 6, "Status:", "", 0, "L", false, 0, "")
	switch run.Status {
	case "passed":
		pdf.SetFillColor(40, 167, 69)
		pdf.SetTextColor(255, 255, 255)
		pdf.CellFormat(20, 6, "[PASS]", "", 0, "C", true, 0, "")
	case "failed":
		pdf.SetFillColor(220, 53, 69)
		pdf.SetTextColor(255, 255, 255)
		pdf.CellFormat(20, 6, "[FAIL]", "", 0, "C", true, 0, "")
	default:
		pdf.SetFont("Helvetica", "I", 10)
		pdf.CellFormat(20, 6, run.Status, "", 0, "L", false, 0, "")
	}
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Helvetica", "", 10)
	pdf.Ln(8)

	// Duration
	pdf.CellFormat(30, 6, "Duration:", "", 0, "L", false, 0, "")
	if run.FinishedAt != nil {
		d := run.FinishedAt.Sub(run.StartedAt).Round(time.Millisecond)
		pdf.CellFormat(0, 6, d.String(), "", 1, "L", false, 0, "")
	} else {
		pdf.SetFont("Helvetica", "I", 10)
		pdf.CellFormat(0, 6, "In progress", "", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 10)
	}

	// Start time
	pdf.CellFormat(30, 6, "Started:", "", 0, "L", false, 0, "")
	pdf.CellFormat(0, 6, run.StartedAt.Format("2006-01-02 15:04:05 UTC"), "", 1, "L", false, 0, "")

	// End time
	pdf.CellFormat(30, 6, "Finished:", "", 0, "L", false, 0, "")
	if run.FinishedAt != nil {
		pdf.CellFormat(0, 6, run.FinishedAt.Format("2006-01-02 15:04:05 UTC"), "", 1, "L", false, 0, "")
	} else {
		pdf.SetFont("Helvetica", "I", 10)
		pdf.CellFormat(0, 6, "In progress", "", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 10)
	}

	// Summary text
	if run.Summary != "" {
		pdf.CellFormat(30, 6, "Notes:", "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 6, run.Summary, "", 1, "L", false, 0, "")
	}

	pdf.Ln(6)
}

func pdfMeasurements(pdf *fpdf.Fpdf, measurements []store.Measurement) {
	pdf.SetFont("Helvetica", "B", 12)
	pdf.CellFormat(0, 8, "Measurements", "", 1, "L", false, 0, "")
	pdf.SetDrawColor(200, 200, 200)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(3)

	if len(measurements) == 0 {
		pdf.SetFont("Helvetica", "I", 10)
		pdf.CellFormat(0, 8, "No measurements recorded", "", 1, "C", false, 0, "")
		return
	}

	// Column widths (total 180mm)
	colW := []float64{10, 32, 35, 14, 40, 22, 27}
	headers := []string{"#", "Device ID", "Command", "Pass", "Response", "Duration", "Timestamp"}

	// Table header
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetFillColor(240, 240, 240)
	for i, h := range headers {
		pdf.CellFormat(colW[i], 7, h, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)

	// Table rows
	pdf.SetFont("Helvetica", "", 7)
	for i, m := range measurements {
		// Alternating row fill
		if i%2 == 1 {
			pdf.SetFillColor(248, 249, 250)
		} else {
			pdf.SetFillColor(255, 255, 255)
		}
		fill := true

		pdf.CellFormat(colW[0], 6, strconv.Itoa(i+1), "1", 0, "C", fill, 0, "")
		pdf.CellFormat(colW[1], 6, truncate(m.DeviceID, 20), "1", 0, "L", fill, 0, "")
		pdf.CellFormat(colW[2], 6, truncate(m.CommandName, 22), "1", 0, "L", fill, 0, "")

		passText := "[FAIL]"
		if m.Success {
			passText = "[PASS]"
		}
		pdf.CellFormat(colW[3], 6, passText, "1", 0, "C", fill, 0, "")
		pdf.CellFormat(colW[4], 6, truncate(m.Response, 20), "1", 0, "L", fill, 0, "")
		pdf.CellFormat(colW[5], 6, fmt.Sprintf("%dms", m.DurationMs), "1", 0, "R", fill, 0, "")
		pdf.CellFormat(colW[6], 6, m.Timestamp.Format("15:04:05"), "1", 0, "C", fill, 0, "")
		pdf.Ln(-1)
	}
}

func pdfFooter(pdf *fpdf.Fpdf) {
	pdf.Ln(10)
	pdf.SetFont("Helvetica", "I", 8)
	pdf.SetTextColor(150, 150, 150)
	pdf.CellFormat(0, 6, "Generated by Arturo Test Automation System", "", 0, "C", false, 0, "")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
