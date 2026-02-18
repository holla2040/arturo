package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/holla2040/arturo/internal/estop"
	"github.com/holla2040/arturo/internal/protocol"
	"github.com/holla2040/arturo/internal/redishealth"
	"github.com/holla2040/arturo/internal/registry"
	"github.com/holla2040/arturo/internal/report"
	"github.com/holla2040/arturo/internal/store"
)

// CommandSender abstracts the Redis XADD operation for testability.
type CommandSender interface {
	SendCommand(ctx context.Context, stream string, msg *protocol.Message) error
}

// commandRequest is the JSON body for POST /devices/{id}/command.
type commandRequest struct {
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
	RedisHealth RedisHealthChecker // nil means no health checking
}

// RegisterRoutes adds all API routes to the given ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
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
	if !h.redisAvailable() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "redis unavailable"})
		return
	}

	deviceID := r.PathValue("id")

	device := h.Registry.LookupDevice(deviceID)
	if device == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "device not found"})
		return
	}

	var req commandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Command == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "command is required"})
		return
	}

	timeoutMs := req.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}

	msg, err := protocol.BuildCommandRequest(h.Source, deviceID, req.Command, req.Parameters, timeoutMs)
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

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
