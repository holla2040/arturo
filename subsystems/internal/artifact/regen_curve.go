package artifact

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"github.com/holla2040/arturo/internal/regen"
	"github.com/holla2040/arturo/internal/store"
	"github.com/holla2040/arturo/internal/testmanager"
)

// ExportStationRegenCurve writes a station-scoped regen curve CSV for the
// time window [since, until]. The file has a metadata header, a blank
// separator row, then a data table joining temperature_log and
// pump_status_log. Missing per-row values are forward-filled with the last
// known reading for that column.
func ExportStationRegenCurve(
	w io.Writer,
	st *store.Store,
	stationID string,
	sess *testmanager.SessionInfo,
	since, until time.Time,
) error {
	temps, err := st.QueryTemperatureLogRange(stationID, since, until)
	if err != nil {
		return fmt.Errorf("query temperatures: %w", err)
	}
	pumps, err := st.QueryPumpStatusLog(stationID, since, until)
	if err != nil {
		return fmt.Errorf("query pump status: %w", err)
	}

	// Metadata lookups — blank when unavailable.
	var rmaNumber, pumpSerial, script string
	if sess != nil {
		rmaNumber = sess.RMANumber
		script = sess.ScriptPath
		if sess.RMAID != "" {
			if rma, err := st.GetRMA(sess.RMAID); err == nil && rma != nil {
				pumpSerial = rma.PumpSerialNumber
				if rmaNumber == "" {
					rmaNumber = rma.RMANumber
				}
			}
		}
	}

	cw := csv.NewWriter(w)

	// Metadata header
	cw.Write([]string{"Station ID", stationID})
	cw.Write([]string{"Pump Serial Number", pumpSerial})
	cw.Write([]string{"RMA", rmaNumber})
	cw.Write([]string{"Script", script})
	cw.Write([]string{"Exported", time.Now().In(denverTZ).Format("2006-01-02 15:04:05 MST")})

	// Blank separator
	cw.Write([]string{})

	// Data header
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

// writeRegenCurveRows buckets temperature_log + pump_status_log entries by
// unix second, forward-fills missing per-column values from the last known
// reading, and writes one CSV row per bucket with the 8 columns defined by
// the data header: Timestamp, 1st Stage (K), 2nd Stage (K), Regen Status,
// Regen State, Pump, Rough, Purge.
func writeRegenCurveRows(cw *csv.Writer, temps []store.TemperatureLogEntry, pumps []store.PumpStatusLogEntry) {
	type bucket struct {
		first     *float64
		second    *float64
		pumpOn    *bool
		rough     *bool
		purge     *bool
		regenChar string
	}
	buckets := map[int64]*bucket{}
	get := func(ts time.Time) *bucket {
		key := ts.Unix()
		b, ok := buckets[key]
		if !ok {
			b = &bucket{}
			buckets[key] = b
		}
		return b
	}

	for _, t := range temps {
		b := get(t.Timestamp)
		v := t.TemperatureK
		switch t.Stage {
		case "first_stage":
			b.first = &v
		case "second_stage":
			b.second = &v
		}
	}
	for _, p := range pumps {
		b := get(p.Timestamp)
		on, r, pg := p.PumpOn, p.RoughValveOpen, p.PurgeValveOpen
		b.pumpOn = &on
		b.rough = &r
		b.purge = &pg
		if p.RegenStatus != "" {
			b.regenChar = p.RegenStatus
		}
	}

	keys := make([]int64, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	var rollFirst, rollSecond *float64
	var rollPump, rollRough, rollPurge *bool
	var rollRegen string

	for _, k := range keys {
		b := buckets[k]
		if b.first != nil {
			rollFirst = b.first
		}
		if b.second != nil {
			rollSecond = b.second
		}
		if b.pumpOn != nil {
			rollPump = b.pumpOn
		}
		if b.rough != nil {
			rollRough = b.rough
		}
		if b.purge != nil {
			rollPurge = b.purge
		}
		if b.regenChar != "" {
			rollRegen = b.regenChar
		}

		ts := time.Unix(k, 0).In(denverTZ).Format("2006-01-02 15:04:05")
		regenState := ""
		if rollRegen != "" {
			regenState = regen.StateName(rollRegen)
		}
		cw.Write([]string{
			ts,
			formatTemp(rollFirst),
			formatTemp(rollSecond),
			rollRegen,
			regenState,
			formatBool(rollPump),
			formatBool(rollRough),
			formatBool(rollPurge),
		})
	}
}

func formatTemp(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', -1, 64)
}

func formatBool(v *bool) string {
	if v == nil {
		return ""
	}
	if *v {
		return "1"
	}
	return "0"
}
