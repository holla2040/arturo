// Package mockpump simulates a CTI cryopump with realistic temperature curves.
package mockpump

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"
)

// State represents the pump's operational state.
type State int

const (
	StateOff     State = iota // Pump off, at or drifting to room temp
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
	ExtPurgeDuration time.Duration // Extended purge hold time
	RoughVacuumTorr  float64       // Roughing target pressure (Torr)
	RORLimitMTorrMin float64       // Max acceptable rate-of-rise (mTorr/min)
	MaxRORRetries    int           // Max ROR retry attempts before abort
	WarmupTimeout    time.Duration // Max time in warming phase
	RoughTimeout     time.Duration // Max time in roughing phase
	CooldownTargetK  float64       // Target temp to end cooldown (K)

	// Simulation time constants (seconds). Control how fast the sim runs.
	WarmupTau1    float64 // 1st stage warming exp-decay tau (s)
	WarmupTau2    float64 // 2nd stage warming exp-decay tau (s)
	RoughTau      float64 // Roughing pressure exp-decay tau (s)
	CooldownTau1  float64 // 1st stage regen-cooling exp-decay tau (s)
	CooldownTau2  float64 // 2nd stage regen-cooling exp-decay tau (s)
}

// DefaultRegenParams returns regen parameters tuned for a ~5-minute total cycle:
// ~60s warming, ~60s purge, ~60s roughing, ~60s ROR, ~60s cooling.
func DefaultRegenParams() RegenParams {
	return RegenParams{
		WarmupTempK:      310.0,
		ExtPurgeDuration: 60 * time.Second,
		RoughVacuumTorr:  0.050,
		RORLimitMTorrMin: 20.0,
		MaxRORRetries:    3,
		WarmupTimeout:    2 * time.Minute,
		RoughTimeout:     2 * time.Minute,
		CooldownTargetK:  20.0,

		WarmupTau1:   15.0,  // ~60s to reach 310K from 65K
		WarmupTau2:   15.0,  // ~60s to reach 310K from 15K
		RoughTau:     20.0,  // ~60s for pressure 1 Torr → 50 mTorr
		CooldownTau1: 15.0,  // ~60s to cool from 310K → 65K
		CooldownTau2: 15.0,  // ~60s to cool from 310K → 20K
	}
}

// Pump simulates a CTI cryopump with temperature curves.
type Pump struct {
	mu            sync.RWMutex
	state         State
	pumpOnTime    time.Time
	firstStageK   float64
	secondStageK  float64
	lastUpdate    time.Time
	cooldownHours float64
	failRate      float64

	// Operating hours accumulator
	totalOnSeconds float64

	// Valve state
	roughValveOpen bool
	purgeValveOpen bool

	// Regen state
	regenPhase      RegenPhase
	regenPhaseStart time.Time
	regenError      byte    // '@' = no error, 'F' = manual, 'E' = ROR limit, 'B' = warmup timeout, 'G' = rough timeout
	regenRetryCount int     // ROR retry counter
	regenPressure   float64 // Chamber pressure during roughing/ROR (Torr)
	rorStartPressure float64
	rorStartTime    time.Time
	heatersOn       bool
	regenParams     RegenParams
	regenCompleted  bool // Post-regen flag, cleared on next pump-off or pump-on
}

// NewPump creates a pump simulator.
func NewPump(cooldownHours float64, failRate float64) *Pump {
	now := time.Now()
	return &Pump{
		state:         StateOff,
		firstStageK:   295.0, // Room temperature
		secondStageK:  295.0,
		lastUpdate:    now,
		cooldownHours: cooldownHours,
		failRate:      failRate,
		regenError:    '@',
		regenParams:   DefaultRegenParams(),
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
			p.state = StateCooling
			p.pumpOnTime = time.Now()
			p.regenCompleted = false
		}
		return "A", true

	case "pump_off":
		if p.state == StateRegen {
			p.abortRegen('F')
		} else {
			p.state = StateOff
		}
		p.regenCompleted = false
		return "A", true

	case "get_temp_1st_stage":
		return fmt.Sprintf("%.1f", p.firstStageK), true

	case "get_temp_2nd_stage":
		return fmt.Sprintf("%.1f", p.secondStageK), true

	case "get_pump_tc_pressure":
		pressure := p.simulatePressure()
		return fmt.Sprintf("%.2e", pressure), true

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

	default:
		return "?", false
	}
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

