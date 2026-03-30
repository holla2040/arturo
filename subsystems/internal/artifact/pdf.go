package artifact

import (
	"fmt"
	"io"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/holla2040/arturo/internal/store"
)

// GeneratePDF creates a customer-facing PDF report for the given RMA.
func GeneratePDF(w io.Writer, st *store.Store, rmaID string) error {
	artifact, err := Generate(st, rmaID)
	if err != nil {
		return fmt.Errorf("generate artifact: %w", err)
	}
	if artifact == nil {
		return fmt.Errorf("RMA not found")
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 15)

	// --- Page 1: RMA Header ---
	pdf.AddPage()

	// Title
	pdf.SetFont("Arial", "B", 18)
	pdf.CellFormat(0, 12, "Test Report", "", 1, "C", false, 0, "")
	pdf.Ln(4)

	// RMA Info
	pdf.SetFont("Arial", "", 10)
	info := []struct{ label, value string }{
		{"RMA Number", artifact.RMANumber},
		{"Pump Serial Number", artifact.PumpSerialNumber},
		{"Customer", artifact.CustomerName},
		{"Pump Model", artifact.PumpModel},
		{"Employee", fmt.Sprintf("%s (%s)", artifact.Employee.Name, artifact.Employee.ID)},
		{"Status", artifact.Status},
		{"Created", artifact.CreatedAt.Format(time.RFC3339)},
	}
	if artifact.ClosedAt != nil {
		info = append(info, struct{ label, value string }{"Closed", artifact.ClosedAt.Format(time.RFC3339)})
	}

	for _, item := range info {
		pdf.SetFont("Arial", "B", 10)
		pdf.CellFormat(45, 7, item.label+":", "", 0, "L", false, 0, "")
		pdf.SetFont("Arial", "", 10)
		pdf.CellFormat(0, 7, item.value, "", 1, "L", false, 0, "")
	}

	if artifact.Notes != "" {
		pdf.Ln(2)
		pdf.SetFont("Arial", "B", 10)
		pdf.CellFormat(45, 7, "Notes:", "", 0, "L", false, 0, "")
		pdf.SetFont("Arial", "", 10)
		pdf.MultiCell(0, 7, artifact.Notes, "", "L", false)
	}

	pdf.Ln(6)

	// --- Test Run Summary Table ---
	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 8, "Test Run History", "", 1, "L", false, 0, "")
	pdf.Ln(2)

	if len(artifact.Runs) == 0 {
		pdf.SetFont("Arial", "I", 10)
		pdf.CellFormat(0, 7, "No test runs recorded.", "", 1, "L", false, 0, "")
	} else {
		// Table header
		pdf.SetFont("Arial", "B", 9)
		pdf.SetFillColor(220, 220, 220)
		pdf.CellFormat(35, 7, "Script", "1", 0, "L", true, 0, "")
		pdf.CellFormat(35, 7, "Started", "1", 0, "L", true, 0, "")
		pdf.CellFormat(25, 7, "Status", "1", 0, "C", true, 0, "")
		pdf.CellFormat(0, 7, "Summary", "1", 1, "L", true, 0, "")

		// Table rows
		pdf.SetFont("Arial", "", 9)
		for _, run := range artifact.Runs {
			pdf.CellFormat(35, 7, truncate(run.ScriptName, 20), "1", 0, "L", false, 0, "")
			pdf.CellFormat(35, 7, run.StartedAt.Format("2006-01-02 15:04"), "1", 0, "L", false, 0, "")
			pdf.CellFormat(25, 7, run.Status, "1", 0, "C", false, 0, "")
			pdf.CellFormat(0, 7, truncate(run.Summary, 40), "1", 1, "L", false, 0, "")
		}
	}

	// --- Per-run details ---
	for i, run := range artifact.Runs {
		pdf.AddPage()
		pdf.SetFont("Arial", "B", 14)
		pdf.CellFormat(0, 10, fmt.Sprintf("Run %d: %s", i+1, run.ScriptName), "", 1, "L", false, 0, "")

		pdf.SetFont("Arial", "", 10)
		pdf.CellFormat(0, 7, fmt.Sprintf("Status: %s    Started: %s", run.Status, run.StartedAt.Format(time.RFC3339)), "", 1, "L", false, 0, "")
		if run.FinishedAt != nil {
			pdf.CellFormat(0, 7, fmt.Sprintf("Finished: %s", run.FinishedAt.Format(time.RFC3339)), "", 1, "L", false, 0, "")
		}
		if run.Summary != "" {
			pdf.CellFormat(0, 7, fmt.Sprintf("Summary: %s", run.Summary), "", 1, "L", false, 0, "")
		}
		pdf.Ln(4)

		// Measurements
		if len(run.Measurements) > 0 {
			pdf.SetFont("Arial", "B", 11)
			pdf.CellFormat(0, 7, "Measurements", "", 1, "L", false, 0, "")

			pdf.SetFont("Arial", "B", 8)
			pdf.SetFillColor(220, 220, 220)
			pdf.CellFormat(30, 6, "Device", "1", 0, "L", true, 0, "")
			pdf.CellFormat(40, 6, "Command", "1", 0, "L", true, 0, "")
			pdf.CellFormat(15, 6, "OK", "1", 0, "C", true, 0, "")
			pdf.CellFormat(50, 6, "Response", "1", 0, "L", true, 0, "")
			pdf.CellFormat(20, 6, "Duration", "1", 0, "R", true, 0, "")
			pdf.CellFormat(0, 6, "Time", "1", 1, "L", true, 0, "")

			pdf.SetFont("Arial", "", 8)
			for _, m := range run.Measurements {
				ok := "Y"
				if !m.Success {
					ok = "N"
				}
				pdf.CellFormat(30, 6, truncate(m.DeviceID, 15), "1", 0, "L", false, 0, "")
				pdf.CellFormat(40, 6, truncate(m.CommandName, 22), "1", 0, "L", false, 0, "")
				pdf.CellFormat(15, 6, ok, "1", 0, "C", false, 0, "")
				pdf.CellFormat(50, 6, truncate(m.Response, 30), "1", 0, "L", false, 0, "")
				pdf.CellFormat(20, 6, fmt.Sprintf("%dms", m.DurationMs), "1", 0, "R", false, 0, "")
				pdf.CellFormat(0, 6, m.Timestamp.Format("15:04:05"), "1", 1, "L", false, 0, "")
			}
			pdf.Ln(4)
		}

		// Events
		if len(run.Events) > 0 {
			pdf.SetFont("Arial", "B", 11)
			pdf.CellFormat(0, 7, "Events", "", 1, "L", false, 0, "")

			pdf.SetFont("Arial", "B", 8)
			pdf.SetFillColor(220, 220, 220)
			pdf.CellFormat(30, 6, "Type", "1", 0, "L", true, 0, "")
			pdf.CellFormat(30, 6, "Employee", "1", 0, "L", true, 0, "")
			pdf.CellFormat(60, 6, "Reason", "1", 0, "L", true, 0, "")
			pdf.CellFormat(0, 6, "Time", "1", 1, "L", true, 0, "")

			pdf.SetFont("Arial", "", 8)
			for _, e := range run.Events {
				pdf.CellFormat(30, 6, e.Type, "1", 0, "L", false, 0, "")
				pdf.CellFormat(30, 6, truncate(e.EmployeeID, 15), "1", 0, "L", false, 0, "")
				pdf.CellFormat(60, 6, truncate(e.Reason, 35), "1", 0, "L", false, 0, "")
				pdf.CellFormat(0, 6, e.Timestamp.Format("15:04:05"), "1", 1, "L", false, 0, "")
			}
		}
	}

	return pdf.Output(w)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
