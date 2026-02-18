package report

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"strconv"
	"time"

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
