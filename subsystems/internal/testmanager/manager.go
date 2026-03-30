package testmanager

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/holla2040/arturo/internal/protocol"
	"github.com/holla2040/arturo/internal/script/executor"
	"github.com/holla2040/arturo/internal/script/redisrouter"
	"github.com/holla2040/arturo/internal/store"
	"github.com/redis/go-redis/v9"
)

// Broadcaster sends events to connected clients (e.g., WebSocket).
type Broadcaster interface {
	BroadcastEvent(eventType string, payload interface{})
}

// RouterFactory creates a DeviceRouter for a given station.
// This allows injecting mock routers in tests.
type RouterFactory func(station string) executor.DeviceRouter

// TestManager manages all active test sessions across stations.
type TestManager struct {
	mu            sync.RWMutex
	sessions      map[string]*TestSession // keyed by station instance
	store         *store.Store
	hub           Broadcaster
	routerFactory RouterFactory
	ctx           context.Context
}

// New creates a new TestManager.
func New(ctx context.Context, st *store.Store, hub Broadcaster, rdb *redis.Client, source protocol.Source) *TestManager {
	return &TestManager{
		sessions: make(map[string]*TestSession),
		store:    st,
		hub:      hub,
		ctx:      ctx,
		routerFactory: func(station string) executor.DeviceRouter {
			return redisrouter.New(rdb, source, station)
		},
	}
}

// NewWithFactory creates a TestManager with a custom router factory (for testing).
func NewWithFactory(ctx context.Context, st *store.Store, hub Broadcaster, factory RouterFactory) *TestManager {
	return &TestManager{
		sessions:      make(map[string]*TestSession),
		store:         st,
		hub:           hub,
		ctx:           ctx,
		routerFactory: factory,
	}
}

// StartTest starts a test on the given station.
func (m *TestManager) StartTest(stationInstance, deviceID, scriptPath, rmaID, testRunID, employeeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[stationInstance]; exists {
		return fmt.Errorf("station %s already has an active test", stationInstance)
	}

	rawRouter := m.routerFactory(stationInstance)

	session, err := NewSession(m.ctx, StartSessionParams{
		TestRunID:       testRunID,
		RMAID:           rmaID,
		StationInstance: stationInstance,
		DeviceID:        deviceID,
		ScriptPath:      scriptPath,
		EmployeeID:      employeeID,
		RawRouter:       rawRouter,
		Store:           m.store,
		Hub:             m.hub,
	})
	if err != nil {
		return err
	}

	m.sessions[stationInstance] = session

	// Watch for session completion to clean up
	go func() {
		<-session.Done()
		m.mu.Lock()
		delete(m.sessions, stationInstance)
		m.mu.Unlock()
	}()

	return nil
}

// PauseTest pauses the test on the given station.
func (m *TestManager) PauseTest(stationInstance, employeeID string) error {
	m.mu.RLock()
	session, exists := m.sessions[stationInstance]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no active test on station %s", stationInstance)
	}

	return session.Pause(employeeID)
}

// ResumeTest resumes the test on the given station.
func (m *TestManager) ResumeTest(stationInstance, employeeID string) error {
	m.mu.RLock()
	session, exists := m.sessions[stationInstance]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no active test on station %s", stationInstance)
	}

	return session.Resume(employeeID)
}

// TerminateTest terminates the test on the given station, preserving data.
func (m *TestManager) TerminateTest(stationInstance, employeeID, reason string) error {
	m.mu.RLock()
	session, exists := m.sessions[stationInstance]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no active test on station %s", stationInstance)
	}

	return session.Terminate(employeeID, reason)
}

// AbortTest aborts the test on the given station, discarding data.
func (m *TestManager) AbortTest(stationInstance, employeeID string) error {
	m.mu.RLock()
	session, exists := m.sessions[stationInstance]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no active test on station %s", stationInstance)
	}

	return session.Abort(employeeID)
}

// GetStationState returns the current state of a station.
func (m *TestManager) GetStationState(stationInstance string) string {
	m.mu.RLock()
	session, exists := m.sessions[stationInstance]
	m.mu.RUnlock()

	if !exists {
		return "idle"
	}

	info := session.Info()
	return string(info.State)
}

// GetSession returns session info for a station, or nil if no active session.
func (m *TestManager) GetSession(stationInstance string) *SessionInfo {
	m.mu.RLock()
	session, exists := m.sessions[stationInstance]
	m.mu.RUnlock()

	if !exists {
		return nil
	}

	info := session.Info()
	return &info
}

// ListSessions returns info for all active sessions.
func (m *TestManager) ListSessions() []SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	infos := make([]SessionInfo, 0, len(m.sessions))
	for _, session := range m.sessions {
		infos = append(infos, session.Info())
	}
	return infos
}

// EmergencyStopAll terminates all running tests.
func (m *TestManager) EmergencyStopAll() {
	m.mu.RLock()
	sessions := make([]*TestSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.mu.RUnlock()

	for _, session := range sessions {
		info := session.Info()
		if err := session.Terminate("system", "emergency stop"); err != nil {
			log.Printf("testmanager: e-stop terminate %s: %v", info.StationInstance, err)
		}
	}
}

// HandleHeartbeat updates station state when a heartbeat is received.
// If the station has no active session, ensures it's marked as idle.
func (m *TestManager) HandleHeartbeat(stationInstance string) {
	m.mu.RLock()
	_, hasSession := m.sessions[stationInstance]
	m.mu.RUnlock()

	if !hasSession {
		// Station is alive but not testing — make sure it's idle in the store
		m.store.SetStationState(stationInstance, "idle", nil)
	}
}

// HandleOffline marks a station as offline when its heartbeat times out.
func (m *TestManager) HandleOffline(stationInstance string) {
	m.mu.RLock()
	session, hasSession := m.sessions[stationInstance]
	m.mu.RUnlock()

	// If there's an active session, terminate it
	if hasSession {
		info := session.Info()
		if err := session.Terminate("system", "station went offline"); err != nil {
			log.Printf("testmanager: offline terminate %s: %v", info.StationInstance, err)
		}
	}

	m.store.SetStationState(stationInstance, "offline", nil)

	if m.hub != nil {
		m.hub.BroadcastEvent("station_state", map[string]interface{}{
			"station_instance": stationInstance,
			"state":            "offline",
			"test_run_id":      nil,
		})
	}
}

// HasActiveSession returns true if the station has an active test.
func (m *TestManager) HasActiveSession(stationInstance string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.sessions[stationInstance]
	return exists
}

// HasActiveTestForRMA returns true if any station has an active test for the given RMA.
func (m *TestManager) HasActiveTestForRMA(rmaID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, session := range m.sessions {
		info := session.Info()
		if info.RMAID == rmaID {
			return true
		}
	}
	return false
}
