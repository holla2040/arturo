package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/holla2040/arturo/internal/artifact"
	"github.com/holla2040/arturo/internal/estop"
	"github.com/holla2040/arturo/internal/protocol"
	"github.com/holla2040/arturo/internal/redishealth"
	"github.com/holla2040/arturo/internal/registry"
	"github.com/holla2040/arturo/internal/report"
	"github.com/holla2040/arturo/internal/store"
	"github.com/holla2040/arturo/internal/testmanager"
)

// CommandSender abstracts the Redis PUBLISH operation for testability.
type CommandSender interface {
	SendCommand(ctx context.Context, channel string, msg *protocol.Message) error
}

// commandRequest is the JSON body for POST /devices/{id}/command.
type commandRequest struct {
	Command    string            `json:"command"`
	Parameters map[string]string `json:"parameters,omitempty"`
	TimeoutMs  int               `json:"timeout_ms,omitempty"`
}

// stationCommandRequest is the JSON body for POST /stations/{id}/command.
// It includes a device_id field since the URL path {id} is the station instance.
type stationCommandRequest struct {
	DeviceID   string            `json:"device_id"`
	Command    string            `json:"command"`
	Parameters map[string]string `json:"parameters,omitempty"`
	TimeoutMs  int               `json:"timeout_ms,omitempty"`
}

// otaRequest is the JSON body for POST /ota.
type otaRequest struct {
	Station     string `json:"station"`
	FirmwareURL string `json:"firmware_url"`
	Version     string `json:"version"`
	SHA256      string `json:"sha256"`
	Force       bool   `json:"force"`
}

// RedisHealthChecker provides Redis connection health information.
type RedisHealthChecker interface {
	IsConnected() bool
	GetStatus() redishealth.Status
}

// systemStatus is the response for GET /system/status.
type systemStatus struct {
	StationCount int                `json:"station_count"`
	DeviceCount  int                `json:"device_count"`
	EstopState   estop.State        `json:"estop_state"`
	RedisHealth  *redishealth.Status `json:"redis_health,omitempty"`
}

// Handler holds all dependencies for HTTP request handling.
type Handler struct {
	Registry    *registry.Registry
	Store       *store.Store
	Estop       *estop.Coordinator
	Dispatcher  *ResponseDispatcher
	Sender      CommandSender
	Source      protocol.Source
	RedisHealth RedisHealthChecker       // nil means no health checking
	TestMgr     *testmanager.TestManager // nil means no test management
	ReportDir   string                   // local report storage (e.g., /var/lib/arturo/reports)
	SMBMountDir string                   // CIFS mount point (e.g., /mnt/reports)
}

