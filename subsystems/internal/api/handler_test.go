package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/holla2040/arturo/internal/estop"
	"github.com/holla2040/arturo/internal/protocol"
	"github.com/holla2040/arturo/internal/redishealth"
	"github.com/holla2040/arturo/internal/registry"
	"github.com/holla2040/arturo/internal/store"
)

// mockRedisHealth implements RedisHealthChecker for tests.
type mockRedisHealth struct {
	connected bool
}

func (m *mockRedisHealth) IsConnected() bool {
	return m.connected
}

func (m *mockRedisHealth) GetStatus() redishealth.Status {
	return redishealth.Status{
		Connected:  m.connected,
		Reconnects: 0,
	}
}

// mockSender implements CommandSender for tests.
type mockSender struct {
	mu       sync.Mutex
	sent     []*sentCommand
	sendFunc func(ctx context.Context, stream string, msg *protocol.Message) error
}

type sentCommand struct {
	Stream string
	Msg    *protocol.Message
}

func (m *mockSender) SendCommand(ctx context.Context, stream string, msg *protocol.Message) error {
	m.mu.Lock()
	m.sent = append(m.sent, &sentCommand{Stream: stream, Msg: msg})
	m.mu.Unlock()
	if m.sendFunc != nil {
		return m.sendFunc(ctx, stream, msg)
	}
	return nil
}

func newTestHandler(t *testing.T) (*Handler, *mockSender) {
	t.Helper()

	reg := registry.New()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	e := estop.New(nil)
	d := NewResponseDispatcher()
	sender := &mockSender{}

	h := &Handler{
		Registry:   reg,
		Store:      s,
		Estop:      e,
		Dispatcher: d,
		Sender:     sender,
		Source: protocol.Source{
			Service:  "arturo_controller",
			Instance: "ctrl-test",
			Version:  "1.0.0",
		},
	}
	return h, sender
}

func newTestServer(t *testing.T, h *Handler) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return httptest.NewServer(mux)
}

func seedRegistry(reg *registry.Registry) {
	reg.UpdateFromHeartbeat("station-01", &protocol.HeartbeatPayload{
		Status:          "running",
		Devices:         []string{"fluke-8846a", "relay-board-01"},
		FreeHeap:        200000,
		WifiRSSI:        -45,
		FirmwareVersion: "1.0.0",
		UptimeSeconds:   3600,
	})
}

// --- Dispatcher Tests ---

func TestDispatcherRegisterAndDispatch(t *testing.T) {
	d := NewResponseDispatcher()

	ch := d.Register("corr-123")
	if d.PendingCount() != 1 {
		t.Fatalf("expected 1 pending, got %d", d.PendingCount())
	}

	msg := &protocol.Message{
		Envelope: protocol.Envelope{CorrelationID: "corr-123"},
	}

	ok := d.Dispatch(msg)
	if !ok {
		t.Fatal("expected dispatch to succeed")
	}

	select {
	case got := <-ch:
		if got.Envelope.CorrelationID != "corr-123" {
			t.Errorf("expected corr-123, got %s", got.Envelope.CorrelationID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for response")
	}

	if d.PendingCount() != 0 {
		t.Fatalf("expected 0 pending after dispatch, got %d", d.PendingCount())
	}
}

func TestDispatcherDispatchUnknown(t *testing.T) {
	d := NewResponseDispatcher()
	msg := &protocol.Message{
		Envelope: protocol.Envelope{CorrelationID: "unknown"},
	}
	ok := d.Dispatch(msg)
	if ok {
		t.Fatal("expected dispatch to return false for unknown correlation ID")
	}
}

func TestDispatcherDeregister(t *testing.T) {
	d := NewResponseDispatcher()
	d.Register("corr-456")
	if d.PendingCount() != 1 {
		t.Fatalf("expected 1 pending, got %d", d.PendingCount())
	}

	d.Deregister("corr-456")
	if d.PendingCount() != 0 {
		t.Fatalf("expected 0 pending after deregister, got %d", d.PendingCount())
	}
}

func TestDispatcherDeregisterUnknown(t *testing.T) {
	d := NewResponseDispatcher()
	// Should not panic
	d.Deregister("nonexistent")
}

func TestDispatcherConcurrentAccess(t *testing.T) {
	d := NewResponseDispatcher()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("corr-%d", i)
			ch := d.Register(id)
			msg := &protocol.Message{
				Envelope: protocol.Envelope{CorrelationID: id},
			}
			d.Dispatch(msg)
			<-ch
		}(i)
	}
	wg.Wait()

	if d.PendingCount() != 0 {
		t.Fatalf("expected 0 pending after concurrent test, got %d", d.PendingCount())
	}
}

