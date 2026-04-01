package artifact

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/holla2040/arturo/internal/store"
)

// regenStateName maps a CTI O-command response letter to a human-readable
// regen state description. Mirrors the mapping in onboard_regen.art.
func regenStateName(letter string) string {
	switch letter {
	case "A", "\\":
		return "Pump OFF"
	case "B", "C", "E", "^", "]":
		return "Warmup"
	case "D", "F", "G", "Q", "R":
		return "Purge gas failure"
	case "H":
		return "Extended purge"
	case "S":
		return "Repurge cycle"
	case "I", "J", "K", "T", "a", "b", "j", "n":
		return "Rough to base pressure"
	case "L":
		return "Rate of rise test"
	case "M", "N", "c", "d", "o":
		return "Cooldown"
	case "P":
		return "Regen complete"
	case "U":
		return "Beginning of fast regen"
	case "V":
		return "Regen aborted"
	case "W":
		return "Delay restart"
	case "X", "Y":
		return "Power failure"
	case "Z":
		return "Delay start"
	case "O", "[":
		return "Zeroing TC gauge"
	case "f":
		return "Share regen wait"
	case "e":
		return "Repurge during fast regen"
	case "h":
		return "Purge coordinate wait"
	case "i":
		return "Rough coordinate wait"
	case "k":
		return "Purge gas fail, recovering"
	default:
		return "Unknown (" + letter + ")"
	}
}

// regenSample holds one grouped sample row for the regen CSV table.
type regenSample struct {
	timestamp  time.Time
	first      string
	second     string
	regenState string
}

// buildRegenSamples groups query events into sample rows by parsing the
// reason field (e.g. "get_temp_1st_stage -> 15.0") and collecting one of
// each command name per row.
func buildRegenSamples(events []ArtifactEvent) []regenSample {
	var samples []regenSample
	var cur regenSample
	filled := map[string]bool{}

	for _, e := range events {
		if e.Type != "query" {
			continue
		}

		parts := strings.SplitN(e.Reason, " -> ", 2)
		if len(parts) != 2 {
			continue
		}
		cmd, val := parts[0], parts[1]

		switch cmd {
		case "get_regen_status", "get_temp_1st_stage", "get_temp_2nd_stage":
		default:
			continue
		}

		if filled[cmd] {
			if len(filled) > 0 {
				samples = append(samples, cur)
			}
			cur = regenSample{}
			filled = map[string]bool{}
		}

		if cur.timestamp.IsZero() {
			cur.timestamp = e.Timestamp
		}
		filled[cmd] = true

		switch cmd {
		case "get_temp_1st_stage":
			cur.first = val
		case "get_temp_2nd_stage":
			cur.second = val
		case "get_regen_status":
			cur.regenState = regenStateName(val)
		}
	}

	if len(filled) > 0 {
		samples = append(samples, cur)
	}
	return samples
}

func renderRegenCSV(pdf *fpdf.Fpdf, run ArtifactRun) {
	samples := buildRegenSamples(run.Events)
	if len(samples) == 0 {
		pdf.SetFont("Arial", "I", 10)
		pdf.CellFormat(0, 7, "No temperature data recorded.", "", 1, "L", false, 0, "")
		return
	}

	pdf.SetFont("Courier", "B", 7)
	pdf.CellFormat(0, 4, "Timestamp (MST), 1st Stage (K), 2nd Stage (K), Regen State", "", 1, "L", false, 0, "")

	pdf.SetFont("Courier", "", 7)
	for _, s := range samples {
		line := fmt.Sprintf("%s, %s, %s, %s",
			s.timestamp.In(denverTZ).Format("2006-01-02 15:04:05"),
			s.first, s.second, s.regenState)
		pdf.CellFormat(0, 3.5, line, "", 1, "L", false, 0, "")
	}
}

// denverTZ is used for operator-facing timestamps in customer PDFs.
var denverTZ *time.Location

