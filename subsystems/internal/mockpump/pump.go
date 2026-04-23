// Package mockpump simulates a CTI cryopump with pendant2-style temperature curves.
//
// Temperature simulation uses linear interpolation over fixed durations for state
// transitions and sinusoidal variation for steady-state operation, matching the
// pendant2 mock pump implementation.
package mockpump

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"
)

// State represents the pump's operational state.
type State int

const (
	StateOff     State = iota // Pump off, warming to room temp
	StateCooling              // Pump on, cooling down
	StateCold                 // Pump on, at base temperature
	StateRegen                // Regeneration cycle
)

// RegenPhase represents the current phase within a regeneration cycle.
type RegenPhase int

const (
	RegenPhaseNone     RegenPhase = iota // Not in regen
	RegenPhaseWarming                    // Phase 1: Heating to warmup temp
	RegenPhasePurge                      // Phase 2: Extended nitrogen purge
	RegenPhaseRoughing                   // Phase 3: Rough pump to base vacuum
	RegenPhaseROR                        // Phase 4: Rate-of-rise test
	RegenPhaseCooling                    // Phase 5: Cooldown after successful ROR
)

// String returns the human-readable name for a regen phase.
func (rp RegenPhase) String() string {
	switch rp {
	case RegenPhaseNone:
		return "none"
	case RegenPhaseWarming:
		return "warming"
	case RegenPhasePurge:
		return "extended purge"
	case RegenPhaseRoughing:
		return "roughing"
	case RegenPhaseROR:
		return "rate of rise"
	case RegenPhaseCooling:
		return "cooling"
	default:
		return "unknown"
	}
}

// RegenParams holds configurable parameters for the regen cycle.
type RegenParams struct {
	WarmupTempK      float64       // Target warmup temperature (K)
	RoughVacuumTorr  float64       // Retained for API compat
	RORLimitMTorrMin float64       // Max acceptable rate-of-rise (mTorr/min)
	MaxRORRetries    int           // Max ROR retry attempts before abort
	WarmupTimeout    time.Duration // Max time in warming phase (safety net)
	RoughTimeout     time.Duration // Max time in roughing phase (safety net)
	CooldownTargetK  float64       // Retained for API compat

	// Phase durations (pendant2-style fixed timing)
	WarmupDuration   time.Duration
	PurgeDuration    time.Duration
	RoughDuration    time.Duration
	RORDuration      time.Duration
	CooldownDuration time.Duration
}

// DefaultRegenParams returns regen parameters matching pendant2 timing.
func DefaultRegenParams() RegenParams {
	return RegenParams{
		WarmupTempK:      295.0,
		RoughVacuumTorr:  0.050,
		RORLimitMTorrMin: 20.0,
		MaxRORRetries:    3,
		WarmupTimeout:    2 * time.Minute,
		RoughTimeout:     2 * time.Minute,
		CooldownTargetK:  15.0,

		WarmupDuration:   40 * time.Second,
		PurgeDuration:    30 * time.Second,
		RoughDuration:    20 * time.Second,
		RORDuration:      20 * time.Second,
		CooldownDuration: 60 * time.Second,
	}
}

// Pump simulates a CTI cryopump with pendant2-style temperature curves.
type Pump struct {
	mu            sync.RWMutex
	state         State
	pumpOnTime    time.Time
	firstStageK   float64
	secondStageK  float64
	pressure      float64 // Tracked pressure (Torr)
	lastUpdate    time.Time
	cooldownHours float64 // Retained for API compat
	failRate      float64

	// Operating hours accumulator
	totalOnSeconds float64

	// Valve state
	roughValveOpen bool
	purgeValveOpen bool

	// Interpolation state (pendant2-style linear interpolation)
	stage1Target   float64
	stage2Target   float64
	pressureTarget float64
	stage1Start    float64
	stage2Start    float64
	pressureStart  float64
	phaseDuration  float64   // Seconds; 0 = sinusoidal steady-state
	phaseStartTime time.Time // When current phase began

	// Sinusoidal variation state (pendant2-style steady-state)
	variationPhase  float64   // Radians, wraps at 2*pi
	lastVariationAt time.Time // Last sinusoidal phase increment

	// Regen state
	regenPhase       RegenPhase
	regenPhaseStart  time.Time
	regenError       byte    // '@' = no error, 'F' = manual, 'E' = ROR limit, 'B' = warmup timeout, 'G' = rough timeout
	regenRetryCount  int     // ROR retry counter
	regenPressure    float64 // Chamber pressure during roughing/ROR (Torr)
	rorStartPressure float64
	rorStartTime     time.Time
	heatersOn        bool
	regenParams      RegenParams
	regenCompleted   bool // Post-regen flag, cleared on next pump-off or pump-on
}

