package registry

import (
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/holla2040/arturo/internal/protocol"
)

func makePayload(devices []string) *protocol.HeartbeatPayload {
	return &protocol.HeartbeatPayload{
		Status:          "ok",
		UptimeSeconds:   3600,
		Devices:         devices,
		FreeHeap:        200000,
		WifiRSSI:        -55,
		FirmwareVersion: "1.0.0",
	}
}

func TestNewRegistryIsEmpty(t *testing.T) {
	r := New()
	if got := r.ListDevices(); len(got) != 0 {
		t.Fatalf("expected 0 devices, got %d", len(got))
	}
	if got := r.ListStations(); len(got) != 0 {
		t.Fatalf("expected 0 stations, got %d", len(got))
	}
}

func TestUpdateFromHeartbeatAddsStationAndDevices(t *testing.T) {
	r := New()
	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1", "psu-1"}))

	stations := r.ListStations()
	if len(stations) != 1 {
		t.Fatalf("expected 1 station, got %d", len(stations))
	}
	s := stations[0]
	if s.Instance != "station-1" {
		t.Errorf("expected instance station-1, got %s", s.Instance)
	}
	if s.Status != StatusOnline {
		t.Errorf("expected status online, got %s", s.Status)
	}
	if s.FreeHeap != 200000 {
		t.Errorf("expected FreeHeap 200000, got %d", s.FreeHeap)
	}
	if s.WifiRSSI != -55 {
		t.Errorf("expected WifiRSSI -55, got %d", s.WifiRSSI)
	}
	if s.FirmwareVersion != "1.0.0" {
		t.Errorf("expected FirmwareVersion 1.0.0, got %s", s.FirmwareVersion)
	}
	if s.UptimeSeconds != 3600 {
		t.Errorf("expected UptimeSeconds 3600, got %d", s.UptimeSeconds)
	}

	devices := r.ListDevices()
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
}

func TestUpdateFromHeartbeatUpdatesExistingStation(t *testing.T) {
	r := New()
	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1"}))

	updated := &protocol.HeartbeatPayload{
		Status:          "ok",
		UptimeSeconds:   7200,
		Devices:         []string{"dmm-1"},
		FreeHeap:        150000,
		WifiRSSI:        -60,
		FirmwareVersion: "1.1.0",
	}
	r.UpdateFromHeartbeat("station-1", updated)

	stations := r.ListStations()
	if len(stations) != 1 {
		t.Fatalf("expected 1 station, got %d", len(stations))
	}
	s := stations[0]
	if s.FreeHeap != 150000 {
		t.Errorf("expected FreeHeap 150000, got %d", s.FreeHeap)
	}
	if s.FirmwareVersion != "1.1.0" {
		t.Errorf("expected FirmwareVersion 1.1.0, got %s", s.FirmwareVersion)
	}
	if s.UptimeSeconds != 7200 {
		t.Errorf("expected UptimeSeconds 7200, got %d", s.UptimeSeconds)
	}
}

func TestDeviceReconciliationAddsNewDevices(t *testing.T) {
	r := New()
	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1"}))

	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1", "psu-1"}))

	devices := r.ListDevices()
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
}

func TestDeviceReconciliationRemovesOldDevices(t *testing.T) {
	r := New()
	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1", "psu-1"}))

	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1"}))

	devices := r.ListDevices()
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].DeviceID != "dmm-1" {
		t.Errorf("expected dmm-1 to remain, got %s", devices[0].DeviceID)
	}
}

func TestDeviceReconciliationMixedAddRemove(t *testing.T) {
	r := New()
	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1", "psu-1"}))

	r.UpdateFromHeartbeat("station-1", makePayload([]string{"psu-1", "relay-1"}))

	devices := r.ListDevices()
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	ids := make(map[string]bool)
	for _, d := range devices {
		ids[d.DeviceID] = true
	}
	if !ids["psu-1"] {
		t.Error("expected psu-1 to remain")
	}
	if !ids["relay-1"] {
		t.Error("expected relay-1 to be added")
	}
	if ids["dmm-1"] {
		t.Error("expected dmm-1 to be removed")
	}
}

