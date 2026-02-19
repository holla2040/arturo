package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type TestRun struct {
	ID              string
	ScriptName      string
	StartedAt       time.Time
	FinishedAt      *time.Time
	Status          string // "running", "passed", "failed", "error"
	Summary         string
	RMAID           *string
	StationInstance *string
	ScriptSHA256    *string
	ScriptContent   *string
}

type Measurement struct {
	ID          int64
	TestRunID   string
	DeviceID    string
	CommandName string
	Success     bool
	Response    string
	DurationMs  int
	Timestamp   time.Time
}

type DeviceEvent struct {
	ID              int64
	DeviceID        string
	StationInstance string
	EventType       string // "connected", "disconnected", "error", etc.
	Details         string
	Timestamp       time.Time
}

type Employee struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

type RMA struct {
	ID                string
	RMANumber         string
	PumpSerialNumber  string
	CustomerName      string
	PumpModel         string
	EmployeeID        string
	Status            string // "open", "closed"
	CreatedAt         time.Time
	ClosedAt          *time.Time
	Notes             string
}

type StationState struct {
	StationInstance  string
	State            string // "offline", "idle", "testing", "paused"
	CurrentTestRunID *string
	UpdatedAt        time.Time
}

type TemperatureSample struct {
	ID              int64
	TestRunID       string
	StationInstance string
	DeviceID        string
	Stage           string // "first_stage", "second_stage"
	TemperatureK    float64
	Timestamp       time.Time
}

type TestEvent struct {
	ID         int64
	TestRunID  string
	EventType  string // "started", "paused", "resumed", "terminated", "aborted", "completed"
	EmployeeID string
	Reason     string
	Timestamp  time.Time
}

type Store struct {
	db *sql.DB
}

func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// SQLite requires single-connection mode for :memory: databases
	// (each pool connection gets its own in-memory DB otherwise).
	// For file-based DBs this also avoids "database is locked" errors.
	db.SetMaxOpenConns(1)

	schema := `
CREATE TABLE IF NOT EXISTS test_runs (
    id TEXT PRIMARY KEY,
    script_name TEXT NOT NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    status TEXT NOT NULL,
    summary TEXT DEFAULT ''
);

CREATE TABLE IF NOT EXISTS measurements (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    test_run_id TEXT NOT NULL REFERENCES test_runs(id),
    device_id TEXT NOT NULL,
    command_name TEXT NOT NULL,
    success INTEGER NOT NULL,
    response TEXT DEFAULT '',
    duration_ms INTEGER DEFAULT 0,
    timestamp TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS device_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL,
    station_instance TEXT NOT NULL,
    event_type TEXT NOT NULL,
    details TEXT DEFAULT '',
    timestamp TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS employees (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS rmas (
    id TEXT PRIMARY KEY,
    rma_number TEXT NOT NULL UNIQUE,
    pump_serial_number TEXT NOT NULL,
    customer_name TEXT NOT NULL,
    pump_model TEXT NOT NULL,
    employee_id TEXT NOT NULL REFERENCES employees(id),
    status TEXT NOT NULL DEFAULT 'open',
    created_at TEXT NOT NULL,
    closed_at TEXT,
    notes TEXT DEFAULT ''
);

CREATE TABLE IF NOT EXISTS station_states (
    station_instance TEXT PRIMARY KEY,
    state TEXT NOT NULL DEFAULT 'offline',
    current_test_run_id TEXT,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS temperature_samples (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    test_run_id TEXT NOT NULL REFERENCES test_runs(id),
    station_instance TEXT NOT NULL,
    device_id TEXT NOT NULL,
    stage TEXT NOT NULL,
    temperature_k REAL NOT NULL,
    timestamp TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS test_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    test_run_id TEXT NOT NULL REFERENCES test_runs(id),
    event_type TEXT NOT NULL,
    employee_id TEXT DEFAULT '',
    reason TEXT DEFAULT '',
    timestamp TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_temperature_samples_run ON temperature_samples(test_run_id);
CREATE INDEX IF NOT EXISTS idx_temperature_samples_run_ts ON temperature_samples(test_run_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_test_events_run ON test_events(test_run_id);`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}

	// Migrate existing test_runs table to add new columns
	if err := migrateTestRuns(db); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