// NewPump creates a pump simulator.
func NewPump(cooldownHours float64, failRate float64) *Pump {
	now := time.Now()
	return &Pump{
		state:          StateOff,
		firstStageK:    295.0,
		secondStageK:   295.0,
		pressure:       1.0,
		lastUpdate:     now,
		cooldownHours:  cooldownHours,
		failRate:       failRate,
		regenError:     '@',
		regenParams:    DefaultRegenParams(),
		stage1Target:   295.0,
		stage2Target:   295.0,
		pressureTarget: 1.0,
		stage1Start:    295.0,
		stage2Start:    295.0,
		pressureStart:  1.0,
		phaseDuration:  120.0,
		phaseStartTime: now,
		lastVariationAt: now,
	}
}

// HandleCommand processes a pump command and returns a response.
func (p *Pump) HandleCommand(command string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.updateTemperatures()

	// Random failure simulation
	if p.failRate > 0 && rand.Float64() < p.failRate {
		return "ERR", false
	}

	switch command {
	case "pump_status":
		if p.state == StateOff {
			return "0", true
		}
		return "1", true

	case "pump_on":
		if p.state == StateOff {
			p.transitionTo(StateCooling)
			p.pumpOnTime = time.Now()
			p.regenCompleted = false
		}
		return "A", true

	case "pump_off":
		if p.state == StateRegen {
			p.abortRegen('F')
		} else {
			p.transitionTo(StateOff)
		}
		p.regenCompleted = false
		return "A", true

	case "get_temp_1st_stage":
		return fmt.Sprintf("%.1f", p.firstStageK), true

	case "get_temp_2nd_stage":
		return fmt.Sprintf("%.1f", p.secondStageK), true

	case "get_pump_tc_pressure":
		return fmt.Sprintf("%.2e", p.simulatePressure()), true

	case "get_operating_hours":
		hours := p.totalOnSeconds / 3600.0
		return fmt.Sprintf("%.1f", hours), true

	case "get_status_1":
		return p.statusByte1(), true

	case "get_status_2":
		return p.statusByte2(), true

	case "get_status_3":
		return "0", true

	case "start_regen":
		if p.state != StateRegen {
			p.startRegen()
			return "A", true
		}
		return "N", false

	case "abort_regen":
		if p.state == StateRegen {
			p.abortRegen('F')
			return "A", true
		}
		return "N", false

	case "get_regen_step":
		return fmt.Sprintf("%d", int(p.regenPhase)), true

	case "get_regen_error":
		return string(p.regenError), true

	case "get_regen_status":
		return string(p.regenStatusChar()), true

	case "open_rough_valve":
		p.roughValveOpen = true
		return "A", true

	case "close_rough_valve":
		p.roughValveOpen = false
		return "A", true

	case "get_rough_valve":
		if p.roughValveOpen {
			return "1", true
		}
		return "0", true

	case "open_purge_valve":
		p.purgeValveOpen = true
		return "A", true

	case "close_purge_valve":
		p.purgeValveOpen = false
		return "A", true

	case "get_purge_valve":
		if p.purgeValveOpen {
			return "1", true
		}
		return "0", true

	case "identify":
		return "CTI-Cryogenics,Cryo-Torr 8,SIM-001,1.0", true

	case "get_telemetry":
		return p.buildTelemetryJSON(), true

	default:
		return "?", false
	}
}

