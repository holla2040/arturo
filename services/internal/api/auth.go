package api

import (
	"encoding/json"
	"net/http"

	"github.com/holla2040/arturo/internal/store"
)

// loginRequest is the JSON body for POST /auth/login.
type loginRequest struct {
	EmployeeID string `json:"employee_id"`
	Name       string `json:"name"`
}

// handleLogin upserts an employee and returns their info.
func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.EmployeeID == "" || req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "employee_id and name are required"})
		return
	}

	if err := h.Store.UpsertEmployee(req.EmployeeID, req.Name); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create employee"})
		return
	}

	emp, err := h.Store.GetEmployee(req.EmployeeID)
	if err != nil || emp == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get employee"})
		return
	}

	writeJSON(w, http.StatusOK, emp)
}

// handleSession returns info about the current session based on X-Employee-ID header.
func (h *Handler) handleSession(w http.ResponseWriter, r *http.Request) {
	employeeID := r.Header.Get("X-Employee-ID")
	if employeeID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "X-Employee-ID header required"})
		return
	}

	emp, err := h.Store.GetEmployee(employeeID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get employee"})
		return
	}
	if emp == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
		return
	}

	writeJSON(w, http.StatusOK, emp)
}

// getEmployeeID extracts the employee ID from the X-Employee-ID header.
// Returns empty string if not set.
func getEmployeeID(r *http.Request) string {
	return r.Header.Get("X-Employee-ID")
}

// requireEmployee is a middleware helper that checks for X-Employee-ID header.
func requireEmployee(h *Handler, w http.ResponseWriter, r *http.Request) (*store.Employee, bool) {
	employeeID := getEmployeeID(r)
	if employeeID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "X-Employee-ID header required"})
		return nil, false
	}

	emp, err := h.Store.GetEmployee(employeeID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get employee"})
		return nil, false
	}
	if emp == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "employee not found"})
		return nil, false
	}

	return emp, true
}