// RegisterRoutes adds all API routes to the given ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Existing routes
	mux.HandleFunc("GET /devices", h.listDevices)
	mux.HandleFunc("GET /devices/{id}", h.getDevice)
	mux.HandleFunc("POST /devices/{id}/command", h.sendCommand)
	mux.HandleFunc("GET /stations", h.listStations)
	mux.HandleFunc("GET /system/status", h.getSystemStatus)
	mux.HandleFunc("GET /test-runs", h.listTestRuns)
	mux.HandleFunc("GET /reports/{id}/csv", h.exportCSV)
	mux.HandleFunc("GET /reports/{id}/json", h.exportJSON)
	mux.HandleFunc("GET /reports/{id}/pdf", h.exportPDF)
	mux.HandleFunc("POST /ota", h.triggerOTA)

	// Auth routes
	mux.HandleFunc("POST /auth/login", h.handleLogin)
	mux.HandleFunc("GET /auth/session", h.handleSession)

	// RMA routes
	mux.HandleFunc("POST /rmas", h.createRMA)
	mux.HandleFunc("GET /rmas", h.listRMAs)
	mux.HandleFunc("GET /rmas/search", h.searchRMAs)
	mux.HandleFunc("GET /rmas/{id}", h.getRMA)
	mux.HandleFunc("POST /rmas/{id}/close", h.closeRMA)

	// Station test control routes
	mux.HandleFunc("POST /stations/{id}/test/start", h.startTest)
	mux.HandleFunc("POST /stations/{id}/test/pause", h.pauseTest)
	mux.HandleFunc("POST /stations/{id}/test/resume", h.resumeTest)
	mux.HandleFunc("POST /stations/{id}/test/terminate", h.terminateTest)
	mux.HandleFunc("POST /stations/{id}/test/abort", h.abortTest)
	mux.HandleFunc("GET /stations/{id}/state", h.getStationState)
	mux.HandleFunc("POST /stations/{id}/command", h.stationCommand)

	// Continuous temperature log route
	mux.HandleFunc("GET /stations/{id}/temperatures", h.getStationTemperatures)

	// Test run data routes
	mux.HandleFunc("GET /test-runs/{id}/temperatures", h.getTemperatures)
	mux.HandleFunc("GET /test-runs/{id}/events", h.getTestEvents)

	// Artifact routes
	mux.HandleFunc("GET /rmas/{id}/artifact", h.getRMAArtifact)
	mux.HandleFunc("GET /rmas/{id}/pdf", h.getRMAPDF)
}

func (h *Handler) listDevices(w http.ResponseWriter, r *http.Request) {
	devices := h.Registry.ListDevices()
	writeJSON(w, http.StatusOK, devices)
}

func (h *Handler) getDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	device := h.Registry.LookupDevice(id)
	if device == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "device not found"})
		return
	}
	writeJSON(w, http.StatusOK, device)
}

// redisAvailable returns true if the Redis health checker is nil (not configured)
// or reports connected. Returns false only when we know Redis is down.
func (h *Handler) redisAvailable() bool {
	if h.RedisHealth == nil {
		return true
	}
	return h.RedisHealth.IsConnected()
}

func (h *Handler) sendCommand(w http.ResponseWriter, r *http.Request) {
	deviceID := r.PathValue("id")

	var req commandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	h.executeCommand(w, r, deviceID, req.Command, req.Parameters, req.TimeoutMs)
}

// executeCommand is the shared core for sending a device command and waiting for a response.
func (h *Handler) executeCommand(w http.ResponseWriter, r *http.Request, deviceID, command string, parameters map[string]string, timeoutMs int) {
	if !h.redisAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "redis unavailable"})
		return
	}

	device := h.Registry.LookupDevice(deviceID)
	if device == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "device not found"})
		return
	}

	if command == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "command is required"})
		return
	}

	if timeoutMs <= 0 {
		timeoutMs = 5000
	}

	msg, err := protocol.BuildCommandRequest(h.Source, deviceID, command, parameters, timeoutMs)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to build command: %v", err)})
		return
	}

	// Register waiter before sending command
	waiterCh := h.Dispatcher.Register(msg.Envelope.CorrelationID)

	if err := h.Sender.SendCommand(r.Context(), device.CommandStream, msg); err != nil {
		h.Dispatcher.Deregister(msg.Envelope.CorrelationID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to send command: %v", err)})
		return
	}

	// Wait for response with timeout
	timeout := time.Duration(timeoutMs) * time.Millisecond
	select {
	case resp := <-waiterCh:
		payload, err := protocol.ParseCommandResponse(resp)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to parse response: %v", err)})
			return
		}
		writeJSON(w, http.StatusOK, payload)

	case <-time.After(timeout):
		h.Dispatcher.Deregister(msg.Envelope.CorrelationID)
		writeJSON(w, http.StatusGatewayTimeout, map[string]string{
			"error":          "command timed out",
			"correlation_id": msg.Envelope.CorrelationID,
		})

	case <-r.Context().Done():
		h.Dispatcher.Deregister(msg.Envelope.CorrelationID)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "request cancelled"})
	}
}