// migrateTestRuns adds new columns to test_runs if they don't already exist.
func migrateTestRuns(db *sql.DB) error {
	columns := []struct {
		name string
		def  string
	}{
		{"rma_id", "TEXT"},
		{"station_instance", "TEXT"},
		{"script_sha256", "TEXT"},
		{"script_content", "TEXT"},
	}
	for _, col := range columns {
		_, err := db.Exec(fmt.Sprintf("ALTER TABLE test_runs ADD COLUMN %s %s", col.name, col.def))
		if err != nil {
			// Ignore "duplicate column" errors
			if !isDuplicateColumnError(err) {
				return fmt.Errorf("migrate test_runs add %s: %w", col.name, err)
			}
		}
	}
	return nil
}

func isDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// SQLite returns "duplicate column name: ..."
	return len(msg) > 0 && (contains(msg, "duplicate column") || contains(msg, "already exists"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB for use by packages that need direct access.
func (s *Store) DB() *sql.DB {
	return s.db
}

// ---------------------------------------------------------------------------
// Test Runs
// ---------------------------------------------------------------------------

func (s *Store) CreateTestRun(id, scriptName string) error {
	_, err := s.db.Exec(
		`INSERT INTO test_runs (id, script_name, started_at, status, summary) VALUES (?, ?, ?, ?, ?)`,
		id, scriptName, time.Now().UTC().Format(time.RFC3339Nano), "running", "",
	)
	return err
}

func (s *Store) CreateTestRunWithRMA(id, scriptName, rmaID, stationInstance, scriptSHA256, scriptContent string) error {
	_, err := s.db.Exec(
		`INSERT INTO test_runs (id, script_name, started_at, status, summary, rma_id, station_instance, script_sha256, script_content) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, scriptName, time.Now().UTC().Format(time.RFC3339Nano), "running", "",
		rmaID, stationInstance, scriptSHA256, scriptContent,
	)
	return err
}

func (s *Store) FinishTestRun(id, status, summary string) error {
	_, err := s.db.Exec(
		`UPDATE test_runs SET finished_at = ?, status = ?, summary = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), status, summary, id,
	)
	return err
}

func (s *Store) GetTestRun(id string) (*TestRun, error) {
	var r TestRun
	var startedAt string
	var finishedAt, rmaID, stationInstance, scriptSHA256, scriptContent sql.NullString
	err := s.db.QueryRow(
		`SELECT id, script_name, started_at, finished_at, status, summary,
		        rma_id, station_instance, script_sha256, script_content
		 FROM test_runs WHERE id = ?`, id,
	).Scan(&r.ID, &r.ScriptName, &startedAt, &finishedAt, &r.Status, &r.Summary,
		&rmaID, &stationInstance, &scriptSHA256, &scriptContent)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.StartedAt, err = time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return nil, err
	}
	if finishedAt.Valid {
		t, err := time.Parse(time.RFC3339Nano, finishedAt.String)
		if err != nil {
			return nil, err
		}
		r.FinishedAt = &t
	}
	if rmaID.Valid {
		r.RMAID = &rmaID.String
	}
	if stationInstance.Valid {
		r.StationInstance = &stationInstance.String
	}
	if scriptSHA256.Valid {
		r.ScriptSHA256 = &scriptSHA256.String
	}
	if scriptContent.Valid {
		r.ScriptContent = &scriptContent.String
	}
	return &r, nil
}