// buildTelemetryJSON emits the cached telemetry snapshot in the shape
// documented in docs/SCRIPTING_HAL.md "Telemetry Snapshot". Must match
// the firmware's serializePumpTelemetryJson exactly.
func (p *Pump) buildTelemetryJSON() string {
	var status1 int
	if p.state != StateOff {
		status1 |= 0x01
	}
	if p.roughValveOpen {
		status1 |= 0x02
	}
	if p.purgeValveOpen {
		status1 |= 0x04
	}
	if p.state != StateOff {
		status1 |= 0x08
	}
	status1 |= 0x20

	snap := map[string]interface{}{
		"stage1_temp_k":    p.firstStageK,
		"stage2_temp_k":    p.secondStageK,
		"pressure_torr":    p.simulatePressure(),
		"pump_on":          p.state != StateOff,
		"rough_valve_open": p.roughValveOpen,
		"purge_valve_open": p.purgeValveOpen,
		"regen_char":       string(p.regenStatusChar()),
		"operating_hours":  int(p.totalOnSeconds / 3600.0),
		"status_1":         status1,
		"stale_count":      0,
		"last_update_ms":   uint32(time.Now().UnixMilli()),
	}
	b, err := json.Marshal(snap)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// startRegen initializes the regen cycle and enters the warming phase.
func (p *Pump) startRegen() {
	p.state = StateRegen
	p.regenError = '@'
	p.regenRetryCount = 0
	p.regenPressure = 0
	p.regenCompleted = false
	p.enterPhase(RegenPhaseWarming)
}

// enterPhase transitions to a new regen phase, capturing start values and
// setting targets/durations for pendant2-style linear interpolation.
func (p *Pump) enterPhase(phase RegenPhase) {
	p.captureStartValues()
	p.regenPhase = phase
	p.regenPhaseStart = time.Now()

	switch phase {
	case RegenPhaseWarming:
		p.heatersOn = true
		p.purgeValveOpen = true
		p.roughValveOpen = false
		p.stage1Target = 295.0
		p.stage2Target = 295.0
		p.pressureTarget = 100.0
		p.phaseDuration = p.regenParams.WarmupDuration.Seconds()

	case RegenPhasePurge:
		p.heatersOn = true
		p.purgeValveOpen = true
		p.roughValveOpen = false
		p.stage1Target = 295.0
		p.stage2Target = 295.0
		p.pressureTarget = 50.0
		p.phaseDuration = p.regenParams.PurgeDuration.Seconds()

	case RegenPhaseRoughing:
		p.purgeValveOpen = false
		p.roughValveOpen = true
		p.heatersOn = true
		p.stage1Target = 295.0
		p.stage2Target = 295.0
		p.pressureTarget = 25.0
		p.phaseDuration = p.regenParams.RoughDuration.Seconds()

	case RegenPhaseROR:
		p.roughValveOpen = false
		p.purgeValveOpen = false
		p.heatersOn = true
		p.stage1Target = 295.0
		p.stage2Target = 295.0
		p.pressureTarget = 25.0
		p.phaseDuration = p.regenParams.RORDuration.Seconds()
		p.rorStartPressure = p.pressure
		p.regenPressure = p.pressure
		p.rorStartTime = time.Now()

	case RegenPhaseCooling:
		p.heatersOn = false
		p.roughValveOpen = false
		p.purgeValveOpen = false
		p.stage1Target = 65.0
		p.stage2Target = 15.0
		p.pressureTarget = 1.5e-6
		p.phaseDuration = p.regenParams.CooldownDuration.Seconds()
	}
}

// abortRegen stops the regen cycle with an error code and transitions to StateOff.
func (p *Pump) abortRegen(errCode byte) {
	p.regenError = errCode
	p.regenPhase = RegenPhaseNone
	p.heatersOn = false
	p.roughValveOpen = false
	p.purgeValveOpen = false
	p.transitionTo(StateOff)
}

// captureStartValues snapshots current values as interpolation start points.
func (p *Pump) captureStartValues() {
	p.stage1Start = p.firstStageK
	p.stage2Start = p.secondStageK
	p.pressureStart = p.pressure
	p.phaseStartTime = time.Now()
}

// transitionTo transitions the pump to a new state with appropriate targets.
func (p *Pump) transitionTo(s State) {
	p.captureStartValues()
	p.state = s

	switch s {
	case StateOff:
		p.stage1Target = 295.0
		p.stage2Target = 295.0
		p.pressureTarget = 1.0
		p.phaseDuration = 120.0

	case StateCooling:
		p.stage1Target = 65.0
		p.stage2Target = 15.0
		p.pressureTarget = 1.5e-6
		p.phaseDuration = 60.0

	case StateCold:
		p.stage1Target = 65.0
		p.stage2Target = 15.0
		p.pressureTarget = 1.5e-6
		p.phaseDuration = 0 // Sinusoidal steady-state
	}
}

// updateTemperatures simulates temperature and pressure changes using
// pendant2-style linear interpolation and sinusoidal variation.
func (p *Pump) updateTemperatures() {
	now := time.Now()
	dt := now.Sub(p.lastUpdate).Seconds()
	p.lastUpdate = now

	if p.state == StateCooling || p.state == StateCold || p.state == StateRegen {
		p.totalOnSeconds += dt
	}

	// State-specific transitions
	switch p.state {
	case StateCooling:
		if p.phaseDuration > 0 {
			elapsed := now.Sub(p.phaseStartTime).Seconds()
			if elapsed >= p.phaseDuration {
				p.transitionTo(StateCold)
			}
		}
	case StateRegen:
		p.updateRegen(now, dt)
	}

	// Temperature and pressure update
	if p.phaseDuration > 0 {
		// Linear interpolation over phase duration (pendant2-style)
		elapsed := now.Sub(p.phaseStartTime).Seconds()
		progress := math.Min(elapsed/p.phaseDuration, 1.0)

		p.firstStageK = p.stage1Start + (p.stage1Target-p.stage1Start)*progress
		p.secondStageK = p.stage2Start + (p.stage2Target-p.stage2Start)*progress

		// Pressure: log-scale interpolation (skip during ROR -- handled by updateRegen)
		if !(p.state == StateRegen && p.regenPhase == RegenPhaseROR) {
			if p.pressureStart > 0 && p.pressureTarget > 0 {
				logStart := math.Log(p.pressureStart)
				logTarget := math.Log(p.pressureTarget)
				p.pressure = math.Exp(logStart + (logTarget-logStart)*progress)
			}
		}
	} else {
		// Sinusoidal steady-state variation (pendant2-style)
		if now.Sub(p.lastVariationAt) >= 250*time.Millisecond {
			p.variationPhase += 0.196 // pi/16 radians
			if p.variationPhase > 2*math.Pi {
				p.variationPhase -= 2 * math.Pi
			}
			p.lastVariationAt = now
		}

		variation1 := 2.0 * math.Sin(p.variationPhase)
		variation2 := 3.0 * math.Sin(p.variationPhase+math.Pi/2)

		const maxChange = 0.1
		delta1 := (p.stage1Target + variation1) - p.firstStageK
		if delta1 > maxChange {
			delta1 = maxChange
		}
		if delta1 < -maxChange {
			delta1 = -maxChange
		}
		p.firstStageK += delta1

		delta2 := (p.stage2Target + variation2) - p.secondStageK
		if delta2 > maxChange {
			delta2 = maxChange
		}
		if delta2 < -maxChange {
			delta2 = -maxChange
		}
		p.secondStageK += delta2

		// Pressure: sinusoidal +/-20% variation around target
		pressureVariation := 0.2 * math.Sin(p.variationPhase+math.Pi)
		effectiveTarget := p.pressureTarget * (1.0 + pressureVariation)
		p.pressure += (effectiveTarget - p.pressure) * 0.05
	}

	// Track regenPressure during roughing for Snapshot
	if p.state == StateRegen && p.regenPhase == RegenPhaseRoughing {
		p.regenPressure = p.pressure
	}

	// Clamp temperatures (pendant2 bounds: 10-350K)
	p.firstStageK = math.Max(10.0, math.Min(350.0, p.firstStageK))
	p.secondStageK = math.Max(10.0, math.Min(350.0, p.secondStageK))
	p.pressure = math.Max(1e-9, math.Min(1000.0, p.pressure))
}

// updateRegen handles regen phase transitions and ROR-specific pressure logic.
func (p *Pump) updateRegen(now time.Time, dt float64) {
	elapsed := now.Sub(p.regenPhaseStart)

	switch p.regenPhase {
	case RegenPhaseWarming:
		if elapsed >= p.regenParams.WarmupDuration {
			p.enterPhase(RegenPhasePurge)
		} else if elapsed >= p.regenParams.WarmupTimeout {
			p.abortRegen('B')
		}

	case RegenPhasePurge:
		if elapsed >= p.regenParams.PurgeDuration {
			p.enterPhase(RegenPhaseRoughing)
		}

	case RegenPhaseRoughing:
		if elapsed >= p.regenParams.RoughDuration {
			p.enterPhase(RegenPhaseROR)
		} else if elapsed >= p.regenParams.RoughTimeout {
			p.abortRegen('G')
		}

	case RegenPhaseROR:
		// Pressure rises linearly for rate-of-rise test
		riseTorrPerSec := 10.0e-3 / 60.0
		p.regenPressure += riseTorrPerSec * dt
		p.pressure = p.regenPressure

		// Evaluate after ROR duration
		rorElapsed := now.Sub(p.rorStartTime)
		if rorElapsed >= p.regenParams.RORDuration {
			minutes := rorElapsed.Minutes()
			rateMTorrMin := (p.regenPressure - p.rorStartPressure) * 1000.0 / minutes

			if rateMTorrMin < p.regenParams.RORLimitMTorrMin {
				p.enterPhase(RegenPhaseCooling)
			} else {
				p.regenRetryCount++
				if p.regenRetryCount >= p.regenParams.MaxRORRetries {
					p.abortRegen('E')
				} else {
					p.enterPhase(RegenPhasePurge)
				}
			}
		}

	case RegenPhaseCooling:
		if elapsed >= p.regenParams.CooldownDuration {
			p.regenPhase = RegenPhaseNone
			p.regenCompleted = true
			p.transitionTo(StateCooling)
		}
	}
}

func (p *Pump) simulatePressure() float64 {
	return p.pressure
}

// statusByte1 returns CTI S1 status byte.
// Bit 0 (0x01) = Pump ON
// Bit 1 (0x02) = Rough valve ON
// Bit 2 (0x04) = Purge valve ON
// Bit 3 (0x08) = Cryo TC ON (on when pump is on)
// Bit 5 (0x20) = Power fail (1 = normal, always set)
func (p *Pump) statusByte1() string {
	var status int
	if p.state != StateOff {
		status |= 0x01 // Pump ON
	}
	if p.roughValveOpen {
		status |= 0x02 // Rough valve ON
	}
	if p.purgeValveOpen {
		status |= 0x04 // Purge valve ON
	}
	if p.state != StateOff {
		status |= 0x08 // Cryo TC ON
	}
	status |= 0x20 // Power fail = normal (always set)
	return fmt.Sprintf("%d", status)
}

func (p *Pump) statusByte2() string {
	return "0"
}

// regenStatusChar returns a CTI-style O-command character for the current regen state.
func (p *Pump) regenStatusChar() byte {
	// Error state takes precedence
	if p.regenError != '@' {
		return 'V' // Regen aborted
	}

	if p.state == StateRegen {
		switch p.regenPhase {
		case RegenPhaseWarming:
			return 'B'
		case RegenPhasePurge:
			return 'H'
		case RegenPhaseRoughing:
			return 'I'
		case RegenPhaseROR:
			return 'L'
		case RegenPhaseCooling:
			return 'M'
		}
	}

	// Post-regen completed, still cooling down
	if p.regenCompleted {
		if p.state == StateCold {
			return 'P' // Regen complete, fully cold
		}
		return 'M' // Still cooling
	}

	return 'P' // No regen in progress
}

// String returns the human-readable name for a pump state.
func (s State) String() string {
	switch s {
	case StateOff:
		return "off"
	case StateCooling:
		return "cooling"
	case StateCold:
		return "cold"
	case StateRegen:
		return "regen"
	default:
		return "unknown"
	}
}

// PumpSnapshot captures a point-in-time view of the pump's state.
type PumpSnapshot struct {
	State          State   `json:"state"`
	StateName      string  `json:"state_name"`
	FirstStageK    float64 `json:"first_stage_k"`
	SecondStageK   float64 `json:"second_stage_k"`
	PressureAtm    float64 `json:"pressure_atm"`
	RegenStep      int     `json:"regen_step"`
	RegenPhaseName string  `json:"regen_phase_name"`
	RegenStatus    string  `json:"regen_status"`
	RegenPressure  float64 `json:"regen_pressure"`
	RegenRetries   int     `json:"regen_retries"`
	RegenError     string  `json:"regen_error"`
	HeatersOn      bool    `json:"heaters_on"`
	CooldownHours  float64 `json:"cooldown_hours"`
	FailRate       float64 `json:"fail_rate"`
	OperatingHours float64 `json:"operating_hours"`
	RoughValveOpen bool    `json:"rough_valve_open"`
	PurgeValveOpen bool    `json:"purge_valve_open"`
}

// Snapshot returns a point-in-time view of the pump's state.
func (p *Pump) Snapshot() PumpSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.updateTemperatures()

	return PumpSnapshot{
		State:          p.state,
		StateName:      p.state.String(),
		FirstStageK:    p.firstStageK,
		SecondStageK:   p.secondStageK,
		PressureAtm:    p.simulatePressure(),
		RegenStep:      int(p.regenPhase),
		RegenPhaseName: p.regenPhase.String(),
		RegenStatus:    string(p.regenStatusChar()),
		RegenPressure:  p.regenPressure,
		RegenRetries:   p.regenRetryCount,
		RegenError:     string(p.regenError),
		HeatersOn:      p.heatersOn,
		CooldownHours:  p.cooldownHours,
		FailRate:       p.failRate,
		OperatingHours: p.totalOnSeconds / 3600.0,
		RoughValveOpen: p.roughValveOpen,
		PurgeValveOpen: p.purgeValveOpen,
	}
}

