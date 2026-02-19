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
	StateRegen                // Regeneration cycle (heating up)
)

// Pump simulates a CTI cryopump with temperature curves.
type Pump struct {
	mu            sync.RWMutex
	state         State
	pumpOnTime    time.Time
	regenStart    time.Time
	regenStep     int // 0=not regen, 1-6 = regen steps
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
		}
		return "A", true

	case "pump_off":
		p.state = StateOff
		return "A", true

	case "get_temp_1st_stage":
		return fmt.Sprintf("%.1f", p.firstStageK), true

	case "get_temp_2nd_stage":
		return fmt.Sprintf("%.1f", p.secondStageK), true

	case "get_pump_tc_pressure":
		// Simulate pressure based on temperature
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
		if p.state != StateOff && p.state != StateRegen {
			p.state = StateRegen
			p.regenStart = time.Now()
			p.regenStep = 1
			return "A", true
		}
		return "N", false

	case "abort_regen":
		if p.state == StateRegen {
			p.state = StateOff
			p.regenStep = 0
			return "A", true
		}
		return "N", false

	case "get_regen_step":
		return fmt.Sprintf("%d", p.regenStep), true

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

// updateTemperatures simulates temperature changes based on state and elapsed time.
func (p *Pump) updateTemperatures() {
	now := time.Now()
	dt := now.Sub(p.lastUpdate).Seconds()
	p.lastUpdate = now

	if p.state == StateCooling || p.state == StateCold {
		p.totalOnSeconds += dt
	}

	switch p.state {
	case StateOff:
		// Drift toward room temperature (295K)
		p.firstStageK = driftToward(p.firstStageK, 295.0, dt, 0.01)
		p.secondStageK = driftToward(p.secondStageK, 295.0, dt, 0.005)

	case StateCooling:
		// Exponential decay toward base temperatures
		// 1st stage target: 65K, time constant based on cooldown hours
		// 2nd stage target: 15K, slightly slower
		tau1 := p.cooldownHours * 3600.0 / 4.0 // time constant in seconds
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
		// Heat toward room temp over ~2 hours
		tauRegen := 2.0 * 3600.0 / 3.0
		p.firstStageK = driftToward(p.firstStageK, 295.0, dt, 1.0/tauRegen)
		p.secondStageK = driftToward(p.secondStageK, 295.0, dt, 1.0/tauRegen)

		// Advance regen steps
		elapsed := now.Sub(p.regenStart).Minutes()
		switch {
		case elapsed > 120:
			p.regenStep = 6
			// Regen complete, return to off
			p.state = StateOff
			p.regenStep = 0
		case elapsed > 100:
			p.regenStep = 5
		case elapsed > 80:
			p.regenStep = 4
		case elapsed > 60:
			p.regenStep = 3
		case elapsed > 30:
			p.regenStep = 2
		default:
			p.regenStep = 1
		}
	}

	// Clamp temperatures
	p.firstStageK = math.Max(10.0, math.Min(300.0, p.firstStageK))
	p.secondStageK = math.Max(4.0, math.Min(300.0, p.secondStageK))
}

func (p *Pump) simulatePressure() float64 {
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

func (p *Pump) statusByte1() string {
	// Bit 0: pump on, Bit 1: at temp, Bit 2: regen
	status := 0
	if p.state != StateOff {
		status |= 1
	}
	if p.state == StateCold {
		status |= 2
	}
	if p.state == StateRegen {
		status |= 4
	}
	return fmt.Sprintf("%d", status)
}

func (p *Pump) statusByte2() string {
	return "0"
}

// regenStatusChar returns a CTI-style O-command character for the current regen phase.
func (p *Pump) regenStatusChar() byte {
	if p.state != StateRegen {
		return 'A' // pump off
	}
	switch p.regenStep {
	case 1:
		return 'B' // warmup
	case 2:
		return 'I' // rough to base pressure
	case 3:
		return 'L' // rate of rise
	case 4:
		return 'M' // cooldown
	case 5:
		return 'M' // cooldown
	case 6:
		return 'P' // regen complete
	default:
		return 'A'
	}
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
		RegenStep:      p.regenStep,
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
		p.regenStart = now
		p.regenStep = 1
	case StateOff:
		p.regenStep = 0
	}

	p.state = s
}

// SetTemperatures overrides first and second stage temperatures.
// Values are clamped to [4, 300].
func (p *Pump) SetTemperatures(firstK, secondK float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.firstStageK = math.Max(4.0, math.Min(300.0, firstK))
	p.secondStageK = math.Max(4.0, math.Min(300.0, secondK))
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
