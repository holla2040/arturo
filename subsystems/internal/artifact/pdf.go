package artifact

import (
	"bytes"
	"fmt"
	"image/color"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/holla2040/arturo/internal/regen"
	"github.com/holla2040/arturo/internal/store"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
)

// regenSample holds one grouped sample row for the regen CSV table.
type regenSample struct {
	timestamp   time.Time
	first       string
	second      string
	regenLetter string
	regenState  string
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
			cur.regenLetter = val
			cur.regenState = regen.StateName(val)
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

// renderRegenPlot generates a temperature-vs-time line chart from regen
// samples and embeds it in the PDF. Skips if fewer than 2 plottable points.
func renderRegenPlot(pdf *fpdf.Fpdf, run ArtifactRun, runIndex int) {
	samples := buildRegenSamples(run.Events)

	// Parse temperature strings to floats, build XY series.
	var firstPts, secondPts plotter.XYs
	for _, s := range samples {
		x := float64(s.timestamp.Unix())
		if v, err := strconv.ParseFloat(s.first, 64); err == nil {
			firstPts = append(firstPts, plotter.XY{X: x, Y: v})
		}
		if v, err := strconv.ParseFloat(s.second, 64); err == nil {
			secondPts = append(secondPts, plotter.XY{X: x, Y: v})
		}
	}

	if len(firstPts)+len(secondPts) < 2 {
		return
	}

	p := plot.New()
	p.Title.Text = "Temperature vs Time"
	p.X.Label.Text = "Time (MST)"
	p.Y.Label.Text = "Temperature (K)"

	// Fixed Y-axis range 0-320 with grid lines every 20 degrees.
	p.Y.Min = 0
	p.Y.Max = 320
	p.Y.Tick.Marker = fixedStepTicks{min: 0, max: 320, step: 40}

	// Custom X-axis tick formatter: Unix seconds -> MST time strings.
	p.X.Tick.Marker = mstTimeTicks{}

	// Grid lines on both axes.
	p.Add(plotter.NewGrid())

	// Border around the plot area.
	gridColor := color.RGBA{R: 200, G: 200, B: 200, A: 255}
	p.X.Color = gridColor
	p.Y.Color = gridColor

	// Horizontal dashed reference line at 20K.
	refPts := plotter.XYs{plotter.XY{X: firstPts[0].X, Y: 20}, plotter.XY{X: firstPts[len(firstPts)-1].X, Y: 20}}
	if len(secondPts) > 0 {
		if secondPts[0].X < refPts[0].X {
			refPts[0].X = secondPts[0].X
		}
		if secondPts[len(secondPts)-1].X > refPts[1].X {
			refPts[1].X = secondPts[len(secondPts)-1].X
		}
	}
	refLine, err := plotter.NewLine(refPts)
	if err == nil {
		refLine.Color = color.RGBA{R: 239, G: 68, B: 68, A: 255}
		refLine.Width = vg.Points(1)
		refLine.Dashes = []vg.Length{vg.Points(5), vg.Points(3)}
		p.Add(refLine)
		p.Legend.Add("20 K", refLine)
	}

	if len(firstPts) >= 2 {
		line, err := plotter.NewLine(firstPts)
		if err == nil {
			line.Color = color.RGBA{R: 34, G: 197, B: 94, A: 255}
			line.Width = vg.Points(1)
			p.Add(line)
			p.Legend.Add("1st Stage (K)", line)
		}
	}

	if len(secondPts) >= 2 {
		line, err := plotter.NewLine(secondPts)
		if err == nil {
			line.Color = color.RGBA{R: 74, G: 158, B: 255, A: 255}
			line.Width = vg.Points(1)
			p.Add(line)
			p.Legend.Add("2nd Stage (K)", line)
		}
	}

	p.Legend.Top = true

	// Render to PNG in memory.
	w, err := p.WriterTo(8*vg.Inch, 8*vg.Inch, "png")
	if err != nil {
		return
	}
	var buf bytes.Buffer
	if _, err := w.WriteTo(&buf); err != nil {
		return
	}

	imgName := fmt.Sprintf("regen_plot_%d", runIndex)
	pdf.RegisterImageOptionsReader(imgName, fpdf.ImageOptions{ImageType: "PNG"}, &buf)
	pdf.ImageOptions(imgName, 10, pdf.GetY(), 190, 0, true, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	pdf.Ln(4)
}

// fixedStepTicks generates ticks at a fixed interval between min and max.
type fixedStepTicks struct {
	min, max, step float64
}

func (t fixedStepTicks) Ticks(_, _ float64) []plot.Tick {
	var ticks []plot.Tick
	for v := t.min; v <= t.max; v += t.step {
		ticks = append(ticks, plot.Tick{Value: v, Label: strconv.Itoa(int(v))})
	}
	return ticks
}

// mstTimeTicks formats Unix-second X values as MST time strings.
type mstTimeTicks struct{}

func (mstTimeTicks) Ticks(min, max float64) []plot.Tick {
	span := max - min
	// Choose interval: aim for ~6-10 ticks.
	intervals := []float64{
		10, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200,
	}
	interval := intervals[len(intervals)-1]
	for _, iv := range intervals {
		if span/iv <= 10 {
			interval = iv
			break
		}
	}

	// Use longer format if span > 24 hours.
	format := "15:04"
	if span > 86400 {
		format = "01/02 15:04"
	}

	start := float64(int64(min/interval) * int64(interval))
	var ticks []plot.Tick
	for v := start; v <= max; v += interval {
		if v >= min {
			label := time.Unix(int64(v), 0).In(denverTZ).Format(format)
			ticks = append(ticks, plot.Tick{Value: v, Label: label})
		}
	}
	return ticks
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
	pdf.SetAutoPageBreak(true, 20)
	pdf.AliasNbPages("")

	// Find the date of the last passed test run for the footer.
	reportDate := ""
	for i := len(artifact.Runs) - 1; i >= 0; i-- {
		if artifact.Runs[i].Status == "passed" && artifact.Runs[i].FinishedAt != nil {
			reportDate = artifact.Runs[i].FinishedAt.In(denverTZ).Format("2006-01-02")
			break
		}
	}

	rmaNum := artifact.RMANumber
	pdf.SetFooterFunc(func() {
		pdf.SetY(-15)
		pdf.SetFont("Arial", "", 8)
		pageW, _ := pdf.GetPageSize()
		marginL, _, marginR, _ := pdf.GetMargins()
		usable := pageW - marginL - marginR
		colW := usable / 3
		pdf.CellFormat(colW, 10, rmaNum, "", 0, "L", false, 0, "")
		pdf.CellFormat(colW, 10, reportDate, "", 0, "C", false, 0, "")
		pdf.CellFormat(colW, 10, fmt.Sprintf("Page %d/{nb}", pdf.PageNo()), "", 0, "R", false, 0, "")
	})

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
		pdf.CellFormat(55, 7, "Test", "1", 0, "L", true, 0, "")
		pdf.CellFormat(35, 7, "Started", "1", 0, "L", true, 0, "")
		pdf.CellFormat(25, 7, "Status", "1", 0, "C", true, 0, "")
		pdf.CellFormat(0, 7, "Technician", "1", 1, "L", true, 0, "")

		// Table rows
		pdf.SetFont("Arial", "", 9)
		for _, run := range artifact.Runs {
			pdf.CellFormat(55, 7, truncate(run.ScriptName, 30), "1", 0, "L", false, 0, "")
			pdf.CellFormat(35, 7, run.StartedAt.In(denverTZ).Format("2006-01-02 15:04"), "1", 0, "L", false, 0, "")
			pdf.CellFormat(25, 7, run.Status, "1", 0, "C", false, 0, "")
			pdf.CellFormat(0, 7, run.EmployeeName, "1", 1, "L", false, 0, "")
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
		runInfo := []struct{ label, value string }{
			{"Status:", run.Status},
			{"Started:", run.StartedAt.In(denverTZ).Format("2006-01-02 15:04:05 MST")},
		}
		if run.FinishedAt != nil {
			runInfo = append(runInfo, struct{ label, value string }{"Finished:", run.FinishedAt.In(denverTZ).Format("2006-01-02 15:04:05 MST")})
		}
		if run.EmployeeName != "" {
			runInfo = append(runInfo, struct{ label, value string }{"Technician:", run.EmployeeName})
		}
		for _, item := range runInfo {
			pdf.SetFont("Arial", "B", 10)
			pdf.CellFormat(25, 7, item.label, "", 0, "L", false, 0, "")
			pdf.SetFont("Arial", "", 10)
			pdf.CellFormat(0, 7, item.value, "", 1, "L", false, 0, "")
		}
		pdf.Ln(4)

		if run.ReportType == "regen" {
			renderRegenPlot(pdf, run, i)
			pdf.AddPage()
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
