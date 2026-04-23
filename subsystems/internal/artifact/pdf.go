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
	"gonum.org/v1/plot/vg/draw"
	"gonum.org/v1/plot/vg/vgimg"
)

// plotDPI raises the raster resolution of the temperature plot well above
// gonum's 96 DPI default so text stays crisp when zoomed in the PDF.
const plotDPI = 300

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

// buildTempSeries extracts 1st- and 2nd-stage temperature points for the
// plot. Prefers the canonical temperature_samples feed (all samples from
// the poller). Falls back to parsing events for older runs that predate
// the samples table or when samples are absent.
func buildTempSeries(run ArtifactRun) (firstPts, secondPts plotter.XYs) {
	if len(run.Temperatures) > 0 {
		for _, t := range run.Temperatures {
			x := float64(t.Timestamp.Unix())
			switch t.Stage {
			case "first_stage":
				firstPts = append(firstPts, plotter.XY{X: x, Y: t.TemperatureK})
			case "second_stage":
				secondPts = append(secondPts, plotter.XY{X: x, Y: t.TemperatureK})
			}
		}
		return
	}
	for _, e := range run.Events {
		x := float64(e.Timestamp.Unix())
		switch e.Type {
		case "query":
			parts := strings.SplitN(e.Reason, " -> ", 2)
			if len(parts) != 2 {
				continue
			}
			v, err := strconv.ParseFloat(parts[1], 64)
			if err != nil {
				continue
			}
			switch parts[0] {
			case "get_temp_1st_stage":
				firstPts = append(firstPts, plotter.XY{X: x, Y: v})
			case "get_temp_2nd_stage":
				secondPts = append(secondPts, plotter.XY{X: x, Y: v})
			}
		case "regen_state":
			if v, ok := parseTempField(e.Reason, "1st="); ok {
				firstPts = append(firstPts, plotter.XY{X: x, Y: v})
			}
			if v, ok := parseTempField(e.Reason, "2nd="); ok {
				secondPts = append(secondPts, plotter.XY{X: x, Y: v})
			}
		}
	}
	return
}