func (s *Store) QueryTestRuns() ([]TestRun, error) {
	rows, err := s.db.Query(`SELECT id, script_name, started_at, finished_at, status, summary,
	                                rma_id, station_instance, script_sha256, script_content
	                         FROM test_runs ORDER BY started_at DESC, _rowid_ DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := []TestRun{}
	for rows.Next() {
		var r TestRun
		var startedAt string
		var finishedAt, rmaID, stationInstance, scriptSHA256, scriptContent sql.NullString
		if err := rows.Scan(&r.ID, &r.ScriptName, &startedAt, &finishedAt, &r.Status, &r.Summary,
			&rmaID, &stationInstance, &scriptSHA256, &scriptContent); err != nil {
			return nil, err
		}
		r.StartedAt, err = time.Parse(time.RFC3339Nano, startedAt)
		if err != nil {
			return nil, err
		}
		if finishedAt.Valid {
			t, err := time.Parse(time.RFC3339Nano, finishedAt.String)
			if err != nil {
				return nil, err
			}
			r.FinishedAt = &t
		}
		if rmaID.Valid {
			r.RMAID = &rmaID.String
		}
		if stationInstance.Valid {
			r.StationInstance = &stationInstance.String
		}
		if scriptSHA256.Valid {
			r.ScriptSHA256 = &scriptSHA256.String
		}
		if scriptContent.Valid {
			r.ScriptContent = &scriptContent.String
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func (s *Store) QueryTestRunsByRMA(rmaID string) ([]TestRun, error) {
	rows, err := s.db.Query(
		`SELECT id, script_name, started_at, finished_at, status, summary,
		        rma_id, station_instance, script_sha256, script_content
		 FROM test_runs WHERE rma_id = ? ORDER BY started_at ASC`,
		rmaID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := []TestRun{}
	for rows.Next() {
		var r TestRun
		var startedAt string
		var finishedAt, rid, stationInstance, scriptSHA256, scriptContent sql.NullString
		if err := rows.Scan(&r.ID, &r.ScriptName, &startedAt, &finishedAt, &r.Status, &r.Summary,
			&rid, &stationInstance, &scriptSHA256, &scriptContent); err != nil {
			return nil, err
		}
		r.StartedAt, err = time.Parse(time.RFC3339Nano, startedAt)
		if err != nil {
			return nil, err
		}
		if finishedAt.Valid {
			t, err := time.Parse(time.RFC3339Nano, finishedAt.String)
			if err != nil {
				return nil, err
			}
			r.FinishedAt = &t
		}
		if rid.Valid {
			r.RMAID = &rid.String
		}
		if stationInstance.Valid {
			r.StationInstance = &stationInstance.String
		}
		if scriptSHA256.Valid {
			r.ScriptSHA256 = &scriptSHA256.String
		}
		if scriptContent.Valid {
			r.ScriptContent = &scriptContent.String
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func (s *Store) DeleteTestRun(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM temperature_samples WHERE test_run_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM test_events WHERE test_run_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM measurements WHERE test_run_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM test_runs WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Measurements
// ---------------------------------------------------------------------------

func (s *Store) RecordCommandResult(testRunID, deviceID, commandName string, success bool, response string, durationMs int) error {
	successInt := 0
	if success {
		successInt = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO measurements (test_run_id, device_id, command_name, success, response, duration_ms, timestamp) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		testRunID, deviceID, commandName, successInt, response, durationMs, time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) QueryMeasurements(testRunID string) ([]Measurement, error) {
	rows, err := s.db.Query(
		`SELECT id, test_run_id, device_id, command_name, success, response, duration_ms, timestamp FROM measurements WHERE test_run_id = ? ORDER BY timestamp ASC`,
		testRunID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	measurements := []Measurement{}
	for rows.Next() {
		var m Measurement
		var successInt int
		var ts string
		if err := rows.Scan(&m.ID, &m.TestRunID, &m.DeviceID, &m.CommandName, &successInt, &m.Response, &m.DurationMs, &ts); err != nil {
			return nil, err
		}
		m.Success = successInt != 0
		m.Timestamp, err = time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return nil, err
		}
		measurements = append(measurements, m)
	}
	return measurements, rows.Err()
}

// ---------------------------------------------------------------------------
// Device History
// ---------------------------------------------------------------------------

func (s *Store) RecordDeviceEvent(deviceID, stationInstance, eventType, details string) error {
	_, err := s.db.Exec(
		`INSERT INTO device_history (device_id, station_instance, event_type, details, timestamp) VALUES (?, ?, ?, ?, ?)`,
		deviceID, stationInstance, eventType, details, time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

// ---------------------------------------------------------------------------
// Employees
// ---------------------------------------------------------------------------

func (s *Store) CreateEmployee(id, name string) error {
	_, err := s.db.Exec(
		`INSERT INTO employees (id, name, created_at) VALUES (?, ?, ?)`,
		id, name, time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) UpsertEmployee(id, name string) error {
	_, err := s.db.Exec(
		`INSERT INTO employees (id, name, created_at) VALUES (?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET name = excluded.name`,
		id, name, time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) GetEmployee(id string) (*Employee, error) {
	var e Employee
	var createdAt string
	err := s.db.QueryRow(
		`SELECT id, name, created_at FROM employees WHERE id = ?`, id,
	).Scan(&e.ID, &e.Name, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *Store) ListEmployees() ([]Employee, error) {
	rows, err := s.db.Query(`SELECT id, name, created_at FROM employees ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	employees := []Employee{}
	for rows.Next() {
		var e Employee
		var createdAt string
		if err := rows.Scan(&e.ID, &e.Name, &createdAt); err != nil {
			return nil, err
		}
		e.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		employees = append(employees, e)
	}
	return employees, rows.Err()
}

// ---------------------------------------------------------------------------
// RMAs
// ---------------------------------------------------------------------------

func (s *Store) CreateRMA(id, rmaNumber, pumpSerialNumber, customerName, pumpModel, employeeID, notes string) error {
	_, err := s.db.Exec(
		`INSERT INTO rmas (id, rma_number, pump_serial_number, customer_name, pump_model, employee_id, status, created_at, notes)
		 VALUES (?, ?, ?, ?, ?, ?, 'open', ?, ?)`,
		id, rmaNumber, pumpSerialNumber, customerName, pumpModel, employeeID,
		time.Now().UTC().Format(time.RFC3339Nano), notes,
	)
	return err
}

func (s *Store) GetRMA(id string) (*RMA, error) {
	var r RMA
	var createdAt string
	var closedAt sql.NullString
	err := s.db.QueryRow(
		`SELECT id, rma_number, pump_serial_number, customer_name, pump_model, employee_id, status, created_at, closed_at, notes
		 FROM rmas WHERE id = ?`, id,
	).Scan(&r.ID, &r.RMANumber, &r.PumpSerialNumber, &r.CustomerName, &r.PumpModel,
		&r.EmployeeID, &r.Status, &createdAt, &closedAt, &r.Notes)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, err
	}
	if closedAt.Valid {
		t, err := time.Parse(time.RFC3339Nano, closedAt.String)
		if err != nil {
			return nil, err
		}
		r.ClosedAt = &t
	}
	return &r, nil
}

func (s *Store) GetRMAByNumber(rmaNumber string) (*RMA, error) {
	var r RMA
	var createdAt string
	var closedAt sql.NullString
	err := s.db.QueryRow(
		`SELECT id, rma_number, pump_serial_number, customer_name, pump_model, employee_id, status, created_at, closed_at, notes
		 FROM rmas WHERE rma_number = ?`, rmaNumber,
	).Scan(&r.ID, &r.RMANumber, &r.PumpSerialNumber, &r.CustomerName, &r.PumpModel,
		&r.EmployeeID, &r.Status, &createdAt, &closedAt, &r.Notes)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, err
	}
	if closedAt.Valid {
		t, err := time.Parse(time.RFC3339Nano, closedAt.String)
		if err != nil {
			return nil, err
		}
		r.ClosedAt = &t
	}
	return &r, nil
}