func (h *Handler) listStations(w http.ResponseWriter, r *http.Request) {
	stations := h.Registry.ListStations()
	writeJSON(w, http.StatusOK, stations)
}

func (h *Handler) listTestRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.Store.QueryTestRuns()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to query test runs: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (h *Handler) getSystemStatus(w http.ResponseWriter, r *http.Request) {
	stations := h.Registry.ListStations()
	devices := h.Registry.ListDevices()

	status := systemStatus{
		StationCount: len(stations),
		DeviceCount:  len(devices),
		EstopState:   h.Estop.GetState(),
	}
	if h.RedisHealth != nil {
		rh := h.RedisHealth.GetStatus()
		status.RedisHealth = &rh
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) exportCSV(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.csv", id))
	if err := report.ExportCSV(w, h.Store, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) exportJSON(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	w.Header().Set("Content-Type", "application/json")
	if err := report.ExportJSON(w, h.Store, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) exportPDF(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.pdf", id))
	if err := report.ExportPDF(w, h.Store, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) triggerOTA(w http.ResponseWriter, r *http.Request) {
	if !h.redisAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "redis unavailable"})
		return
	}

	var req otaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Station == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "station is required"})
		return
	}
	if req.FirmwareURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "firmware_url is required"})
		return
	}
	if req.Version == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "version is required"})
		return
	}
	if req.SHA256 == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sha256 is required"})
		return
	}

	// Look up the station in the registry
	stations := h.Registry.ListStations()
	var found bool
	for _, s := range stations {
		if s.Instance == req.Station {
			found = true
			break
		}
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "station not found"})
		return
	}

	// Build the OTA request message
	msg, err := protocol.BuildOTARequest(h.Source, req.FirmwareURL, req.Version, req.SHA256, req.Force)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to build OTA request: %v", err)})
		return
	}

	// Send to the station's command stream
	stream := "commands:" + req.Station
	if err := h.Sender.SendCommand(r.Context(), stream, msg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to send OTA request: %v", err)})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":         "accepted",
		"correlation_id": msg.Envelope.CorrelationID,
		"station":        req.Station,
		"version":        req.Version,
	})
}

// ---------------------------------------------------------------------------
// RMA endpoints
// ---------------------------------------------------------------------------

type createRMARequest struct {
	RMANumber        string `json:"rma_number"`
	PumpSerialNumber string `json:"pump_serial_number"`
	CustomerName     string `json:"customer_name"`
	PumpModel        string `json:"pump_model"`
	Notes            string `json:"notes"`
}

func (h *Handler) createRMA(w http.ResponseWriter, r *http.Request) {
	emp, ok := requireEmployee(h, w, r)
	if !ok {
		return
	}

	var req createRMARequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.RMANumber == "" || req.PumpSerialNumber == "" || req.CustomerName == "" || req.PumpModel == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "rma_number, pump_serial_number, customer_name, and pump_model are required"})
		return
	}

	id := uuid.New().String()
	if err := h.Store.CreateRMA(id, req.RMANumber, req.PumpSerialNumber, req.CustomerName, req.PumpModel, emp.ID, req.Notes); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": fmt.Sprintf("failed to create RMA: %v", err)})
		return
	}

	rma, err := h.Store.GetRMA(id)
	if err != nil || rma == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to retrieve created RMA"})
		return
	}

	writeJSON(w, http.StatusCreated, rma)
}

func (h *Handler) listRMAs(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	rmas, err := h.Store.ListRMAs(status)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list RMAs"})
		return
	}
	writeJSON(w, http.StatusOK, rmas)
}

func (h *Handler) searchRMAs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q parameter required"})
		return
	}

	rmas, err := h.Store.SearchRMAs(q)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "search failed"})
		return
	}
	writeJSON(w, http.StatusOK, rmas)
}

