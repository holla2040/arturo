package store

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

type TestRun struct {
	ID         string
	ScriptName string
	StartedAt  time.Time
	FinishedAt *time.Time
	Status     string // "running", "passed", "failed", "error"
	Summary    string
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

type Store struct {
	db *sql.DB
}

func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

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
);`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) CreateTestRun(id, scriptName string) error {
	_, err := s.db.Exec(
		`INSERT INTO test_runs (id, script_name, started_at, status, summary) VALUES (?, ?, ?, ?, ?)`,
		id, scriptName, time.Now().UTC().Format(time.RFC3339Nano), "running", "",
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

func (s *Store) RecordDeviceEvent(deviceID, stationInstance, eventType, details string) error {
	_, err := s.db.Exec(
		`INSERT INTO device_history (device_id, station_instance, event_type, details, timestamp) VALUES (?, ?, ?, ?, ?)`,
		deviceID, stationInstance, eventType, details, time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) QueryTestRuns() ([]TestRun, error) {
	rows, err := s.db.Query(`SELECT id, script_name, started_at, finished_at, status, summary FROM test_runs ORDER BY started_at DESC, _rowid_ DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := []TestRun{}
	for rows.Next() {
		var r TestRun
		var startedAt string
		var finishedAt sql.NullString
		if err := rows.Scan(&r.ID, &r.ScriptName, &startedAt, &finishedAt, &r.Status, &r.Summary); err != nil {
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
		runs = append(runs, r)
	}
	return runs, rows.Err()
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

func (s *Store) GetTestRun(id string) (*TestRun, error) {
	var r TestRun
	var startedAt string
	var finishedAt sql.NullString
	err := s.db.QueryRow(
		`SELECT id, script_name, started_at, finished_at, status, summary FROM test_runs WHERE id = ?`, id,
	).Scan(&r.ID, &r.ScriptName, &startedAt, &finishedAt, &r.Status, &r.Summary)
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
	return &r, nil
}
