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
	if p.regenError != '@' {
		t.Errorf("expected regenError '@', got %c", p.regenError)
	}
}

func TestPumpOnOff(t *testing.T) {
	p := NewPump(4.0, 0.0)

	resp, ok := p.HandleCommand("pump_status")
	if !ok || resp != "0" {
		t.Errorf("expected pump off (0), got %s ok=%v", resp, ok)
	}

	resp, ok = p.HandleCommand("pump_on")
	if !ok || resp != "A" {
		t.Errorf("expected A, got %s ok=%v", resp, ok)
	}

	resp, ok = p.HandleCommand("pump_status")
	if !ok || resp != "1" {
		t.Errorf("expected pump on (1), got %s ok=%v", resp, ok)
	}

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

	// Off: S1 should only have power-fail bit (0x20 = 32)
	resp, ok := p.HandleCommand("get_status_1")
	if !ok {
		t.Fatal("expected success")
	}
	status, _ := strconv.Atoi(resp)
	if status != 0x20 {
		t.Errorf("expected S1=0x20 (32) when off, got %d (0x%02x)", status, status)
	}

	// Turn on: bit 0 (pump on) + bit 3 (cryo TC) + bit 5 (power)
	p.HandleCommand("pump_on")
	resp, ok = p.HandleCommand("get_status_1")
	if !ok {
		t.Fatal("expected success")
	}
	status, _ = strconv.Atoi(resp)
	if status&0x01 == 0 {
		t.Errorf("expected bit 0 (pump ON) set, got %d", status)
	}
	if status&0x08 == 0 {
		t.Errorf("expected bit 3 (cryo TC) set, got %d", status)
	}
	if status&0x20 == 0 {
		t.Errorf("expected bit 5 (power) set, got %d", status)
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

func TestRegenFromOff(t *testing.T) {
	p := NewPump(4.0, 0.0)

	// Should be able to start regen from off
	resp, ok := p.HandleCommand("start_regen")
	if !ok || resp != "A" {
		t.Errorf("expected A, got %s ok=%v", resp, ok)
	}

	snap := p.Snapshot()
	if snap.State != StateRegen {
		t.Errorf("expected StateRegen, got %d (%s)", snap.State, snap.StateName)
	}
	if snap.RegenStep != int(RegenPhaseWarming) {
		t.Errorf("expected phase 1 (warming), got %d", snap.RegenStep)
	}
}

func TestRegenCycle(t *testing.T) {
	p := NewPump(4.0, 0.0)

	// Put pump in cold state
	p.SetState(StateCooling)
	p.mu.Lock()
	p.firstStageK = 65.0
	p.secondStageK = 15.0
	p.state = StateCold
	p.mu.Unlock()

	// Start regen
	resp, ok := p.HandleCommand("start_regen")
	if !ok || resp != "A" {
		t.Errorf("expected A, got %s ok=%v", resp, ok)
	}

	// Should be in regen phase 1 (warming)
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

func TestRegenPhaseWarming(t *testing.T) {
	p := NewPump(4.0, 0.0)
	setColdState(p)

	p.HandleCommand("start_regen")

	// Verify phase
	snap := p.Snapshot()
	if snap.RegenStep != int(RegenPhaseWarming) {
		t.Errorf("expected phase 1 (warming), got %d", snap.RegenStep)
	}
	if snap.RegenPhaseName != "warming" {
		t.Errorf("expected phase name 'warming', got %q", snap.RegenPhaseName)
	}

	// O-char should be 'B'
	resp, _ := p.HandleCommand("get_regen_status")
	if resp != "B" {
		t.Errorf("expected O-char 'B' for warming, got %q", resp)
	}

	// Heaters on, purge open, rough closed
	if !snap.HeatersOn {
		t.Error("expected heaters on during warming")
	}
	if !snap.PurgeValveOpen {
		t.Error("expected purge valve open during warming")
	}
	if snap.RoughValveOpen {
		t.Error("expected rough valve closed during warming")
	}
}

func TestRegenPhasePurge(t *testing.T) {
	p := NewPump(4.0, 0.0)
	setColdState(p)
	p.HandleCommand("start_regen")

	// Advance to purge
	p.AdvanceRegenStep()

	snap := p.Snapshot()
	if snap.RegenStep != int(RegenPhasePurge) {
		t.Errorf("expected phase 2 (purge), got %d", snap.RegenStep)
	}
	if snap.RegenPhaseName != "extended purge" {
		t.Errorf("expected phase name 'extended purge', got %q", snap.RegenPhaseName)
	}

	// O-char should be 'H'
	resp, _ := p.HandleCommand("get_regen_status")
	if resp != "H" {
		t.Errorf("expected O-char 'H' for purge, got %q", resp)
	}

	// Heaters on, purge open, rough closed
	if !snap.HeatersOn {
		t.Error("expected heaters on during purge")
	}
	if !snap.PurgeValveOpen {
		t.Error("expected purge valve open during purge")
	}
	if snap.RoughValveOpen {
		t.Error("expected rough valve closed during purge")
	}
}

func TestRegenPhaseRoughing(t *testing.T) {
	p := NewPump(4.0, 0.0)
	setColdState(p)
	p.HandleCommand("start_regen")

	// Advance to purge, then roughing
	p.AdvanceRegenStep()
	p.AdvanceRegenStep()

	snap := p.Snapshot()
	if snap.RegenStep != int(RegenPhaseRoughing) {
		t.Errorf("expected phase 3 (roughing), got %d", snap.RegenStep)
	}

	// O-char should be 'I'
	resp, _ := p.HandleCommand("get_regen_status")
	if resp != "I" {
		t.Errorf("expected O-char 'I' for roughing, got %q", resp)
	}

	// Purge closed, rough open
	if snap.PurgeValveOpen {
		t.Error("expected purge valve closed during roughing")
	}
	if !snap.RoughValveOpen {
		t.Error("expected rough valve open during roughing")
	}
	if !snap.HeatersOn {
		t.Error("expected heaters on during roughing")
	}
}

func TestRegenPhaseROR(t *testing.T) {
	p := NewPump(4.0, 0.0)
	setColdState(p)
	p.HandleCommand("start_regen")

	// Advance to ROR
	p.AdvanceRegenStep() // -> purge
	p.AdvanceRegenStep() // -> roughing
	p.AdvanceRegenStep() // -> ROR

	snap := p.Snapshot()
	if snap.RegenStep != int(RegenPhaseROR) {
		t.Errorf("expected phase 4 (ROR), got %d", snap.RegenStep)
	}

	// O-char should be 'L'
	resp, _ := p.HandleCommand("get_regen_status")
	if resp != "L" {
		t.Errorf("expected O-char 'L' for ROR, got %q", resp)
	}

	// Both valves closed
	if snap.RoughValveOpen {
		t.Error("expected rough valve closed during ROR")
	}
	if snap.PurgeValveOpen {
		t.Error("expected purge valve closed during ROR")
	}
}

func TestRegenPhaseCooling(t *testing.T) {
	p := NewPump(4.0, 0.0)
	setColdState(p)
	p.HandleCommand("start_regen")

	// Advance to cooling
	p.AdvanceRegenStep() // -> purge
	p.AdvanceRegenStep() // -> roughing
	p.AdvanceRegenStep() // -> ROR
	p.AdvanceRegenStep() // -> cooling

	snap := p.Snapshot()
	if snap.RegenStep != int(RegenPhaseCooling) {
		t.Errorf("expected phase 5 (cooling), got %d", snap.RegenStep)
	}

	// O-char should be 'M'
	resp, _ := p.HandleCommand("get_regen_status")
	if resp != "M" {
		t.Errorf("expected O-char 'M' for cooling, got %q", resp)
	}

	// Heaters off, all valves closed
	if snap.HeatersOn {
		t.Error("expected heaters off during cooling")
	}
	if snap.RoughValveOpen {
		t.Error("expected rough valve closed during cooling")
	}
	if snap.PurgeValveOpen {
		t.Error("expected purge valve closed during cooling")
	}
}

func TestRegenRORRetry(t *testing.T) {
	p := NewPump(4.0, 0.0)

	// Use very low ROR limit so the simulated 10 mTorr/min rise always fails
	params := DefaultRegenParams()
	params.RORLimitMTorrMin = 0.001 // impossibly low
	params.MaxRORRetries = 2
	p.SetRegenParams(params)

	setColdState(p)
	p.HandleCommand("start_regen")

	// Advance to ROR
	p.AdvanceRegenStep() // -> purge
	p.AdvanceRegenStep() // -> roughing
	p.AdvanceRegenStep() // -> ROR

	// Simulate 2 minutes passing so ROR evaluates
	p.mu.Lock()
	p.rorStartTime = time.Now().Add(-2 * time.Minute)
	p.regenPhaseStart = time.Now().Add(-2 * time.Minute)
	// Add pressure rise to simulate failed ROR
	p.regenPressure = p.rorStartPressure + 0.100 // 100 mTorr rise
	p.mu.Unlock()

	// Trigger update
	p.Snapshot()

	// Should have retried - back to purge
	p.mu.RLock()
	phase := p.regenPhase
	retries := p.regenRetryCount
	p.mu.RUnlock()

	if phase != RegenPhasePurge {
		t.Errorf("expected retry back to purge, got phase %d", phase)
	}
	if retries != 1 {
		t.Errorf("expected retry count 1, got %d", retries)
	}

	// Do another failed ROR cycle
	p.AdvanceRegenStep() // -> roughing
	p.AdvanceRegenStep() // -> ROR

	p.mu.Lock()
	p.rorStartTime = time.Now().Add(-2 * time.Minute)
	p.regenPhaseStart = time.Now().Add(-2 * time.Minute)
	p.regenPressure = p.rorStartPressure + 0.100
	p.mu.Unlock()

	p.Snapshot()

	// Should be aborted after max retries
	p.mu.RLock()
	state := p.state
	errCode := p.regenError
	p.mu.RUnlock()

	if state != StateOff {
		t.Errorf("expected StateOff after ROR exhaustion, got %d", state)
	}
	if errCode != 'E' {
		t.Errorf("expected error 'E' for ROR limit, got %c", errCode)
	}
}

func TestRegenAbortManual(t *testing.T) {
	p := NewPump(4.0, 0.0)
	setColdState(p)
	p.HandleCommand("start_regen")

	resp, ok := p.HandleCommand("abort_regen")
	if !ok || resp != "A" {
		t.Errorf("expected A, got %s ok=%v", resp, ok)
	}

	// Check error code 'F' and O-char 'V'
	resp, _ = p.HandleCommand("get_regen_error")
	if resp != "F" {
		t.Errorf("expected error 'F' for manual abort, got %q", resp)
	}

	resp, _ = p.HandleCommand("get_regen_status")
	if resp != "V" {
		t.Errorf("expected O-char 'V' for aborted, got %q", resp)
	}
}

func TestRegenAbortWarmupTimeout(t *testing.T) {
	p := NewPump(4.0, 0.0)

	// Very short warmup timeout
	params := DefaultRegenParams()
	params.WarmupTimeout = 1 * time.Millisecond
	p.SetRegenParams(params)

	setColdState(p)
	p.HandleCommand("start_regen")

	// Wait for timeout
	time.Sleep(5 * time.Millisecond)

	// Trigger update
	p.Snapshot()

	p.mu.RLock()
	state := p.state
	errCode := p.regenError
	p.mu.RUnlock()

	if state != StateOff {
		t.Errorf("expected StateOff after warmup timeout, got %d", state)
	}
	if errCode != 'B' {
		t.Errorf("expected error 'B' for warmup timeout, got %c", errCode)
	}
}

func TestRegenAbortRoughTimeout(t *testing.T) {
	p := NewPump(4.0, 0.0)

	// Very short rough timeout, very low target so it won't reach
	params := DefaultRegenParams()
	params.RoughTimeout = 1 * time.Millisecond
	params.RoughVacuumTorr = 1e-10 // impossibly low
	p.SetRegenParams(params)

	setColdState(p)
	p.HandleCommand("start_regen")

	// Advance to roughing
	p.AdvanceRegenStep() // -> purge
	p.AdvanceRegenStep() // -> roughing

	// Wait for timeout
	time.Sleep(5 * time.Millisecond)

	// Trigger update
	p.Snapshot()

	p.mu.RLock()
	state := p.state
	errCode := p.regenError
	p.mu.RUnlock()

	if state != StateOff {
		t.Errorf("expected StateOff after rough timeout, got %d", state)
	}
	if errCode != 'G' {
		t.Errorf("expected error 'G' for rough timeout, got %c", errCode)
	}
}

func TestRegenGetError(t *testing.T) {
	p := NewPump(4.0, 0.0)

	// Before any regen, error should be '@'
	resp, ok := p.HandleCommand("get_regen_error")
	if !ok || resp != "@" {
		t.Errorf("expected '@' (no error), got %q ok=%v", resp, ok)
	}

	// Start and abort regen
	setColdState(p)
	p.HandleCommand("start_regen")
	p.HandleCommand("abort_regen")

	resp, ok = p.HandleCommand("get_regen_error")
	if !ok || resp != "F" {
		t.Errorf("expected 'F' after abort, got %q ok=%v", resp, ok)
	}
}

func TestAdvanceAllPhases(t *testing.T) {
	p := NewPump(4.0, 0.0)
	setColdState(p)
	p.HandleCommand("start_regen")

	expected := []struct {
		phase int
		name  string
	}{
		{1, "warming"},
		{2, "extended purge"},
		{3, "roughing"},
		{4, "rate of rise"},
		{5, "cooling"},
	}

	for i, exp := range expected {
		snap := p.Snapshot()
		if snap.RegenStep != exp.phase {
			t.Errorf("step %d: expected phase %d, got %d", i, exp.phase, snap.RegenStep)
		}
		if snap.RegenPhaseName != exp.name {
			t.Errorf("step %d: expected name %q, got %q", i, exp.name, snap.RegenPhaseName)
		}
		if i < len(expected)-1 {
			p.AdvanceRegenStep()
		}
	}

	// Final advance should complete regen → StateCooling (may immediately transition to StateCold)
	p.AdvanceRegenStep()
	snap := p.Snapshot()
	if snap.State != StateCooling && snap.State != StateCold {
		t.Errorf("expected StateCooling or StateCold after full advance, got %d (%s)", snap.State, snap.StateName)
	}
	if snap.RegenStep != 0 {
		t.Errorf("expected regen step 0 after completion, got %d", snap.RegenStep)
	}
}

func TestPumpOffDuringRegen(t *testing.T) {
	p := NewPump(4.0, 0.0)
	setColdState(p)
	p.HandleCommand("start_regen")

	// pump_off during regen should abort with 'F'
	resp, ok := p.HandleCommand("pump_off")
	if !ok || resp != "A" {
		t.Errorf("expected A, got %s ok=%v", resp, ok)
	}

	p.mu.RLock()
	state := p.state
	errCode := p.regenError
	p.mu.RUnlock()

	if state != StateOff {
		t.Errorf("expected StateOff, got %d", state)
	}
	if errCode != 'F' {
		t.Errorf("expected error 'F', got %c", errCode)
	}
}

func TestStatusByte1Valves(t *testing.T) {
	p := NewPump(4.0, 0.0)
	setColdState(p)
	p.HandleCommand("start_regen")

	// Warming: purge open (bit 2), rough closed (bit 1 off)
	resp, _ := p.HandleCommand("get_status_1")
	status, _ := strconv.Atoi(resp)
	if status&0x04 == 0 {
		t.Errorf("warming: expected purge valve bit (0x04) set, got 0x%02x", status)
	}
	if status&0x02 != 0 {
		t.Errorf("warming: expected rough valve bit (0x02) clear, got 0x%02x", status)
	}

	// Advance to roughing: rough open (bit 1), purge closed (bit 2 off)
	p.AdvanceRegenStep() // -> purge
	p.AdvanceRegenStep() // -> roughing

	resp, _ = p.HandleCommand("get_status_1")
	status, _ = strconv.Atoi(resp)
	if status&0x02 == 0 {
		t.Errorf("roughing: expected rough valve bit (0x02) set, got 0x%02x", status)
	}
	if status&0x04 != 0 {
		t.Errorf("roughing: expected purge valve bit (0x04) clear, got 0x%02x", status)
	}

	// Advance to ROR: both valves closed
	p.AdvanceRegenStep() // -> ROR
	resp, _ = p.HandleCommand("get_status_1")
	status, _ = strconv.Atoi(resp)
	if status&0x02 != 0 {
		t.Errorf("ROR: expected rough valve bit clear, got 0x%02x", status)
	}
	if status&0x04 != 0 {
		t.Errorf("ROR: expected purge valve bit clear, got 0x%02x", status)
	}
}

func TestCoolingStateTransition(t *testing.T) {
	p := NewPump(4.0, 0.0)

	p.HandleCommand("pump_on")
	if p.state != StateCooling {
		t.Errorf("expected StateCooling after pump_on, got %d", p.state)
	}

	// Simulate passage of time
	p.mu.Lock()
	p.lastUpdate = time.Now().Add(-24 * time.Hour)
	p.mu.Unlock()

	p.HandleCommand("pump_status")

	p.mu.RLock()
	state := p.state
	first := p.firstStageK
	second := p.secondStageK
	p.mu.RUnlock()

	if state != StateCold {
		t.Errorf("expected StateCold after 24h, got %d (1st=%.1fK 2nd=%.1fK)", state, first, second)
	}
}

func TestExponentialDecay(t *testing.T) {
	result := exponentialDecay(295.0, 65.0, 3600.0, 3600.0)
	expected := 65.0 + (295.0-65.0)*0.367879441
	if diff := result - expected; diff > 1.0 || diff < -1.0 {
		t.Errorf("expected ~%.1f, got %.1f", expected, result)
	}
}

func TestDriftToward(t *testing.T) {
	result := driftToward(100.0, 295.0, 10.0, 0.01)
	if result <= 100.0 {
		t.Errorf("expected drift upward, got %.1f", result)
	}
	if result > 295.0 {
		t.Errorf("should not overshoot target, got %.1f", result)
	}

	result = driftToward(294.0, 295.0, 1000.0, 1.0)
	if result != 295.0 {
		t.Errorf("expected clamped to 295.0, got %.1f", result)
	}
}

func TestFailRate(t *testing.T) {
	p := NewPump(4.0, 1.0)
	_, ok := p.HandleCommand("pump_status")
	if ok {
		t.Error("expected failure with 100% fail rate")
	}
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateOff, "off"},
		{StateCooling, "cooling"},
		{StateCold, "cold"},
		{StateRegen, "regen"},
		{State(99), "unknown"},
	}
	for _, tt := range tests {
		got := tt.state.String()
		if got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestRegenPhaseString(t *testing.T) {
	tests := []struct {
		phase RegenPhase
		want  string
	}{
		{RegenPhaseNone, "none"},
		{RegenPhaseWarming, "warming"},
		{RegenPhasePurge, "extended purge"},
		{RegenPhaseRoughing, "roughing"},
		{RegenPhaseROR, "rate of rise"},
		{RegenPhaseCooling, "cooling"},
		{RegenPhase(99), "unknown"},
	}
	for _, tt := range tests {
		got := tt.phase.String()
		if got != tt.want {
			t.Errorf("RegenPhase(%d).String() = %q, want %q", tt.phase, got, tt.want)
		}
	}
}

func TestSnapshot(t *testing.T) {
	p := NewPump(4.0, 0.05)
	snap := p.Snapshot()

	if snap.State != StateOff {
		t.Errorf("expected StateOff, got %d", snap.State)
	}
	if snap.StateName != "off" {
		t.Errorf("expected state_name 'off', got %q", snap.StateName)
	}
	if snap.FirstStageK < 290.0 || snap.FirstStageK > 300.0 {
		t.Errorf("expected room temp 1st stage, got %.1f", snap.FirstStageK)
	}
	if snap.SecondStageK < 290.0 || snap.SecondStageK > 300.0 {
		t.Errorf("expected room temp 2nd stage, got %.1f", snap.SecondStageK)
	}
	if snap.CooldownHours != 4.0 {
		t.Errorf("expected cooldown 4.0, got %.1f", snap.CooldownHours)
	}
	if snap.FailRate != 0.05 {
		t.Errorf("expected fail rate 0.05, got %.2f", snap.FailRate)
	}
	if snap.OperatingHours > 0.1 {
		t.Errorf("expected ~0 operating hours, got %.1f", snap.OperatingHours)
	}
	if snap.RegenPhaseName != "none" {
		t.Errorf("expected regen phase 'none', got %q", snap.RegenPhaseName)
	}
	if snap.RegenError != "@" {
		t.Errorf("expected regen error '@', got %q", snap.RegenError)
	}
}

func TestSetState(t *testing.T) {
	p := NewPump(4.0, 0.0)

	// Off -> Cooling
	p.SetState(StateCooling)
	snap := p.Snapshot()
	if snap.State != StateCooling {
		t.Errorf("expected StateCooling, got %d", snap.State)
	}

	// Cooling -> Regen (allowed)
	p.SetState(StateRegen)
	snap = p.Snapshot()
	if snap.State != StateRegen {
		t.Errorf("expected StateRegen, got %d", snap.State)
	}
	if snap.RegenStep != 1 {
		t.Errorf("expected regen step 1, got %d", snap.RegenStep)
	}

	// Regen -> Off (via abort)
	p.SetState(StateOff)
	snap = p.Snapshot()
	if snap.State != StateOff {
		t.Errorf("expected StateOff, got %d", snap.State)
	}
	if snap.RegenStep != 0 {
		t.Errorf("expected regen step 0 after off, got %d", snap.RegenStep)
	}

	// Off -> Regen (allowed)
	p.SetState(StateRegen)
	snap = p.Snapshot()
	if snap.State != StateRegen {
		t.Errorf("expected StateRegen from off, got %d (%s)", snap.State, snap.StateName)
	}
	p.SetState(StateOff)
}

func TestSetTemperatures(t *testing.T) {
	p := NewPump(4.0, 0.0)

	p.SetTemperatures(100.0, 50.0)
	snap := p.Snapshot()
	if snap.FirstStageK < 95.0 || snap.FirstStageK > 300.0 {
		t.Errorf("expected ~100K 1st stage, got %.1f", snap.FirstStageK)
	}

	p.SetTemperatures(1.0, 1.0)
	snap = p.Snapshot()
	if snap.FirstStageK < 4.0 {
		t.Errorf("expected clamped to >= 4K, got %.1f", snap.FirstStageK)
	}
	if snap.SecondStageK < 4.0 {
		t.Errorf("expected clamped to >= 4K, got %.1f", snap.SecondStageK)
	}

	p.SetTemperatures(500.0, 500.0)
	snap = p.Snapshot()
	if snap.FirstStageK > 320.0 {
		t.Errorf("expected clamped to <= 320K, got %.1f", snap.FirstStageK)
	}
	if snap.SecondStageK > 320.0 {
		t.Errorf("expected clamped to <= 320K, got %.1f", snap.SecondStageK)
	}
}

func TestSetCooldownHours(t *testing.T) {
	p := NewPump(4.0, 0.0)

	err := p.SetCooldownHours(6.0)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	snap := p.Snapshot()
	if snap.CooldownHours != 6.0 {
		t.Errorf("expected 6.0, got %.1f", snap.CooldownHours)
	}

	err = p.SetCooldownHours(0)
	if err == nil {
		t.Error("expected error for zero cooldown hours")
	}

	err = p.SetCooldownHours(-1.0)
	if err == nil {
		t.Error("expected error for negative cooldown hours")
	}
}

func TestSetFailRate(t *testing.T) {
	p := NewPump(4.0, 0.0)

	p.SetFailRate(0.5)
	snap := p.Snapshot()
	if snap.FailRate != 0.5 {
		t.Errorf("expected 0.5, got %.2f", snap.FailRate)
	}

	p.SetFailRate(2.0)
	snap = p.Snapshot()
	if snap.FailRate != 1.0 {
		t.Errorf("expected clamped to 1.0, got %.2f", snap.FailRate)
	}

	p.SetFailRate(-0.5)
	snap = p.Snapshot()
	if snap.FailRate != 0.0 {
		t.Errorf("expected clamped to 0.0, got %.2f", snap.FailRate)
	}
}

func TestDuplicatePumpOn(t *testing.T) {
	p := NewPump(4.0, 0.0)

	p.HandleCommand("pump_on")
	if p.state != StateCooling {
		t.Fatal("expected cooling state")
	}

	resp, ok := p.HandleCommand("pump_on")
	if !ok || resp != "A" {
		t.Errorf("expected A for duplicate pump_on, got %s ok=%v", resp, ok)
	}
}

func TestOperatingHoursDuringRegen(t *testing.T) {
	p := NewPump(4.0, 0.0)
	setColdState(p)

	initialHours := p.Snapshot().OperatingHours

	p.HandleCommand("start_regen")

	// Simulate time passing
	p.mu.Lock()
	p.lastUpdate = time.Now().Add(-1 * time.Hour)
	p.mu.Unlock()

	snap := p.Snapshot()
	if snap.OperatingHours <= initialHours {
		t.Errorf("expected operating hours to increase during regen, initial=%.2f current=%.2f",
			initialHours, snap.OperatingHours)
	}
}

func TestAbortRegenMethod(t *testing.T) {
	p := NewPump(4.0, 0.0)
	setColdState(p)
	p.HandleCommand("start_regen")

	p.AbortRegen()

	snap := p.Snapshot()
	if snap.State != StateOff {
		t.Errorf("expected StateOff after AbortRegen(), got %d", snap.State)
	}
	if snap.RegenError != "F" {
		t.Errorf("expected error 'F', got %q", snap.RegenError)
	}
}

// setColdState is a helper that puts the pump into StateCold directly.
func setColdState(p *Pump) {
	p.mu.Lock()
	p.state = StateCold
	p.firstStageK = 65.0
	p.secondStageK = 15.0
	p.lastUpdate = time.Now()
	p.mu.Unlock()
}
