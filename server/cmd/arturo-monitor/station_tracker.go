package main

import (
	"sort"
	"sync"
	"time"
)

// StationState represents the connectivity state of a station.
type StationState int

const (
	StateOnline  StationState = iota
	StateStale
	StateOffline
)

func (s StationState) String() string {
	switch s {
	case StateOnline:
		return "ONLINE"
	case StateStale:
		return "STALE"
	case StateOffline:
		return "OFFLINE"
	default:
		return "UNKNOWN"
	}
}

// StationTracker tracks station heartbeat times for state detection.
type StationTracker struct {
	mu            sync.RWMutex
	LastHeartbeat map[string]time.Time
}

// NewStationTracker creates a new StationTracker.
func NewStationTracker() *StationTracker {
	return &StationTracker{
		LastHeartbeat: make(map[string]time.Time),
	}
}

// RecordHeartbeat records the current time as the last heartbeat for an instance.
func (t *StationTracker) RecordHeartbeat(instance string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.LastHeartbeat[instance] = time.Now()
}

// GetState returns the station state and last-seen time based on TTL and heartbeat history.
// A TTL <= 0 means OFFLINE. No heartbeat within staleThreshold means STALE.
func (t *StationTracker) GetState(instance string, ttl time.Duration) (StationState, time.Time) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	lastSeen := t.LastHeartbeat[instance]

	if ttl <= 0 {
		return StateOffline, lastSeen
	}

	if !lastSeen.IsZero() && time.Since(lastSeen) > 60*time.Second {
		return StateStale, lastSeen
	}

	return StateOnline, lastSeen
}

// KnownInstances returns a sorted list of all instances that have ever sent a heartbeat.
func (t *StationTracker) KnownInstances() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	instances := make([]string, 0, len(t.LastHeartbeat))
	for inst := range t.LastHeartbeat {
		instances = append(instances, inst)
	}
	sort.Strings(instances)
	return instances
}