// --- Handler Tests ---

func TestListDevicesEmpty(t *testing.T) {
	h, _ := newTestHandler(t)
	srv := newTestServer(t, h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/devices")
	if err != nil {
		t.Fatalf("GET /devices failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var devices []interface{}
	json.NewDecoder(resp.Body).Decode(&devices)
	if len(devices) != 0 {
		t.Fatalf("expected 0 devices, got %d", len(devices))
	}
}

func TestListDevicesWithData(t *testing.T) {
	h, _ := newTestHandler(t)
	seedRegistry(h.Registry)
	srv := newTestServer(t, h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/devices")
	if err != nil {
		t.Fatalf("GET /devices failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var devices []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&devices)
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}
}

func TestGetDeviceFound(t *testing.T) {
	h, _ := newTestHandler(t)
	seedRegistry(h.Registry)
	srv := newTestServer(t, h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/devices/fluke-8846a")
	if err != nil {
		t.Fatalf("GET /devices/fluke-8846a failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var device map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&device)
	if device["DeviceID"] != "fluke-8846a" {
		t.Errorf("expected DeviceID fluke-8846a, got %v", device["DeviceID"])
	}
}

func TestGetDeviceNotFound(t *testing.T) {
	h, _ := newTestHandler(t)
	srv := newTestServer(t, h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/devices/nonexistent")
	if err != nil {
		t.Fatalf("GET /devices/nonexistent failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestGetSystemStatus(t *testing.T) {
	h, _ := newTestHandler(t)
	seedRegistry(h.Registry)
	srv := newTestServer(t, h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/system/status")
	if err != nil {
		t.Fatalf("GET /system/status failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var status systemStatus
	json.NewDecoder(resp.Body).Decode(&status)
	if status.StationCount != 1 {
		t.Errorf("expected 1 station, got %d", status.StationCount)
	}
	if status.DeviceCount != 2 {
		t.Errorf("expected 2 devices, got %d", status.DeviceCount)
	}
	if status.EstopState.Active {
		t.Error("expected e-stop inactive")
	}
}

func TestGetSystemStatusWithEstop(t *testing.T) {
	h, _ := newTestHandler(t)
	h.Estop.Trigger("manual", "test stop", "operator")
	srv := newTestServer(t, h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/system/status")
	if err != nil {
		t.Fatalf("GET /system/status failed: %v", err)
	}
	defer resp.Body.Close()

	var status systemStatus
	json.NewDecoder(resp.Body).Decode(&status)
	if !status.EstopState.Active {
		t.Error("expected e-stop active")
	}
}

func TestSendCommandDeviceNotFound(t *testing.T) {
	h, _ := newTestHandler(t)
	srv := newTestServer(t, h)
	defer srv.Close()

	body := `{"command": "measure_dc_voltage"}`
	resp, err := http.Post(srv.URL+"/devices/nonexistent/command", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestSendCommandInvalidBody(t *testing.T) {
	h, _ := newTestHandler(t)
	seedRegistry(h.Registry)
	srv := newTestServer(t, h)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/devices/fluke-8846a/command", "application/json", bytes.NewBufferString("not json"))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestSendCommandEmptyCommand(t *testing.T) {
	h, _ := newTestHandler(t)
	seedRegistry(h.Registry)
	srv := newTestServer(t, h)
	defer srv.Close()

	body := `{"command": ""}`
	resp, err := http.Post(srv.URL+"/devices/fluke-8846a/command", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestSendCommandSuccess(t *testing.T) {
	h, sender := newTestHandler(t)
	seedRegistry(h.Registry)
	srv := newTestServer(t, h)
	defer srv.Close()

	// Mock sender feeds the response back through the dispatcher
	sender.sendFunc = func(ctx context.Context, stream string, msg *protocol.Message) error {
		go func() {
			// Simulate station response
			time.Sleep(10 * time.Millisecond)
			respStr := "1.234"
			durationMs := 150
			respPayload := protocol.CommandResponsePayload{
				DeviceID:    "fluke-8846a",
				CommandName: "measure_dc_voltage",
				Success:     true,
				Response:    &respStr,
				DurationMs:  &durationMs,
			}
			payloadBytes, _ := json.Marshal(respPayload)
			respMsg := &protocol.Message{
				Envelope: protocol.Envelope{
					CorrelationID: msg.Envelope.CorrelationID,
					Type:          protocol.TypeDeviceCommandResponse,
				},
				Payload: json.RawMessage(payloadBytes),
			}
			h.Dispatcher.Dispatch(respMsg)
		}()
		return nil
	}

	body := `{"command": "measure_dc_voltage", "timeout_ms": 2000}`
	resp, err := http.Post(srv.URL+"/devices/fluke-8846a/command", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result protocol.CommandResponsePayload
	json.NewDecoder(resp.Body).Decode(&result)
	if !result.Success {
		t.Error("expected success=true")
	}
	if result.Response == nil || *result.Response != "1.234" {
		t.Errorf("expected response 1.234, got %v", result.Response)
	}
}

func TestSendCommandTimeout(t *testing.T) {
	h, _ := newTestHandler(t)
	seedRegistry(h.Registry)
	srv := newTestServer(t, h)
	defer srv.Close()

	// No response will be dispatched, so it will timeout
	body := `{"command": "measure_dc_voltage", "timeout_ms": 100}`
	resp, err := http.Post(srv.URL+"/devices/fluke-8846a/command", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", resp.StatusCode)
	}
}

func TestSendCommandSenderError(t *testing.T) {
	h, sender := newTestHandler(t)
	seedRegistry(h.Registry)
	srv := newTestServer(t, h)
	defer srv.Close()

	sender.sendFunc = func(ctx context.Context, stream string, msg *protocol.Message) error {
		return fmt.Errorf("redis connection failed")
	}

	body := `{"command": "measure_dc_voltage"}`
	resp, err := http.Post(srv.URL+"/devices/fluke-8846a/command", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

func TestSendCommandDefaultTimeout(t *testing.T) {
	h, sender := newTestHandler(t)
	seedRegistry(h.Registry)
	srv := newTestServer(t, h)
	defer srv.Close()

	// Record what was sent to verify timeout_ms default
	sender.sendFunc = func(ctx context.Context, stream string, msg *protocol.Message) error {
		go func() {
			time.Sleep(10 * time.Millisecond)
			respStr := "ok"
			respPayload := protocol.CommandResponsePayload{
				DeviceID:    "fluke-8846a",
				CommandName: "identify",
				Success:     true,
				Response:    &respStr,
			}
			payloadBytes, _ := json.Marshal(respPayload)
			respMsg := &protocol.Message{
				Envelope: protocol.Envelope{
					CorrelationID: msg.Envelope.CorrelationID,
					Type:          protocol.TypeDeviceCommandResponse,
				},
				Payload: json.RawMessage(payloadBytes),
			}
			h.Dispatcher.Dispatch(respMsg)
		}()
		return nil
	}

	// No timeout_ms in body — should use default 5000
	body := `{"command": "identify"}`
	resp, err := http.Post(srv.URL+"/devices/fluke-8846a/command", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestContentTypeJSON(t *testing.T) {
	h, _ := newTestHandler(t)
	srv := newTestServer(t, h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/devices")
	if err != nil {
		t.Fatalf("GET /devices failed: %v", err)
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}

// --- Redis Health Integration Tests ---

func TestSendCommandRedisUnavailable(t *testing.T) {
	h, _ := newTestHandler(t)
	seedRegistry(h.Registry)
	h.RedisHealth = &mockRedisHealth{connected: false}
	srv := newTestServer(t, h)
	defer srv.Close()

	body := `{"command": "measure_dc_voltage"}`
	resp, err := http.Post(srv.URL+"/devices/fluke-8846a/command", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["error"] != "redis unavailable" {
		t.Errorf("expected error 'redis unavailable', got %q", result["error"])
	}
}

func TestSendCommandRedisAvailable(t *testing.T) {
	h, sender := newTestHandler(t)
	seedRegistry(h.Registry)
	h.RedisHealth = &mockRedisHealth{connected: true}
	srv := newTestServer(t, h)
	defer srv.Close()

	sender.sendFunc = func(ctx context.Context, stream string, msg *protocol.Message) error {
		go func() {
			time.Sleep(10 * time.Millisecond)
			respStr := "ok"
			respPayload := protocol.CommandResponsePayload{
				DeviceID:    "fluke-8846a",
				CommandName: "identify",
				Success:     true,
				Response:    &respStr,
			}
			payloadBytes, _ := json.Marshal(respPayload)
			respMsg := &protocol.Message{
				Envelope: protocol.Envelope{
					CorrelationID: msg.Envelope.CorrelationID,
					Type:          protocol.TypeDeviceCommandResponse,
				},
				Payload: json.RawMessage(payloadBytes),
			}
			h.Dispatcher.Dispatch(respMsg)
		}()
		return nil
	}

	body := `{"command": "identify", "timeout_ms": 2000}`
	resp, err := http.Post(srv.URL+"/devices/fluke-8846a/command", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestSendCommandNoHealthChecker(t *testing.T) {
	// When RedisHealth is nil (not configured), commands should proceed normally
	h, sender := newTestHandler(t)
	seedRegistry(h.Registry)
	// h.RedisHealth is nil by default
	srv := newTestServer(t, h)
	defer srv.Close()

	sender.sendFunc = func(ctx context.Context, stream string, msg *protocol.Message) error {
		go func() {
			time.Sleep(10 * time.Millisecond)
			respStr := "ok"
			respPayload := protocol.CommandResponsePayload{
				DeviceID:    "fluke-8846a",
				CommandName: "identify",
				Success:     true,
				Response:    &respStr,
			}
			payloadBytes, _ := json.Marshal(respPayload)
			respMsg := &protocol.Message{
				Envelope: protocol.Envelope{
					CorrelationID: msg.Envelope.CorrelationID,
					Type:          protocol.TypeDeviceCommandResponse,
				},
				Payload: json.RawMessage(payloadBytes),
			}
			h.Dispatcher.Dispatch(respMsg)
		}()
		return nil
	}

	body := `{"command": "identify", "timeout_ms": 2000}`
	resp, err := http.Post(srv.URL+"/devices/fluke-8846a/command", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestSystemStatusIncludesRedisHealth(t *testing.T) {
	h, _ := newTestHandler(t)
	h.RedisHealth = &mockRedisHealth{connected: true}
	srv := newTestServer(t, h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/system/status")
	if err != nil {
		t.Fatalf("GET /system/status failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	rh, ok := result["redis_health"].(map[string]interface{})
	if !ok {
		t.Fatal("expected redis_health field in response")
	}
	if rh["connected"] != true {
		t.Errorf("expected redis_health.connected=true, got %v", rh["connected"])
	}
}

func TestSystemStatusRedisHealthDisconnected(t *testing.T) {
	h, _ := newTestHandler(t)
	h.RedisHealth = &mockRedisHealth{connected: false}
	srv := newTestServer(t, h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/system/status")
	if err != nil {
		t.Fatalf("GET /system/status failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	rh, ok := result["redis_health"].(map[string]interface{})
	if !ok {
		t.Fatal("expected redis_health field in response")
	}
	if rh["connected"] != false {
		t.Errorf("expected redis_health.connected=false, got %v", rh["connected"])
	}
}

func TestSystemStatusNoRedisHealthChecker(t *testing.T) {
	h, _ := newTestHandler(t)
	// RedisHealth is nil
	srv := newTestServer(t, h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/system/status")
	if err != nil {
		t.Fatalf("GET /system/status failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	// When no health checker, redis_health should be omitted (null)
	if _, ok := result["redis_health"]; ok {
		rh := result["redis_health"]
		if rh != nil {
			t.Errorf("expected redis_health to be omitted or null, got %v", rh)
		}
	}
}

// --- OTA Endpoint Tests ---

func TestOTASuccess(t *testing.T) {
	h, sender := newTestHandler(t)
	seedRegistry(h.Registry)
	srv := newTestServer(t, h)
	defer srv.Close()

	body := `{
		"station": "station-01",
		"firmware_url": "http://192.168.1.10:8080/firmware/v1.1.0.bin",
		"version": "1.1.0",
		"sha256": "a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1",
		"force": false
	}`
	resp, err := http.Post(srv.URL+"/ota", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /ota failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "accepted" {
		t.Errorf("expected status 'accepted', got %q", result["status"])
	}
	if result["station"] != "station-01" {
		t.Errorf("expected station 'station-01', got %q", result["station"])
	}
	if result["version"] != "1.1.0" {
		t.Errorf("expected version '1.1.0', got %q", result["version"])
	}
	if result["correlation_id"] == "" {
		t.Error("expected non-empty correlation_id")
	}

	// Verify that a command was sent to the correct stream
	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 sent command, got %d", len(sender.sent))
	}
	if sender.sent[0].Stream != "commands:station-01" {
		t.Errorf("expected stream 'commands:station-01', got %q", sender.sent[0].Stream)
	}
	if sender.sent[0].Msg.Envelope.Type != protocol.TypeSystemOTARequest {
		t.Errorf("expected type %q, got %q", protocol.TypeSystemOTARequest, sender.sent[0].Msg.Envelope.Type)
	}
}

func TestOTAForced(t *testing.T) {
	h, sender := newTestHandler(t)
	seedRegistry(h.Registry)
	srv := newTestServer(t, h)
	defer srv.Close()

	body := `{
		"station": "station-01",
		"firmware_url": "http://192.168.1.10:8080/firmware/v1.0.0.bin",
		"version": "1.0.0",
		"sha256": "0000000000000000000000000000000000000000000000000000000000000000",
		"force": true
	}`
	resp, err := http.Post(srv.URL+"/ota", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /ota failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	// Verify force flag in payload
	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 sent command, got %d", len(sender.sent))
	}
	p, err := protocol.ParseOTARequest(sender.sent[0].Msg)
	if err != nil {
		t.Fatalf("ParseOTARequest: %v", err)
	}
	if p.Force == nil || *p.Force != true {
		t.Errorf("expected force=true, got %v", p.Force)
	}
}

func TestOTAStationNotFound(t *testing.T) {
	h, _ := newTestHandler(t)
	seedRegistry(h.Registry)
	srv := newTestServer(t, h)
	defer srv.Close()

	body := `{
		"station": "nonexistent-station",
		"firmware_url": "http://192.168.1.10/fw.bin",
		"version": "1.1.0",
		"sha256": "a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1"
	}`
	resp, err := http.Post(srv.URL+"/ota", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /ota failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestOTAMissingStation(t *testing.T) {
	h, _ := newTestHandler(t)
	srv := newTestServer(t, h)
	defer srv.Close()

	body := `{
		"firmware_url": "http://192.168.1.10/fw.bin",
		"version": "1.1.0",
		"sha256": "a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1"
	}`
	resp, err := http.Post(srv.URL+"/ota", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /ota failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestOTAMissingFirmwareURL(t *testing.T) {
	h, _ := newTestHandler(t)
	srv := newTestServer(t, h)
	defer srv.Close()

	body := `{
		"station": "station-01",
		"version": "1.1.0",
		"sha256": "a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1"
	}`
	resp, err := http.Post(srv.URL+"/ota", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /ota failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestOTAMissingVersion(t *testing.T) {
	h, _ := newTestHandler(t)
	srv := newTestServer(t, h)
	defer srv.Close()

	body := `{
		"station": "station-01",
		"firmware_url": "http://192.168.1.10/fw.bin",
		"sha256": "a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1"
	}`
	resp, err := http.Post(srv.URL+"/ota", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /ota failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestOTAMissingSHA256(t *testing.T) {
	h, _ := newTestHandler(t)
	srv := newTestServer(t, h)
	defer srv.Close()

	body := `{
		"station": "station-01",
		"firmware_url": "http://192.168.1.10/fw.bin",
		"version": "1.1.0"
	}`
	resp, err := http.Post(srv.URL+"/ota", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /ota failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestOTAInvalidBody(t *testing.T) {
	h, _ := newTestHandler(t)
	srv := newTestServer(t, h)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/ota", "application/json", bytes.NewBufferString("not json"))
	if err != nil {
		t.Fatalf("POST /ota failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestOTASenderError(t *testing.T) {
	h, sender := newTestHandler(t)
	seedRegistry(h.Registry)
	srv := newTestServer(t, h)
	defer srv.Close()

	sender.sendFunc = func(ctx context.Context, stream string, msg *protocol.Message) error {
		return fmt.Errorf("redis connection failed")
	}

	body := `{
		"station": "station-01",
		"firmware_url": "http://192.168.1.10/fw.bin",
		"version": "1.1.0",
		"sha256": "a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1"
	}`
	resp, err := http.Post(srv.URL+"/ota", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /ota failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

func TestOTARedisUnavailable(t *testing.T) {
	h, _ := newTestHandler(t)
	seedRegistry(h.Registry)
	h.RedisHealth = &mockRedisHealth{connected: false}
	srv := newTestServer(t, h)
	defer srv.Close()

	body := `{
		"station": "station-01",
		"firmware_url": "http://192.168.1.10/fw.bin",
		"version": "1.1.0",
		"sha256": "a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1"
	}`
	resp, err := http.Post(srv.URL+"/ota", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /ota failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}
