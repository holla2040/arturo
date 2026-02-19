package mockpump

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNewPump(t *testing.T) {
	p := NewPump(4.0, 0.0)
	if p.state != StateOff {
		t.Errorf("expected StateOff, got %d", p.state)
	}
	if p.firstStageK != 295.0 {
		t.Errorf("expected 295.0K 1st stage, got %.1f", p.firstStageK)
	}
	if p.secondStageK != 295.0 {
		t.Errorf("expected 295.0K 2nd stage, got %.1f", p.secondStageK)
	}
}

func TestPumpOnOff(t *testing.T) {
	p := NewPump(4.0, 0.0)

	// Initially off
	resp, ok := p.HandleCommand("pump_status")
	if !ok || resp != "0" {
		t.Errorf("expected pump off (0), got %s ok=%v", resp, ok)
	}

	// Turn on
	resp, ok = p.HandleCommand("pump_on")
	if !ok || resp != "A" {
		t.Errorf("expected A, got %s ok=%v", resp, ok)
	}

	// Should be on now
	resp, ok = p.HandleCommand("pump_status")
	if !ok || resp != "1" {
		t.Errorf("expected pump on (1), got %s ok=%v", resp, ok)
	}

	// Turn off
	resp, ok = p.HandleCommand("pump_off")
	if !ok || resp != "A" {
		t.Errorf("expected A, got %s ok=%v", resp, ok)
	}

	resp, ok = p.HandleCommand("pump_status")
	if !ok || resp != "0" {
		t.Errorf("expected pump off (0), got %s ok=%v", resp, ok)
	}
}

func TestTemperatureQueries(t *testing.T) {
	p := NewPump(4.0, 0.0)

	resp, ok := p.HandleCommand("get_temp_1st_stage")
	if !ok {
		t.Fatal("expected success for get_temp_1st_stage")
	}
	temp, err := strconv.ParseFloat(resp, 64)
	if err != nil {
		t.Fatalf("expected float, got %s: %v", resp, err)
	}
	if temp < 290.0 || temp > 300.0 {
		t.Errorf("expected room temp ~295K, got %.1f", temp)
	}

	resp, ok = p.HandleCommand("get_temp_2nd_stage")
	if !ok {
		t.Fatal("expected success for get_temp_2nd_stage")
	}
	temp, err = strconv.ParseFloat(resp, 64)
	if err != nil {
		t.Fatalf("expected float, got %s: %v", resp, err)
	}
	if temp < 290.0 || temp > 300.0 {
		t.Errorf("expected room temp ~295K, got %.1f", temp)
	}
}

func TestPressureQuery(t *testing.T) {
	p := NewPump(4.0, 0.0)

	resp, ok := p.HandleCommand("get_pump_tc_pressure")
	if !ok {
		t.Fatal("expected success")
	}
	pressure, err := strconv.ParseFloat(resp, 64)
	if err != nil {
		t.Fatalf("expected float, got %s: %v", resp, err)
	}
	// At room temp, pressure should be ~1e-3
	if pressure < 1e-4 || pressure > 1e-2 {
		t.Errorf("expected high pressure at room temp, got %e", pressure)
	}
}

func TestOperatingHours(t *testing.T) {
	p := NewPump(4.0, 0.0)

	resp, ok := p.HandleCommand("get_operating_hours")
	if !ok {
		t.Fatal("expected success")
	}
	hours, err := strconv.ParseFloat(resp, 64)
	if err != nil {
		t.Fatalf("expected float, got %s: %v", resp, err)
	}
	if hours > 0.1 {
		t.Errorf("expected ~0 hours initially, got %.1f", hours)
	}
}

func TestStatusBytes(t *testing.T) {
	p := NewPump(4.0, 0.0)

	// Off: status1 should be 0
	resp, ok := p.HandleCommand("get_status_1")
	if !ok || resp != "0" {
		t.Errorf("expected status1=0 when off, got %s", resp)
	}

	// Turn on: bit 0 should be set
	p.HandleCommand("pump_on")
	resp, ok = p.HandleCommand("get_status_1")
	if !ok {
		t.Fatal("expected success")
	}
	status, _ := strconv.Atoi(resp)
	if status&1 == 0 {
		t.Errorf("expected bit 0 set when on, got %d", status)
	}

	resp, ok = p.HandleCommand("get_status_2")
	if !ok || resp != "0" {
		t.Errorf("expected status2=0, got %s", resp)
	}

	resp, ok = p.HandleCommand("get_status_3")
	if !ok || resp != "0" {
		t.Errorf("expected status3=0, got %s", resp)
	}
}