func (s *Store) CloseRMA(id string) error {
	_, err := s.db.Exec(
		`UPDATE rmas SET status = 'closed', closed_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), id,
	)
	return err
}

func (s *Store) ListRMAs(status string) ([]RMA, error) {
	var rows *sql.Rows
	var err error
	if status != "" {
		rows, err = s.db.Query(
			`SELECT id, rma_number, pump_serial_number, customer_name, pump_model, employee_id, status, created_at, closed_at, notes
			 FROM rmas WHERE status = ? ORDER BY rma_number ASC`, status)
	} else {
		rows, err = s.db.Query(
			`SELECT id, rma_number, pump_serial_number, customer_name, pump_model, employee_id, status, created_at, closed_at, notes
			 FROM rmas ORDER BY rma_number ASC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rmas := []RMA{}
	for rows.Next() {
		var r RMA
		var createdAt string
		var closedAt sql.NullString
		if err := rows.Scan(&r.ID, &r.RMANumber, &r.PumpSerialNumber, &r.CustomerName, &r.PumpModel,
			&r.EmployeeID, &r.Status, &createdAt, &closedAt, &r.Notes); err != nil {
			return nil, err
		}
		r.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		if closedAt.Valid {
			t, err := time.Parse(time.RFC3339Nano, closedAt.String)
			if err != nil {
				return nil, err
			}
			r.ClosedAt = &t
		}
		rmas = append(rmas, r)
	}
	return rmas, rows.Err()
}

func (s *Store) SearchRMAs(query string) ([]RMA, error) {
	like := "%" + query + "%"
	rows, err := s.db.Query(
		`SELECT id, rma_number, pump_serial_number, customer_name, pump_model, employee_id, status, created_at, closed_at, notes
		 FROM rmas WHERE rma_number LIKE ? OR pump_serial_number LIKE ? OR customer_name LIKE ?
		 ORDER BY rma_number ASC`,
		like, like, like,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rmas := []RMA{}
	for rows.Next() {
		var r RMA
		var createdAt string
		var closedAt sql.NullString
		if err := rows.Scan(&r.ID, &r.RMANumber, &r.PumpSerialNumber, &r.CustomerName, &r.PumpModel,
			&r.EmployeeID, &r.Status, &createdAt, &closedAt, &r.Notes); err != nil {
			return nil, err
		}
		r.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		if closedAt.Valid {
			t, err := time.Parse(time.RFC3339Nano, closedAt.String)
			if err != nil {
				return nil, err
			}
			r.ClosedAt = &t
		}
		rmas = append(rmas, r)
	}
	return rmas, rows.Err()
}

// ---------------------------------------------------------------------------
// Station States
// ---------------------------------------------------------------------------

func (s *Store) SetStationState(stationInstance, state string, testRunID *string) error {
	_, err := s.db.Exec(
		`INSERT INTO station_states (station_instance, state, current_test_run_id, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(station_instance) DO UPDATE SET state = excluded.state, current_test_run_id = excluded.current_test_run_id, updated_at = excluded.updated_at`,
		stationInstance, state, testRunID, time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) GetStationState(stationInstance string) (*StationState, error) {
	var ss StationState
	var updatedAt string
	var testRunID sql.NullString
	err := s.db.QueryRow(
		`SELECT station_instance, state, current_test_run_id, updated_at FROM station_states WHERE station_instance = ?`,
		stationInstance,
	).Scan(&ss.StationInstance, &ss.State, &testRunID, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ss.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return nil, err
	}
	if testRunID.Valid {
		ss.CurrentTestRunID = &testRunID.String
	}
	return &ss, nil
}

func (s *Store) ListStationStates() ([]StationState, error) {
	rows, err := s.db.Query(`SELECT station_instance, state, current_test_run_id, updated_at FROM station_states ORDER BY station_instance ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	states := []StationState{}
	for rows.Next() {
		var ss StationState
		var updatedAt string
		var testRunID sql.NullString
		if err := rows.Scan(&ss.StationInstance, &ss.State, &testRunID, &updatedAt); err != nil {
			return nil, err
		}
		ss.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, err
		}
		if testRunID.Valid {
			ss.CurrentTestRunID = &testRunID.String
		}
		states = append(states, ss)
	}
	return states, rows.Err()
}

// ---------------------------------------------------------------------------
// Temperature Samples
// ---------------------------------------------------------------------------

func (s *Store) RecordTemperature(testRunID, stationInstance, deviceID, stage string, temperatureK float64) error {
	_, err := s.db.Exec(
		`INSERT INTO temperature_samples (test_run_id, station_instance, device_id, stage, temperature_k, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		testRunID, stationInstance, deviceID, stage, temperatureK,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) QueryTemperatures(testRunID string) ([]TemperatureSample, error) {
	rows, err := s.db.Query(
		`SELECT id, test_run_id, station_instance, device_id, stage, temperature_k, timestamp
		 FROM temperature_samples WHERE test_run_id = ? ORDER BY timestamp ASC`,
		testRunID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	samples := []TemperatureSample{}
	for rows.Next() {
		var ts TemperatureSample
		var timestamp string
		if err := rows.Scan(&ts.ID, &ts.TestRunID, &ts.StationInstance, &ts.DeviceID, &ts.Stage, &ts.TemperatureK, &timestamp); err != nil {
			return nil, err
		}
		ts.Timestamp, err = time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return nil, err
		}
		samples = append(samples, ts)
	}
	return samples, rows.Err()
}

func (s *Store) QueryTemperaturesSince(testRunID string, since time.Time) ([]TemperatureSample, error) {
	rows, err := s.db.Query(
		`SELECT id, test_run_id, station_instance, device_id, stage, temperature_k, timestamp
		 FROM temperature_samples WHERE test_run_id = ? AND timestamp > ? ORDER BY timestamp ASC`,
		testRunID, since.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	samples := []TemperatureSample{}
	for rows.Next() {
		var ts TemperatureSample
		var timestamp string
		if err := rows.Scan(&ts.ID, &ts.TestRunID, &ts.StationInstance, &ts.DeviceID, &ts.Stage, &ts.TemperatureK, &timestamp); err != nil {
			return nil, err
		}
		ts.Timestamp, err = time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return nil, err
		}
		samples = append(samples, ts)
	}
	return samples, rows.Err()
}

// ---------------------------------------------------------------------------
// Test Events
// ---------------------------------------------------------------------------

func (s *Store) RecordTestEvent(testRunID, eventType, employeeID, reason string) error {
	_, err := s.db.Exec(
		`INSERT INTO test_events (test_run_id, event_type, employee_id, reason, timestamp)
		 VALUES (?, ?, ?, ?, ?)`,
		testRunID, eventType, employeeID, reason,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) QueryTestEvents(testRunID string) ([]TestEvent, error) {
	rows, err := s.db.Query(
		`SELECT id, test_run_id, event_type, employee_id, reason, timestamp
		 FROM test_events WHERE test_run_id = ? ORDER BY timestamp ASC`,
		testRunID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []TestEvent{}
	for rows.Next() {
		var te TestEvent
		var timestamp string
		if err := rows.Scan(&te.ID, &te.TestRunID, &te.EventType, &te.EmployeeID, &te.Reason, &timestamp); err != nil {
			return nil, err
		}
		te.Timestamp, err = time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return nil, err
		}
		events = append(events, te)
	}
	return events, rows.Err()
}
