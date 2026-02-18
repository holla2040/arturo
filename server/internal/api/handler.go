package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/holla2040/arturo/internal/estop"
	"github.com/holla2040/arturo/internal/protocol"
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

// systemStatus is the response for GET /system/status.
type systemStatus struct {
	StationCount int         `json:"station_count"`
	DeviceCount  int         `json:"device_count"`
	EstopState   estop.State `json:"estop_state"`
}

// Handler holds all dependencies for HTTP request handling.
type Handler struct {
	Registry   *registry.Registry
	Store      *store.Store
	Estop      *estop.Coordinator
	Dispatcher *ResponseDispatcher
	Sender     CommandSender
	Source     protocol.Source
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

func (h *Handler) sendCommand(w http.ResponseWriter, r *http.Request) {
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

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