// parseTempField extracts the float after `prefix` up to the next "K" in s.
func parseTempField(s, prefix string) (float64, bool) {
	i := strings.Index(s, prefix)
	if i < 0 {
		return 0, false
	}
	rest := s[i+len(prefix):]
	end := strings.IndexByte(rest, 'K')
	if end < 0 {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(rest[:end]), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// renderAcceptanceDataCSV emits every temperature sample recorded for the
// run as plain comma-separated text so the reader can copy-paste it into
// a spreadsheet. Samples are paired by timestamp (first + second stage
// within ±1s share a row); unpaired samples keep their own row with the
// missing column blank. No borders, just text.
func renderAcceptanceDataCSV(pdf *fpdf.Fpdf, run ArtifactRun) {
	var firsts, seconds []ArtifactTemp
	for _, t := range run.Temperatures {
		switch t.Stage {
		case "first_stage":
			firsts = append(firsts, t)
		case "second_stage":
			seconds = append(seconds, t)
		}
	}

	pdf.SetFont("Courier", "B", 6)
	pdf.CellFormat(0, 3, "Timestamp (MST), 1st Stage (K), 2nd Stage (K)", "", 1, "L", false, 0, "")
	pdf.SetFont("Courier", "", 6)

	if len(firsts) == 0 && len(seconds) == 0 {
		pdf.SetFont("Arial", "I", 10)
		pdf.CellFormat(0, 7, "No temperature samples recorded.", "", 1, "L", false, 0, "")
		return
	}

	emit := func(ts time.Time, first, second string) {
		line := fmt.Sprintf("%s, %s, %s",
			ts.In(denverTZ).Format("2006-01-02 15:04:05"), first, second)
		pdf.CellFormat(0, 2.6, line, "", 1, "L", false, 0, "")
	}

	i, j := 0, 0
	for i < len(firsts) && j < len(seconds) {
		a, b := firsts[i], seconds[j]
		delta := b.Timestamp.Sub(a.Timestamp)
		withinWindow := delta >= -time.Second && delta <= time.Second
		switch {
		case withinWindow:
			emit(a.Timestamp, fmt.Sprintf("%.2f", a.TemperatureK), fmt.Sprintf("%.2f", b.TemperatureK))
			i++
			j++
		case delta < 0:
			emit(b.Timestamp, "", fmt.Sprintf("%.2f", b.TemperatureK))
			j++
		default:
			emit(a.Timestamp, fmt.Sprintf("%.2f", a.TemperatureK), "")
			i++
		}
	}
	for ; i < len(firsts); i++ {
		emit(firsts[i].Timestamp, fmt.Sprintf("%.2f", firsts[i].TemperatureK), "")
	}
	for ; j < len(seconds); j++ {
		emit(seconds[j].Timestamp, "", fmt.Sprintf("%.2f", seconds[j].TemperatureK))
	}
}

// renderRegenPlotRotated generates a temperature-vs-time line chart from
// regen samples and places it on the page rotated 90° clockwise, filling
// all space below the current Y position down to the bottom margin.
func renderRegenPlotRotated(pdf *fpdf.Fpdf, run ArtifactRun, runIndex int) {
	firstPts, secondPts := buildTempSeries(run)

	if len(firstPts)+len(secondPts) < 2 {
		return
	}

	pageW, pageH := pdf.GetPageSize()
	ml, _, mr, mb := pdf.GetMargins()
	x1 := ml
	y1 := pdf.GetY()
	targetW := pageW - ml - mr
	targetH := pageH - y1 - mb

	p := plot.New()
	p.Title.Text = "Temperature vs Time"
	p.X.Label.Text = "Time (MST)"
	p.Y.Label.Text = "Temperature (K)"

	p.Y.Min = 0
	p.Y.Max = 320
	p.Y.Tick.Marker = fixedStepTicks{min: 0, max: 320, step: 40}
	p.X.Tick.Marker = mstTimeTicks{}

	p.Add(plotter.NewGrid())

	gridColor := color.RGBA{R: 200, G: 200, B: 200, A: 255}
	p.X.Color = gridColor
	p.Y.Color = gridColor

	// Establish X-range covering both series for the 20K reference line.
	var xMin, xMax float64
	if len(firstPts) > 0 {
		xMin = firstPts[0].X
		xMax = firstPts[len(firstPts)-1].X
	} else {
		xMin = secondPts[0].X
		xMax = secondPts[len(secondPts)-1].X
	}
	if len(secondPts) > 0 {
		if secondPts[0].X < xMin {
			xMin = secondPts[0].X
		}
		if secondPts[len(secondPts)-1].X > xMax {
			xMax = secondPts[len(secondPts)-1].X
		}
	}
	refPts := plotter.XYs{{X: xMin, Y: 20}, {X: xMax, Y: 20}}
	if refLine, err := plotter.NewLine(refPts); err == nil {
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
			line.Width = vg.Points(1.5)
			p.Add(line)
			p.Legend.Add("1st Stage", line)
		}
	}

	if len(secondPts) >= 2 {
		line, err := plotter.NewLine(secondPts)
		if err == nil {
			line.Color = color.RGBA{R: 74, G: 158, B: 255, A: 255}
			line.Width = vg.Points(1.5)
			p.Add(line)
			p.Legend.Add("2nd Stage", line)
		}
	}

	p.Legend.Top = true

	// Render at pre-rotation dimensions: width=targetH, height=targetW so
	// that after 90° CW rotation the image fills (targetW × targetH) mm.
	// Use an explicit high-DPI canvas so the embedded PNG keeps its detail
	// under zoom; displayed size in the PDF is unchanged.
	canvas := vgimg.NewWith(
		vgimg.UseWH(vg.Length(targetH)*vg.Millimeter, vg.Length(targetW)*vg.Millimeter),
		vgimg.UseDPI(plotDPI),
	)
	p.Draw(draw.New(canvas))
	var buf bytes.Buffer
	if _, err := (vgimg.PngCanvas{Canvas: canvas}).WriteTo(&buf); err != nil {
		return
	}

	imgName := fmt.Sprintf("regen_plot_rot_%d", runIndex)
	pdf.RegisterImageOptionsReader(imgName, fpdf.ImageOptions{ImageType: "PNG"}, &buf)

	// Rotate -90° (CW visually) around target top-right, then draw image
	// anchored at that same point with pre-rotation size (targetH × targetW);
	// the rotation remaps it to fill (x1, y1) .. (x1+targetW, y1+targetH).
	pdf.TransformBegin()
	pdf.TransformRotate(-90, x1+targetW, y1)
	pdf.ImageOptions(imgName, x1+targetW, y1, targetH, targetW, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
	pdf.TransformEnd()
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

	start := float64(int64(min/interval) * int64(interval))
	var ticks []plot.Tick
	for v := start; v <= max; v += interval {
		if v >= min {
			label := time.Unix(int64(v), 0).In(denverTZ).Format("15:04")
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
		pdf.CellFormat(45, 7, "Test", "1", 0, "L", true, 0, "")
		pdf.CellFormat(40, 7, "Test ID", "1", 0, "L", true, 0, "")
		pdf.CellFormat(30, 7, "Started", "1", 0, "L", true, 0, "")
		pdf.CellFormat(20, 7, "Status", "1", 0, "C", true, 0, "")
		pdf.CellFormat(0, 7, "Technician", "1", 1, "L", true, 0, "")

		// Table rows
		pdf.SetFont("Arial", "", 9)
		for _, run := range artifact.Runs {
			pdf.CellFormat(45, 7, truncate(run.ScriptName, 24), "1", 0, "L", false, 0, "")
			pdf.CellFormat(40, 7, truncate(run.RunID, 22), "1", 0, "L", false, 0, "")
			pdf.CellFormat(30, 7, run.StartedAt.In(denverTZ).Format("2006-01-02 15:04"), "1", 0, "L", false, 0, "")
			pdf.CellFormat(20, 7, run.Status, "1", 0, "C", false, 0, "")
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
		pdf.CellFormat(0, 7, runTitle, "", 1, "L", false, 0, "")

		pdf.SetFont("Arial", "", 10)
		runInfo := []struct{ label, value string }{
			{"Test ID:", run.RunID},
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
			pdf.CellFormat(25, 4.5, item.label, "", 0, "L", false, 0, "")
			pdf.SetFont("Arial", "", 10)
			pdf.CellFormat(0, 4.5, item.value, "", 1, "L", false, 0, "")
		}

		if run.ReportType == "regen" {
			renderRegenPlotRotated(pdf, run, i)
			pdf.AddPage()
			renderRegenCSV(pdf, run)
		} else if run.ReportType == "acceptance" {
			renderRegenPlotRotated(pdf, run, i)
			pdf.AddPage()

			if len(run.Events) > 0 {
				pdf.SetFont("Arial", "B", 11)
				pdf.CellFormat(0, 7, "Event Log", "", 1, "L", false, 0, "")

				pdf.SetFont("Arial", "B", 8)
				pdf.SetFillColor(220, 220, 220)
				pdf.CellFormat(25, 6, "Time", "1", 0, "L", true, 0, "")
				pdf.CellFormat(30, 6, "Type", "1", 0, "L", true, 0, "")
				pdf.CellFormat(0, 6, "Description", "1", 1, "L", true, 0, "")

				pdf.SetFont("Arial", "", 8)
				for _, e := range run.Events {
					lines := pdf.SplitLines([]byte(e.Reason), 135)
					if len(lines) == 0 {
						lines = [][]byte{nil}
					}
					rowH := 6.0 * float64(len(lines))
					pdf.CellFormat(25, rowH, e.Timestamp.In(denverTZ).Format("15:04:05"), "1", 0, "L", false, 0, "")
					pdf.CellFormat(30, rowH, e.Type, "1", 0, "L", false, 0, "")
					pdf.MultiCell(135, 6, e.Reason, "1", "L", false)
				}
			}

			pdf.AddPage()
			renderAcceptanceDataCSV(pdf, run)
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
				pdf.CellFormat(25, 6, "Time", "1", 0, "L", true, 0, "")
				pdf.CellFormat(30, 6, "Type", "1", 0, "L", true, 0, "")
				pdf.CellFormat(0, 6, "Description", "1", 1, "L", true, 0, "")

				pdf.SetFont("Arial", "", 8)
				for _, e := range run.Events {
					lines := pdf.SplitLines([]byte(e.Reason), 135)
					if len(lines) == 0 {
						lines = [][]byte{nil}
					}
					rowH := 6.0 * float64(len(lines))
					pdf.CellFormat(25, rowH, e.Timestamp.In(denverTZ).Format("15:04:05"), "1", 0, "L", false, 0, "")
					pdf.CellFormat(30, rowH, e.Type, "1", 0, "L", false, 0, "")
					pdf.MultiCell(135, 6, e.Reason, "1", "L", false)
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