func init() {
	var err error
	denverTZ, err = time.LoadLocation("America/Denver")
	if err != nil {
		denverTZ = time.UTC
	}
}

// GenerateFilteredPDF creates a PDF report containing only the specified runs.
func GenerateFilteredPDF(w io.Writer, st *store.Store, rmaID string, runIDs []string) error {
	artifact, err := GenerateFiltered(st, rmaID, runIDs)
	if err != nil {
		return fmt.Errorf("generate artifact: %w", err)
	}
	return renderPDF(w, artifact)
}

// GeneratePDF creates a customer-facing PDF report for the given RMA.
func GeneratePDF(w io.Writer, st *store.Store, rmaID string) error {
	artifact, err := Generate(st, rmaID)
	if err != nil {
		return fmt.Errorf("generate artifact: %w", err)
	}
	return renderPDF(w, artifact)
}

func renderPDF(w io.Writer, artifact *TestArtifact) error {
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
		{"Created", artifact.CreatedAt.In(denverTZ).Format("2006-01-02 15:04:05 MST")},
	}
	if artifact.ClosedAt != nil {
		info = append(info, struct{ label, value string }{"Closed", artifact.ClosedAt.In(denverTZ).Format("2006-01-02 15:04:05 MST")})
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
			pdf.CellFormat(35, 7, run.StartedAt.In(denverTZ).Format("2006-01-02 15:04"), "1", 0, "L", false, 0, "")
			pdf.CellFormat(25, 7, run.Status, "1", 0, "C", false, 0, "")
			pdf.CellFormat(0, 7, truncate(run.Summary, 40), "1", 1, "L", false, 0, "")
		}
	}

	// --- Per-run details ---
	for i, run := range artifact.Runs {
		pdf.AddPage()
		pdf.SetFont("Arial", "B", 14)
		runTitle := fmt.Sprintf("Run %d: %s", i+1, run.ScriptName)
		if run.ReportVersion != "" {
			runTitle += fmt.Sprintf("  (v%s)", run.ReportVersion)
		}
		pdf.CellFormat(0, 10, runTitle, "", 1, "L", false, 0, "")

		pdf.SetFont("Arial", "", 10)
		pdf.CellFormat(0, 7, fmt.Sprintf("Status: %s    Started: %s", run.Status, run.StartedAt.In(denverTZ).Format("2006-01-02 15:04:05 MST")), "", 1, "L", false, 0, "")
		if run.FinishedAt != nil {
			pdf.CellFormat(0, 7, fmt.Sprintf("Finished: %s", run.FinishedAt.In(denverTZ).Format("2006-01-02 15:04:05 MST")), "", 1, "L", false, 0, "")
		}
		if run.Summary != "" {
			pdf.CellFormat(0, 7, fmt.Sprintf("Summary: %s", run.Summary), "", 1, "L", false, 0, "")
		}
		pdf.Ln(4)

		if run.ReportType == "regen" {
			renderRegenCSV(pdf, run)
		} else {
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
					pdf.CellFormat(0, 6, m.Timestamp.In(denverTZ).Format("15:04:05"), "1", 1, "L", false, 0, "")
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
				pdf.CellFormat(60, 6, "Description", "1", 0, "L", true, 0, "")
				pdf.CellFormat(0, 6, "Time", "1", 1, "L", true, 0, "")

				pdf.SetFont("Arial", "", 8)
				for _, e := range run.Events {
					pdf.CellFormat(30, 6, e.Type, "1", 0, "L", false, 0, "")
					pdf.CellFormat(30, 6, truncate(e.EmployeeID, 15), "1", 0, "L", false, 0, "")
					pdf.CellFormat(60, 6, truncate(e.Reason, 35), "1", 0, "L", false, 0, "")
					pdf.CellFormat(0, 6, e.Timestamp.In(denverTZ).Format("15:04:05"), "1", 1, "L", false, 0, "")
				}
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