func TestIdentify(t *testing.T) {
	p := NewPump(4.0, 0.0)

	resp, ok := p.HandleCommand("identify")
	if !ok {
		t.Fatal("expected success")
	}
	if !strings.Contains(resp, "CTI-Cryogenics") {
		t.Errorf("expected CTI-Cryogenics in identify, got %s", resp)
	}
	if !strings.Contains(resp, "SIM-001") {
		t.Errorf("expected SIM-001 in identify, got %s", resp)
	}
}

func TestUnknownCommand(t *testing.T) {
	p := NewPump(4.0, 0.0)

	resp, ok := p.HandleCommand("bogus_command")
	if ok {
		t.Error("expected failure for unknown command")
	}
	if resp != "?" {
		t.Errorf("expected '?', got %s", resp)
	}
}

func TestRegenCycle(t *testing.T) {
	p := NewPump(4.0, 0.0)

	// Can't start regen when off
	resp, ok := p.HandleCommand("start_regen")
	if ok {
		t.Error("expected failure starting regen when off")
	}
	if resp != "N" {
		t.Errorf("expected N, got %s", resp)
	}

	// Turn on, then start regen
	p.HandleCommand("pump_on")
	resp, ok = p.HandleCommand("start_regen")
	if !ok || resp != "A" {
		t.Errorf("expected A, got %s ok=%v", resp, ok)
	}

	// Should be in regen step 1
	resp, ok = p.HandleCommand("get_regen_step")
	if !ok || resp != "1" {
		t.Errorf("expected regen step 1, got %s", resp)
	}

	// Abort regen
	resp, ok = p.HandleCommand("abort_regen")
	if !ok || resp != "A" {
		t.Errorf("expected A for abort, got %s ok=%v", resp, ok)
	}

	// Regen step should be 0
	resp, ok = p.HandleCommand("get_regen_step")
	if !ok || resp != "0" {
		t.Errorf("expected regen step 0 after abort, got %s", resp)
	}
}

func TestCoolingStateTransition(t *testing.T) {
	p := NewPump(4.0, 0.0)

	p.HandleCommand("pump_on")
	if p.state != StateCooling {
		t.Errorf("expected StateCooling after pump_on, got %d", p.state)
	}

	// Simulate passage of time by manipulating lastUpdate
	p.mu.Lock()
	p.lastUpdate = time.Now().Add(-24 * time.Hour)
	p.mu.Unlock()

	// Next command triggers temperature update
	p.HandleCommand("pump_status")

	p.mu.RLock()
	state := p.state
	first := p.firstStageK
	second := p.secondStageK
	p.mu.RUnlock()

	// After 24 hours the pump should have reached cold state
	if state != StateCold {
		t.Errorf("expected StateCold after 24h, got %d (1st=%.1fK 2nd=%.1fK)", state, first, second)
	}
}

func TestExponentialDecay(t *testing.T) {
	// Test the math helper
	result := exponentialDecay(295.0, 65.0, 3600.0, 3600.0)
	// After one time constant, should be at target + (start-target)*e^(-1) ≈ 65 + 230*0.368 ≈ 149.6
	expected := 65.0 + (295.0-65.0)*0.367879441
	if diff := result - expected; diff > 1.0 || diff < -1.0 {
		t.Errorf("expected ~%.1f, got %.1f", expected, result)
	}
}

func TestDriftToward(t *testing.T) {
	// Should move toward target
	result := driftToward(100.0, 295.0, 10.0, 0.01)
	if result <= 100.0 {
		t.Errorf("expected drift upward, got %.1f", result)
	}
	if result > 295.0 {
		t.Errorf("should not overshoot target, got %.1f", result)
	}

	// Large step should clamp to target
	result = driftToward(294.0, 295.0, 1000.0, 1.0)
	if result != 295.0 {
		t.Errorf("expected clamped to 295.0, got %.1f", result)
	}
}

func TestFailRate(t *testing.T) {
	// With 100% fail rate, all commands should fail
	p := NewPump(4.0, 1.0)

	_, ok := p.HandleCommand("pump_status")
	if ok {
		t.Error("expected failure with 100% fail rate")
	}
}

func TestDuplicatePumpOn(t *testing.T) {
	p := NewPump(4.0, 0.0)

	p.HandleCommand("pump_on")
	if p.state != StateCooling {
		t.Fatal("expected cooling state")
	}

	// Second pump_on should be idempotent
	resp, ok := p.HandleCommand("pump_on")
	if !ok || resp != "A" {
		t.Errorf("expected A for duplicate pump_on, got %s ok=%v", resp, ok)
	}
}
