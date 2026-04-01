package artifact

import (
	"encoding/csv"
	"fmt"
	"io"

	"github.com/holla2040/arturo/internal/store"
)

// ExportRunCSV writes a CSV for a single test run within an RMA.
// The output includes RMA metadata header lines, a blank separator,
// then time-series data rows.
func ExportRunCSV(w io.Writer, st *store.Store, rmaID, runID string) error {
	art, err := GenerateFiltered(st, rmaID, []string{runID})
	if err != nil {
		return fmt.Errorf("generate artifact: %w", err)
	}
	if art == nil {
		return fmt.Errorf("RMA not found")
	}
	if len(art.Runs) == 0 {
		return fmt.Errorf("test run not found")
	}

	run := art.Runs[0]
	cw := csv.NewWriter(w)

	// RMA metadata header
	cw.Write([]string{"RMA Number", art.RMANumber})
	cw.Write([]string{"Pump Serial Number", art.PumpSerialNumber})
	cw.Write([]string{"Customer", art.CustomerName})
	cw.Write([]string{"Pump Model", art.PumpModel})
	cw.Write([]string{"Test", run.ScriptName})
	cw.Write([]string{"Status", run.Status})
	cw.Write([]string{"Started", run.StartedAt.In(denverTZ).Format("2006-01-02 15:04:05 MST")})
	if run.FinishedAt != nil {
		cw.Write([]string{"Finished", run.FinishedAt.In(denverTZ).Format("2006-01-02 15:04:05 MST")})
	}

	// Blank separator line
	cw.Write([]string{})

	// Data rows
	samples := buildRegenSamples(run.Events)
	cw.Write([]string{"Timestamp (MST)", "1st Stage (K)", "2nd Stage (K)", "Regen Letter", "Regen State"})
	for _, s := range samples {
		cw.Write([]string{
			s.timestamp.In(denverTZ).Format("2006-01-02 15:04:05"),
			s.first,
			s.second,
			s.regenLetter,
			s.regenState,
		})
	}

	cw.Flush()
	return cw.Error()
}