// SetState forces the pump to a specific state with proper side effects.
func (p *Pump) SetState(s State) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.updateTemperatures()

	switch s {
	case StateCooling:
		if p.state == StateOff {
			p.pumpOnTime = time.Now()
		}
		p.transitionTo(StateCooling)
	case StateRegen:
		if p.state == StateRegen {
			return
		}
		p.startRegen()
	case StateOff:
		if p.state == StateRegen {
			p.abortRegen('F')
			return
		}
		p.regenPhase = RegenPhaseNone
		p.transitionTo(StateOff)
	case StateCold:
		p.transitionTo(StateCold)
	}
}

// SetTemperatures overrides first and second stage temperatures.
// Values are clamped to [10, 350].
func (p *Pump) SetTemperatures(firstK, secondK float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.firstStageK = math.Max(10.0, math.Min(350.0, firstK))
	p.secondStageK = math.Max(10.0, math.Min(350.0, secondK))
	p.lastUpdate = time.Now()
}

// SetCooldownHours sets the simulated cooldown time. Rejects zero or negative values.
func (p *Pump) SetCooldownHours(hours float64) error {
	if hours <= 0 {
		return fmt.Errorf("cooldown hours must be positive, got %.2f", hours)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cooldownHours = hours
	return nil
}

// SetRoughValve sets the rough valve state.
func (p *Pump) SetRoughValve(open bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.roughValveOpen = open
}

// SetPurgeValve sets the purge valve state.
func (p *Pump) SetPurgeValve(open bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.purgeValveOpen = open
}

// SetFailRate sets the random failure probability. Clamped to [0.0, 1.0].
func (p *Pump) SetFailRate(rate float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failRate = math.Max(0.0, math.Min(1.0, rate))
}

// SetRegenParams sets the regen cycle parameters (for test configurability).
func (p *Pump) SetRegenParams(params RegenParams) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.regenParams = params
}

// AbortRegen aborts the regen cycle with a manual abort code.
func (p *Pump) AbortRegen() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state == StateRegen {
		p.abortRegen('F')
	}
}

// AdvanceRegenStep advances to the next regen phase (for console "+" button).
func (p *Pump) AdvanceRegenStep() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != StateRegen {
		return
	}

	switch p.regenPhase {
	case RegenPhaseWarming:
		// Set temps to warmup target for clean transition
		p.firstStageK = p.stage1Target
		p.secondStageK = p.stage2Target
		p.enterPhase(RegenPhasePurge)

	case RegenPhasePurge:
		p.enterPhase(RegenPhaseRoughing)

	case RegenPhaseRoughing:
		p.enterPhase(RegenPhaseROR)

	case RegenPhaseROR:
		p.enterPhase(RegenPhaseCooling)

	case RegenPhaseCooling:
		p.firstStageK = 65.0
		p.secondStageK = 15.0
		p.regenPhase = RegenPhaseNone
		p.regenCompleted = true
		p.transitionTo(StateCooling)
	}
}