func TestLookupDeviceReturnsCorrectEntry(t *testing.T) {
	r := New()
	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1", "psu-1"}))

	entry := r.LookupDevice("dmm-1")
	if entry == nil {
		t.Fatal("expected entry, got nil")
	}
	if entry.DeviceID != "dmm-1" {
		t.Errorf("expected DeviceID dmm-1, got %s", entry.DeviceID)
	}
	if entry.StationInstance != "station-1" {
		t.Errorf("expected StationInstance station-1, got %s", entry.StationInstance)
	}
	if entry.CommandStream != "commands:station-1" {
		t.Errorf("expected CommandStream commands:station-1, got %s", entry.CommandStream)
	}
	if entry.Status != StatusOnline {
		t.Errorf("expected status online, got %s", entry.Status)
	}
}

func TestLookupDeviceReturnsNilForUnknown(t *testing.T) {
	r := New()
	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1"}))

	if entry := r.LookupDevice("nonexistent"); entry != nil {
		t.Errorf("expected nil for unknown device, got %+v", entry)
	}
}

func TestLookupDeviceReturnsCopy(t *testing.T) {
	r := New()
	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1"}))

	entry := r.LookupDevice("dmm-1")
	entry.Status = "mutated"

	original := r.LookupDevice("dmm-1")
	if original.Status == "mutated" {
		t.Error("LookupDevice should return a copy, not a reference to internal state")
	}
}

func TestListDevicesReturnsAll(t *testing.T) {
	r := New()
	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1", "psu-1"}))
	r.UpdateFromHeartbeat("station-2", makePayload([]string{"relay-1"}))

	devices := r.ListDevices()
	if len(devices) != 3 {
		t.Fatalf("expected 3 devices, got %d", len(devices))
	}
}

func TestListStationsReturnsAll(t *testing.T) {
	r := New()
	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1"}))
	r.UpdateFromHeartbeat("station-2", makePayload([]string{"psu-1"}))

	stations := r.ListStations()
	if len(stations) != 2 {
		t.Fatalf("expected 2 stations, got %d", len(stations))
	}
}

func TestHealthCheckOnlineStaysOnline(t *testing.T) {
	r := New()
	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1"}))

	r.RunHealthCheck(time.Now())

	s := r.ListStations()[0]
	if s.Status != StatusOnline {
		t.Errorf("expected online, got %s", s.Status)
	}
	d := r.LookupDevice("dmm-1")
	if d.Status != StatusOnline {
		t.Errorf("expected device online, got %s", d.Status)
	}
}

func TestHealthCheckBecomesStale(t *testing.T) {
	r := New()
	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1"}))

	past := time.Now().Add(-StaleThreshold - time.Second)
	r.SetStationLastHeartbeat("station-1", past)

	r.RunHealthCheck(time.Now())

	s := r.ListStations()[0]
	if s.Status != StatusStale {
		t.Errorf("expected stale, got %s", s.Status)
	}
	d := r.LookupDevice("dmm-1")
	if d.Status != StatusStale {
		t.Errorf("expected device stale, got %s", d.Status)
	}
}

func TestHealthCheckBecomesOffline(t *testing.T) {
	r := New()
	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1"}))

	past := time.Now().Add(-OfflineThreshold - time.Second)
	r.SetStationLastHeartbeat("station-1", past)

	r.RunHealthCheck(time.Now())

	s := r.ListStations()[0]
	if s.Status != StatusOffline {
		t.Errorf("expected offline, got %s", s.Status)
	}
	d := r.LookupDevice("dmm-1")
	if d.Status != StatusOffline {
		t.Errorf("expected device offline, got %s", d.Status)
	}
}