// enterPhase transitions to a new regen phase and sets valve/heater state.
func (p *Pump) enterPhase(phase RegenPhase) {
	p.regenPhase = phase
	p.regenPhaseStart = time.Now()

	switch phase {
	case RegenPhaseWarming:
		p.heatersOn = true
		p.purgeValveOpen = true
		p.roughValveOpen = false

	case RegenPhasePurge:
		p.heatersOn = true
		p.purgeValveOpen = true
		p.roughValveOpen = false

	case RegenPhaseRoughing:
		p.purgeValveOpen = false
		p.roughValveOpen = true
		p.heatersOn = true
		p.regenPressure = 1.0 // Start at ~1 Torr

	case RegenPhaseROR:
		p.roughValveOpen = false
		p.purgeValveOpen = false
		p.heatersOn = true
		p.rorStartPressure = p.regenPressure
		p.rorStartTime = time.Now()

	case RegenPhaseCooling:
		p.heatersOn = false
		p.roughValveOpen = false
		p.purgeValveOpen = false
	}
}

// abortRegen stops the regen cycle with an error code and transitions to StateOff.
func (p *Pump) abortRegen(errCode byte) {
	p.regenError = errCode
	p.regenPhase = RegenPhaseNone
	p.heatersOn = false
	p.roughValveOpen = false
	p.purgeValveOpen = false
	p.state = StateOff
}

// updateTemperatures simulates temperature changes based on state and elapsed time.
func (p *Pump) updateTemperatures() {
	now := time.Now()
	dt := now.Sub(p.lastUpdate).Seconds()
	p.lastUpdate = now

	if p.state == StateCooling || p.state == StateCold || p.state == StateRegen {
		p.totalOnSeconds += dt
	}

	switch p.state {
	case StateOff:
		// Drift toward room temperature (295K)
		p.firstStageK = driftToward(p.firstStageK, 295.0, dt, 0.01)
		p.secondStageK = driftToward(p.secondStageK, 295.0, dt, 0.005)

	case StateCooling:
		// Exponential decay toward base temperatures
		tau1 := p.cooldownHours * 3600.0 / 4.0
		tau2 := p.cooldownHours * 3600.0 / 3.5

		p.firstStageK = exponentialDecay(p.firstStageK, 65.0, dt, tau1)
		p.secondStageK = exponentialDecay(p.secondStageK, 15.0, dt, tau2)

		// Add small noise
		p.firstStageK += (rand.Float64() - 0.5) * 0.2
		p.secondStageK += (rand.Float64() - 0.5) * 0.1

		// Transition to cold when close enough
		if p.firstStageK < 70.0 && p.secondStageK < 20.0 {
			p.state = StateCold
		}

	case StateCold:
		// Stable at base temperatures with small fluctuations
		p.firstStageK = 65.0 + (rand.Float64()-0.5)*2.0
		p.secondStageK = 15.0 + (rand.Float64()-0.5)*1.0

	case StateRegen:
		p.updateRegen(now, dt)
	}

	// Clamp temperatures (upper bound 320K allows regen warmup past room temp)
	p.firstStageK = math.Max(10.0, math.Min(320.0, p.firstStageK))
	p.secondStageK = math.Max(4.0, math.Min(320.0, p.secondStageK))
}

// updateRegen runs the phase-specific simulation logic for the regen cycle.
func (p *Pump) updateRegen(now time.Time, dt float64) {
	elapsed := now.Sub(p.regenPhaseStart)

	switch p.regenPhase {
	case RegenPhaseWarming:
		p.firstStageK = exponentialDecay(p.firstStageK, p.regenParams.WarmupTempK, dt, p.regenParams.WarmupTau1)
		p.secondStageK = exponentialDecay(p.secondStageK, p.regenParams.WarmupTempK, dt, p.regenParams.WarmupTau2)

		// Transition when 2nd stage is within 1K of warmup target
		if p.secondStageK >= p.regenParams.WarmupTempK-1.0 {
			p.enterPhase(RegenPhasePurge)
		} else if elapsed >= p.regenParams.WarmupTimeout {
			p.abortRegen('B')
		}

	case RegenPhasePurge:
		// Hold near warmup temp with small fluctuation
		p.firstStageK = p.regenParams.WarmupTempK + (rand.Float64()-0.5)*2.0
		p.secondStageK = p.regenParams.WarmupTempK + (rand.Float64()-0.5)*2.0

		// Transition after extended purge duration
		if elapsed >= p.regenParams.ExtPurgeDuration {
			p.enterPhase(RegenPhaseRoughing)
		}

	case RegenPhaseRoughing:
		// Hold near warmup temp
		p.firstStageK = p.regenParams.WarmupTempK + (rand.Float64()-0.5)*2.0
		p.secondStageK = p.regenParams.WarmupTempK + (rand.Float64()-0.5)*2.0

		p.regenPressure = exponentialDecay(p.regenPressure, 0.001, dt, p.regenParams.RoughTau)

		// Transition when pressure below target
		if p.regenPressure <= p.regenParams.RoughVacuumTorr {
			p.enterPhase(RegenPhaseROR)
		} else if elapsed >= p.regenParams.RoughTimeout {
			p.abortRegen('G')
		}

	case RegenPhaseROR:
		// Hold near warmup temp
		p.firstStageK = p.regenParams.WarmupTempK + (rand.Float64()-0.5)*2.0
		p.secondStageK = p.regenParams.WarmupTempK + (rand.Float64()-0.5)*2.0

		// Pressure rises slowly (~10 mTorr/min simulated)
		riseTorrPerSec := 10.0e-3 / 60.0 // 10 mTorr/min
		p.regenPressure += riseTorrPerSec * dt

		// Evaluate after 1 minute
		rorElapsed := now.Sub(p.rorStartTime)
		if rorElapsed >= time.Minute {
			// Calculate rate in mTorr/min
			minutes := rorElapsed.Minutes()
			rateMTorrMin := (p.regenPressure - p.rorStartPressure) * 1000.0 / minutes

			if rateMTorrMin < p.regenParams.RORLimitMTorrMin {
				// PASS - proceed to cooling
				p.enterPhase(RegenPhaseCooling)
			} else {
				// FAIL - retry
				p.regenRetryCount++
				if p.regenRetryCount >= p.regenParams.MaxRORRetries {
					p.abortRegen('E')
				} else {
					// Loop back to purge phase for retry
					p.enterPhase(RegenPhasePurge)
				}
			}
		}

	case RegenPhaseCooling:
		p.firstStageK = exponentialDecay(p.firstStageK, 65.0, dt, p.regenParams.CooldownTau1)
		p.secondStageK = exponentialDecay(p.secondStageK, 15.0, dt, p.regenParams.CooldownTau2)

		// Add small noise
		p.firstStageK += (rand.Float64() - 0.5) * 0.2
		p.secondStageK += (rand.Float64() - 0.5) * 0.1

		// Transition to StateCooling when cold enough
		if p.secondStageK < p.regenParams.CooldownTargetK {
			p.state = StateCooling
			p.regenPhase = RegenPhaseNone
			p.regenCompleted = true
		}
	}
}