func (h *Handler) getRMA(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rma, err := h.Store.GetRMA(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get RMA"})
		return
	}
	if rma == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "RMA not found"})
		return
	}

	// Include test run history
	runs, _ := h.Store.QueryTestRunsByRMA(id)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"rma":  rma,
		"runs": runs,
	})
}

func (h *Handler) closeRMA(w http.ResponseWriter, r *http.Request) {
	_, ok := requireEmployee(h, w, r)
	if !ok {
		return
	}

	id := r.PathValue("id")
	rma, err := h.Store.GetRMA(id)
	if err != nil || rma == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "RMA not found"})
		return
	}

	if rma.Status == "closed" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "RMA is already closed"})
		return
	}

	// Check for active tests on this RMA
	if h.TestMgr != nil && h.TestMgr.HasActiveTestForRMA(id) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "RMA has an active test running"})
		return
	}

	if err := h.Store.CloseRMA(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to close RMA"})
		return
	}

	// Generate artifacts
	rma, _ = h.Store.GetRMA(id)

	var jsonBuf bytes.Buffer
	if err := artifact.GenerateJSON(&jsonBuf, h.Store, id); err != nil {
		log.Printf("closeRMA: generate JSON failed: %v", err)
	}

	var pdfBuf bytes.Buffer
	if err := artifact.GeneratePDF(&pdfBuf, h.Store, id); err != nil {
		log.Printf("closeRMA: generate PDF failed: %v", err)
	}

	// Write to local report storage
	if h.ReportDir != "" && jsonBuf.Len() > 0 {
		if err := artifact.ExportToShare(jsonBuf.Bytes(), pdfBuf.Bytes(), rma.RMANumber, h.ReportDir); err != nil {
			log.Printf("closeRMA: local export failed: %v", err)
		}
	}

	// Copy to SMB mount
	if h.SMBMountDir != "" && jsonBuf.Len() > 0 {
		if err := artifact.ExportToShare(jsonBuf.Bytes(), pdfBuf.Bytes(), rma.RMANumber, h.SMBMountDir); err != nil {
			log.Printf("closeRMA: SMB export failed: %v", err)
		}
	}

	writeJSON(w, http.StatusOK, rma)
}

// ---------------------------------------------------------------------------
// Station test control endpoints
// ---------------------------------------------------------------------------

type startTestRequest struct {
	RMAID      string `json:"rma_id"`
	ScriptPath string `json:"script_path"`
	DeviceID   string `json:"device_id"`
}

type terminateRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) startTest(w http.ResponseWriter, r *http.Request) {
	emp, ok := requireEmployee(h, w, r)
	if !ok {
		return
	}

	if h.TestMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "test manager not available"})
		return
	}

	stationID := r.PathValue("id")

	var req startTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.RMAID == "" || req.ScriptPath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "rma_id and script_path are required"})
		return
	}

	deviceID := req.DeviceID
	if deviceID == "" {
		deviceID = "PUMP-01"
	}

	testRunID := uuid.New().String()

	if err := h.TestMgr.StartTest(stationID, deviceID, req.ScriptPath, req.RMAID, testRunID, emp.ID); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"test_run_id":      testRunID,
		"station_instance": stationID,
		"status":           "started",
	})
}

func (h *Handler) pauseTest(w http.ResponseWriter, r *http.Request) {
	emp, ok := requireEmployee(h, w, r)
	if !ok {
		return
	}

	if h.TestMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "test manager not available"})
		return
	}

	stationID := r.PathValue("id")
	if err := h.TestMgr.PauseTest(stationID, emp.ID); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (h *Handler) resumeTest(w http.ResponseWriter, r *http.Request) {
	emp, ok := requireEmployee(h, w, r)
	if !ok {
		return
	}

	if h.TestMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "test manager not available"})
		return
	}

	stationID := r.PathValue("id")
	if err := h.TestMgr.ResumeTest(stationID, emp.ID); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