func TestHealthCheckMultipleStationsIndependent(t *testing.T) {
	r := New()
	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1"}))
	r.UpdateFromHeartbeat("station-2", makePayload([]string{"psu-1"}))

	// Make station-1 stale, station-2 stays recent.
	past := time.Now().Add(-StaleThreshold - time.Second)
	r.SetStationLastHeartbeat("station-1", past)

	r.RunHealthCheck(time.Now())

	stations := r.ListStations()
	sort.Slice(stations, func(i, j int) bool {
		return stations[i].Instance < stations[j].Instance
	})

	if stations[0].Status != StatusStale {
		t.Errorf("station-1: expected stale, got %s", stations[0].Status)
	}
	if stations[1].Status != StatusOnline {
		t.Errorf("station-2: expected online, got %s", stations[1].Status)
	}

	d1 := r.LookupDevice("dmm-1")
	if d1.Status != StatusStale {
		t.Errorf("dmm-1: expected stale, got %s", d1.Status)
	}
	d2 := r.LookupDevice("psu-1")
	if d2.Status != StatusOnline {
		t.Errorf("psu-1: expected online, got %s", d2.Status)
	}
}

func TestMultipleStationsDontInterfere(t *testing.T) {
	r := New()
	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1", "psu-1"}))
	r.UpdateFromHeartbeat("station-2", makePayload([]string{"relay-1"}))

	// Remove a device from station-1 only.
	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1"}))

	// station-2's device should be unaffected.
	relay := r.LookupDevice("relay-1")
	if relay == nil {
		t.Fatal("relay-1 should still exist")
	}
	if relay.StationInstance != "station-2" {
		t.Errorf("relay-1 should belong to station-2, got %s", relay.StationInstance)
	}

	// psu-1 should be removed.
	psu := r.LookupDevice("psu-1")
	if psu != nil {
		t.Error("psu-1 should have been removed from station-1")
	}

	// dmm-1 should still exist.
	dmm := r.LookupDevice("dmm-1")
	if dmm == nil {
		t.Fatal("dmm-1 should still exist")
	}
}

func TestConcurrentUpdateAndLookup(t *testing.T) {
	r := New()
	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1"}))

	var wg sync.WaitGroup
	const iterations = 100

	// Concurrent updates.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1", "psu-1"}))
		}
	}()

	// Concurrent lookups.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = r.LookupDevice("dmm-1")
		}
	}()

	// Concurrent list operations.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = r.ListDevices()
			_ = r.ListStations()
		}
	}()

	// Concurrent health checks.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			r.RunHealthCheck(time.Now())
		}
	}()

	wg.Wait()

	// If we get here without a race detector panic, concurrency is safe.
	entry := r.LookupDevice("dmm-1")
	if entry == nil {
		t.Fatal("dmm-1 should exist after concurrent operations")
	}
}

func TestSetStationLastHeartbeatNoOpForUnknown(t *testing.T) {
	r := New()
	// Should not panic on unknown station.
	r.SetStationLastHeartbeat("nonexistent", time.Now())
}

func TestDeviceCommandStream(t *testing.T) {
	r := New()
	r.UpdateFromHeartbeat("station-3", makePayload([]string{"cryo-1"}))

	d := r.LookupDevice("cryo-1")
	if d == nil {
		t.Fatal("expected device entry")
	}
	expected := "commands:station-3"
	if d.CommandStream != expected {
		t.Errorf("expected CommandStream %q, got %q", expected, d.CommandStream)
	}
}

func TestHealthCheckRestoredAfterNewHeartbeat(t *testing.T) {
	r := New()
	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1"}))

	// Make it stale.
	past := time.Now().Add(-StaleThreshold - time.Second)
	r.SetStationLastHeartbeat("station-1", past)
	r.RunHealthCheck(time.Now())

	s := r.ListStations()[0]
	if s.Status != StatusStale {
		t.Fatalf("expected stale, got %s", s.Status)
	}

	// New heartbeat should restore to online.
	r.UpdateFromHeartbeat("station-1", makePayload([]string{"dmm-1"}))

	s = r.ListStations()[0]
	if s.Status != StatusOnline {
		t.Errorf("expected online after new heartbeat, got %s", s.Status)
	}
	d := r.LookupDevice("dmm-1")
	if d.Status != StatusOnline {
		t.Errorf("expected device online after new heartbeat, got %s", d.Status)
	}
}
