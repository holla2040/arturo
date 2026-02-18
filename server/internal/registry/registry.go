package registry

import (
	"sync"
	"time"

	"github.com/holla2040/arturo/internal/protocol"
)

// Status constants for stations and devices.
const (
	StatusOnline  = "online"
	StatusStale   = "stale"
	StatusOffline = "offline"
)

// Health check thresholds.
const (
	StaleThreshold   = 60 * time.Second
	OfflineThreshold = 90 * time.Second
)

// DeviceEntry tracks a single device attached to a station.
type DeviceEntry struct {
	DeviceID        string
	StationInstance string
	CommandStream   string // "commands:{station-instance}"
	Status          string
	LastSeen        time.Time
}

// StationEntry tracks a station's most recent heartbeat data.
type StationEntry struct {
	Instance        string
	LastHeartbeat   time.Time
	Status          string
	Devices         []string
	FreeHeap        int64
	WifiRSSI        int
	FirmwareVersion string
	UptimeSeconds   int64
}

// Registry holds the in-memory map of stations and devices.
type Registry struct {
	mu       sync.RWMutex
	devices  map[string]*DeviceEntry  // deviceID -> entry
	stations map[string]*StationEntry // instance -> entry
}

// New creates an empty registry.
func New() *Registry {
	return &Registry{
		devices:  make(map[string]*DeviceEntry),
		stations: make(map[string]*StationEntry),
	}
}

// UpdateFromHeartbeat upserts a station and reconciles its device list.
func (r *Registry) UpdateFromHeartbeat(instance string, payload *protocol.HeartbeatPayload) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	// Upsert station entry.
	station, exists := r.stations[instance]
	if !exists {
		station = &StationEntry{Instance: instance}
		r.stations[instance] = station
	}
	station.LastHeartbeat = now
	station.Status = StatusOnline
	station.Devices = payload.Devices
	station.FreeHeap = payload.FreeHeap
	station.WifiRSSI = payload.WifiRSSI
	station.FirmwareVersion = payload.FirmwareVersion
	station.UptimeSeconds = payload.UptimeSeconds

	// Build set of new device IDs for quick lookup.
	newDevices := make(map[string]struct{}, len(payload.Devices))
	for _, d := range payload.Devices {
		newDevices[d] = struct{}{}
	}

	// Remove devices that are no longer reported by this station.
	for id, entry := range r.devices {
		if entry.StationInstance == instance {
			if _, ok := newDevices[id]; !ok {
				delete(r.devices, id)
			}
		}
	}

	// Add or update devices from the heartbeat.
	commandStream := "commands:" + instance
	for _, id := range payload.Devices {
		if entry, ok := r.devices[id]; ok {
			entry.Status = StatusOnline
			entry.LastSeen = now
		} else {
			r.devices[id] = &DeviceEntry{
				DeviceID:        id,
				StationInstance: instance,
				CommandStream:   commandStream,
				Status:          StatusOnline,
				LastSeen:        now,
			}
		}
	}
}

// LookupDevice returns a copy of the device entry, or nil if not found.
func (r *Registry) LookupDevice(deviceID string) *DeviceEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.devices[deviceID]
	if !ok {
		return nil
	}
	cp := *entry
	return &cp
}

// ListDevices returns copies of all device entries.
func (r *Registry) ListDevices() []*DeviceEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*DeviceEntry, 0, len(r.devices))
	for _, entry := range r.devices {
		cp := *entry
		result = append(result, &cp)
	}
	return result
}

// ListStations returns copies of all station entries.
func (r *Registry) ListStations() []*StationEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*StationEntry, 0, len(r.stations))
	for _, entry := range r.stations {
		cp := *entry
		result = append(result, &cp)
	}
	return result
}

// RunHealthCheck updates the status of all stations and their devices
// based on elapsed time since the last heartbeat. Pass time.Now() in
// production; tests can pass a fixed time to control thresholds.
func (r *Registry) RunHealthCheck(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, station := range r.stations {
		elapsed := now.Sub(station.LastHeartbeat)

		switch {
		case elapsed >= OfflineThreshold:
			station.Status = StatusOffline
		case elapsed >= StaleThreshold:
			station.Status = StatusStale
		default:
			station.Status = StatusOnline
		}

		// Propagate station status to all its devices.
		for _, deviceID := range station.Devices {
			if dev, ok := r.devices[deviceID]; ok {
				dev.Status = station.Status
				dev.LastSeen = station.LastHeartbeat
			}
		}
	}
}

// SetStationLastHeartbeat is a test helper that overrides a station's
// LastHeartbeat so health check thresholds can be exercised.
func (r *Registry) SetStationLastHeartbeat(instance string, t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if station, ok := r.stations[instance]; ok {
		station.LastHeartbeat = t
	}
}