func (p *Pump) simulatePressure() float64 {
	// During roughing/ROR, return the actual chamber pressure being simulated
	if p.state == StateRegen &&
		(p.regenPhase == RegenPhaseRoughing || p.regenPhase == RegenPhaseROR) {
		return p.regenPressure
	}

	// Lower temp = lower pressure (rough simulation)
	avgTemp := (p.firstStageK + p.secondStageK) / 2.0
	if avgTemp < 30 {
		return 1e-8 + rand.Float64()*1e-9
	}
	if avgTemp < 100 {
		return 1e-6 + rand.Float64()*1e-7
	}
	return 1e-3 + rand.Float64()*1e-4
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

	return 'A' // Pump off or normal operation
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
	now := time.Now()

	switch s {
	case StateCooling:
		if p.state == StateOff {
			p.pumpOnTime = now
		}
	case StateRegen:
		if p.state == StateRegen {
			return
		}
		p.startRegen()
		return
	case StateOff:
		if p.state == StateRegen {
			p.abortRegen('F')
			return
		}
		p.regenPhase = RegenPhaseNone
	}

	p.state = s
}

// SetTemperatures overrides first and second stage temperatures.
// Values are clamped to [4, 300].
func (p *Pump) SetTemperatures(firstK, secondK float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.firstStageK = math.Max(4.0, math.Min(320.0, firstK))
	p.secondStageK = math.Max(4.0, math.Min(320.0, secondK))
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

// AdvanceRegenStep advances to the next regen phase, setting temps/pressure
// to make transitions immediate (for console "+" button).
func (p *Pump) AdvanceRegenStep() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != StateRegen {
		return
	}

	switch p.regenPhase {
	case RegenPhaseWarming:
		// Set temps to warmup target to trigger transition
		p.secondStageK = p.regenParams.WarmupTempK
		p.firstStageK = p.regenParams.WarmupTempK
		p.enterPhase(RegenPhasePurge)

	case RegenPhasePurge:
		p.enterPhase(RegenPhaseRoughing)

	case RegenPhaseRoughing:
		// Set pressure below target to trigger transition
		p.regenPressure = p.regenParams.RoughVacuumTorr * 0.5
		p.enterPhase(RegenPhaseROR)

	case RegenPhaseROR:
		// Force pass: set up low rate-of-rise
		p.rorStartPressure = p.regenPressure
		p.enterPhase(RegenPhaseCooling)

	case RegenPhaseCooling:
		// Set temps cold enough to finish
		p.secondStageK = p.regenParams.CooldownTargetK - 1.0
		p.firstStageK = 65.0
		p.state = StateCooling
		p.regenPhase = RegenPhaseNone
		p.regenCompleted = true
	}
}

// exponentialDecay returns the next value decaying toward target.
func exponentialDecay(current, target, dt, tau float64) float64 {
	return target + (current-target)*math.Exp(-dt/tau)
}

// driftToward moves current toward target at the given rate.
func driftToward(current, target, dt, rate float64) float64 {
	diff := target - current
	step := diff * rate * dt
	if math.Abs(step) > math.Abs(diff) {
		return target
	}
	return current + step
}
