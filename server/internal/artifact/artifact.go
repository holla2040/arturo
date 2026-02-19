// Package artifact generates JSON and PDF artifacts from RMA test data.
package artifact

import (
	"encoding/json"
	"io"
	"time"

	"github.com/holla2040/arturo/internal/store"
)

// TestArtifact is the top-level JSON document for an RMA's complete test history.
type TestArtifact struct {
	RMANumber        string          `json:"rma_number"`
	PumpSerialNumber string          `json:"pump_serial_number"`
	CustomerName     string          `json:"customer_name"`
	PumpModel        string          `json:"pump_model"`
	Employee         ArtifactEmployee `json:"employee"`
	Status           string          `json:"status"`
	CreatedAt        time.Time       `json:"created_at"`
	ClosedAt         *time.Time      `json:"closed_at,omitempty"`
	Notes            string          `json:"notes,omitempty"`
	Runs             []ArtifactRun   `json:"runs"`
}

// ArtifactEmployee identifies the employee who created the RMA.
type ArtifactEmployee struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ArtifactRun represents a single test run within an RMA.
type ArtifactRun struct {
	RunID        string              `json:"run_id"`
	ScriptName   string              `json:"script_name"`
	ScriptSHA256 string              `json:"script_sha256,omitempty"`
	StartedAt    time.Time           `json:"started_at"`
	FinishedAt   *time.Time          `json:"finished_at,omitempty"`
	Status       string              `json:"status"`
	Summary      string              `json:"summary,omitempty"`
	Events       []ArtifactEvent     `json:"events,omitempty"`
	Temperatures []ArtifactTemp      `json:"temperatures,omitempty"`
	Measurements []ArtifactMeasure   `json:"measurements,omitempty"`
}

// ArtifactEvent is a test lifecycle event.
type ArtifactEvent struct {
	Type       string    `json:"type"`
	EmployeeID string    `json:"employee_id,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// ArtifactTemp is a temperature sample.
type ArtifactTemp struct {
	Stage        string    `json:"stage"`
	TemperatureK float64   `json:"temperature_k"`
	Timestamp    time.Time `json:"timestamp"`
}

// ArtifactMeasure is a device command measurement.
type ArtifactMeasure struct {
	DeviceID    string    `json:"device_id"`
	CommandName string    `json:"command_name"`
	Success     bool      `json:"success"`
	Response    string    `json:"response"`
	DurationMs  int       `json:"duration_ms"`
	Timestamp   time.Time `json:"timestamp"`
}

// Generate builds a TestArtifact from the store for the given RMA ID.
func Generate(st *store.Store, rmaID string) (*TestArtifact, error) {
	rma, err := st.GetRMA(rmaID)
	if err != nil {
		return nil, err
	}
	if rma == nil {
		return nil, nil
	}

	// Get employee
	emp, err := st.GetEmployee(rma.EmployeeID)
	if err != nil {
		return nil, err
	}
	empInfo := ArtifactEmployee{ID: rma.EmployeeID}
	if emp != nil {
		empInfo.Name = emp.Name
	}

	// Get test runs
	runs, err := st.QueryTestRunsByRMA(rmaID)
	if err != nil {
		return nil, err
	}

	artifactRuns := make([]ArtifactRun, 0, len(runs))
	for _, run := range runs {
		ar := ArtifactRun{
			RunID:      run.ID,
			ScriptName: run.ScriptName,
			StartedAt:  run.StartedAt,
			FinishedAt: run.FinishedAt,
			Status:     run.Status,
			Summary:    run.Summary,
		}
		if run.ScriptSHA256 != nil {
			ar.ScriptSHA256 = *run.ScriptSHA256
		}

		// Events
		events, err := st.QueryTestEvents(run.ID)
		if err == nil {
			for _, e := range events {
				ar.Events = append(ar.Events, ArtifactEvent{
					Type:       e.EventType,
					EmployeeID: e.EmployeeID,
					Reason:     e.Reason,
					Timestamp:  e.Timestamp,
				})
			}
		}

		// Temperatures
		temps, err := st.QueryTemperatures(run.ID)
		if err == nil {
			for _, t := range temps {
				ar.Temperatures = append(ar.Temperatures, ArtifactTemp{
					Stage:        t.Stage,
					TemperatureK: t.TemperatureK,
					Timestamp:    t.Timestamp,
				})
			}
		}

		// Measurements
		measurements, err := st.QueryMeasurements(run.ID)
		if err == nil {
			for _, m := range measurements {
				ar.Measurements = append(ar.Measurements, ArtifactMeasure{
					DeviceID:    m.DeviceID,
					CommandName: m.CommandName,
					Success:     m.Success,
					Response:    m.Response,
					DurationMs:  m.DurationMs,
					Timestamp:   m.Timestamp,
				})
			}
		}

		artifactRuns = append(artifactRuns, ar)
	}

	artifact := &TestArtifact{
		RMANumber:        rma.RMANumber,
		PumpSerialNumber: rma.PumpSerialNumber,
		CustomerName:     rma.CustomerName,
		PumpModel:        rma.PumpModel,
		Employee:         empInfo,
		Status:           rma.Status,
		CreatedAt:        rma.CreatedAt,
		ClosedAt:         rma.ClosedAt,
		Notes:            rma.Notes,
		Runs:             artifactRuns,
	}

	return artifact, nil
}

// GenerateJSON writes the JSON artifact to the given writer.
func GenerateJSON(w io.Writer, st *store.Store, rmaID string) error {
	artifact, err := Generate(st, rmaID)
	if err != nil {
		return err
	}
	if artifact == nil {
		return nil
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(artifact)
}