func (h *Handler) terminateTest(w http.ResponseWriter, r *http.Request) {
	emp, ok := requireEmployee(h, w, r)
	if !ok {
		return
	}

	if h.TestMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "test manager not available"})
		return
	}

	stationID := r.PathValue("id")

	var req terminateRequest
	json.NewDecoder(r.Body).Decode(&req) // reason is optional

	if err := h.TestMgr.TerminateTest(stationID, emp.ID, req.Reason); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "terminated"})
}

func (h *Handler) abortTest(w http.ResponseWriter, r *http.Request) {
	emp, ok := requireEmployee(h, w, r)
	if !ok {
		return
	}

	if h.TestMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "test manager not available"})
		return
	}

	stationID := r.PathValue("id")
	if err := h.TestMgr.AbortTest(stationID, emp.ID); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "aborted"})
}

func (h *Handler) getStationState(w http.ResponseWriter, r *http.Request) {
	stationID := r.PathValue("id")

	state := "offline"
	if h.TestMgr != nil {
		state = h.TestMgr.GetStationState(stationID)
	}

	result := map[string]interface{}{
		"station_instance": stationID,
		"state":            state,
	}

	if h.TestMgr != nil {
		if session := h.TestMgr.GetSession(stationID); session != nil {
			result["session"] = session
		}
	}

	// Also check the store for persisted state
	if ss, err := h.Store.GetStationState(stationID); err == nil && ss != nil {
		if state == "idle" {
			result["state"] = ss.State
		}
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) stationCommand(w http.ResponseWriter, r *http.Request) {
	if h.TestMgr != nil {
		stationID := r.PathValue("id")
		if h.TestMgr.HasActiveSession(stationID) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "station has an active test, cannot send manual commands"})
			return
		}
	}

	var req stationCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.DeviceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "device_id is required"})
		return
	}

	h.executeCommand(w, r, req.DeviceID, req.Command, req.Parameters, req.TimeoutMs)
}

// ---------------------------------------------------------------------------
// Test run data endpoints
// ---------------------------------------------------------------------------

func (h *Handler) getTemperatures(w http.ResponseWriter, r *http.Request) {
	testRunID := r.PathValue("id")

	sinceStr := r.URL.Query().Get("since")
	if sinceStr != "" {
		since, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid since parameter, use ISO8601"})
			return
		}
		temps, err := h.Store.QueryTemperaturesSince(testRunID, since)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to query temperatures"})
			return
		}
		writeJSON(w, http.StatusOK, temps)
		return
	}

	temps, err := h.Store.QueryTemperatures(testRunID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to query temperatures"})
		return
	}
	writeJSON(w, http.StatusOK, temps)
}

func (h *Handler) getStationTemperatures(w http.ResponseWriter, r *http.Request) {
	stationInstance := r.PathValue("id")

	since := time.Now().Add(-12 * time.Hour)
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		parsed, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid since parameter, use RFC3339"})
			return
		}
		since = parsed
	}

	entries, err := h.Store.QueryTemperatureLog(stationInstance, since)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to query temperature log"})
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (h *Handler) getTestEvents(w http.ResponseWriter, r *http.Request) {
	testRunID := r.PathValue("id")

	events, err := h.Store.QueryTestEvents(testRunID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to query events"})
		return
	}
	writeJSON(w, http.StatusOK, events)
}

// ---------------------------------------------------------------------------
// Artifact endpoints (stubs — filled in by Slice 5/8)
// ---------------------------------------------------------------------------

func (h *Handler) getRMAArtifact(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	w.Header().Set("Content-Type", "application/json")
	if err := artifact.GenerateJSON(w, h.Store, id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to generate artifact: %v", err)})
	}
}

func (h *Handler) getRMAPDF(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	rma, err := h.Store.GetRMA(id)
	if err != nil || rma == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "RMA not found"})
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.pdf", rma.RMANumber))
	if err := artifact.GeneratePDF(w, h.Store, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
