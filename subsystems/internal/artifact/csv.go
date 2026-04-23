package artifact

import (
	"encoding/csv"
	"fmt"
	"io"
	"time"

	"github.com/holla2040/arturo/internal/store"
)

// ExportRunCSV writes a CSV for a single test run within an RMA. The output
// includes RMA metadata header lines, a blank separator, then the same
// 8-column regen-curve data table ExportStationRegenCurve produces — sourced
// from the station poller's temperature_log and pump_status_log over the
// run's [StartedAt, FinishedAt] window. Script events are NOT the data
// source: telemetry is owned by the poller per ARCHITECTURE.md §2.8.
func ExportRunCSV(w io.Writer, st *store.Store, rmaID, runID string) error {
	rma, err := st.GetRMA(rmaID)
	if err != nil {
		return fmt.Errorf("get RMA: %w", err)
	}
	if rma == nil {
		return fmt.Errorf("RMA not found")
	}

	run, err := st.GetTestRun(runID)
	if err != nil {
		return fmt.Errorf("get test run: %w", err)
	}
	if run == nil {
		return fmt.Errorf("test run not found")
	}

	stationInstance := ""
	if run.StationInstance != nil {
		stationInstance = *run.StationInstance
	}

	since := run.StartedAt
	until := time.Now()
	if run.FinishedAt != nil {
		until = *run.FinishedAt
	}

	var temps []store.TemperatureLogEntry
	var pumps []store.PumpStatusLogEntry
	if stationInstance != "" {
		temps, err = st.QueryTemperatureLogRange(stationInstance, since, until)
		if err != nil {
			return fmt.Errorf("query temperatures: %w", err)
		}
		pumps, err = st.QueryPumpStatusLog(stationInstance, since, until)
		if err != nil {
			return fmt.Errorf("query pump status: %w", err)
		}
	}

	cw := csv.NewWriter(w)

	// RMA metadata header
	cw.Write([]string{"RMA Number", rma.RMANumber})
	cw.Write([]string{"Pump Serial Number", rma.PumpSerialNumber})
	cw.Write([]string{"Customer", rma.CustomerName})
	cw.Write([]string{"Pump Model", rma.PumpModel})
	cw.Write([]string{"Station ID", stationInstance})
	cw.Write([]string{"Test ID", run.ID})
	cw.Write([]string{"Test", run.ScriptName})
	cw.Write([]string{"Status", run.Status})
	cw.Write([]string{"Started", run.StartedAt.In(denverTZ).Format("2006-01-02 15:04:05 MST")})
	if run.FinishedAt != nil {
		cw.Write([]string{"Finished", run.FinishedAt.In(denverTZ).Format("2006-01-02 15:04:05 MST")})
	}

	// Blank separator line
	cw.Write([]string{})

	// Data header — matches ExportStationRegenCurve
	cw.Write([]string{
		"Timestamp (MST/MDT)",
		"1st Stage (K)",
		"2nd Stage (K)",
		"Regen Status",
		"Regen State",
		"Pump",
		"Rough",
		"Purge",
	})

	writeRegenCurveRows(cw, temps, pumps)

	cw.Flush()
	return cw.Error()
}
