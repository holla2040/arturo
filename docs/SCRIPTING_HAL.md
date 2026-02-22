# Scripting HAL Reference

Hardware Abstraction Layer (HAL) reference for Arturo script authors. This document defines the abstract command vocabulary available in `.art` scripts — what you can do with each device type, without protocol details.

## How It Works

Scripts use **logical command names** like `pump_on` or `get_temp_1st_stage`. Device profiles (YAML files in `profiles/`) map these names to the actual protocol calls (CTI, SCPI, Modbus, ASCII). Script authors never need to know the wire protocol.

A command in a script looks like:

```
SEND "pump_on"
result = QUERY "get_temp_2nd_stage"
```

The engine resolves `"pump_on"` through the device profile to the correct protocol command for whatever pump hardware is connected to the station.

### Implementation Status

Each command is marked with its current implementation status:

- **Profile** — defined in the device profile YAML (available to scripts once firmware implements it)
- **Firmware** — implemented in station firmware (can talk to real hardware)
- **Mock** — implemented in mock pump simulator (available for development/testing)

---

## Pump (Cryopump)

CTI/Brooks On-Board cryopump. Profile: `profiles/pumps/cti_onboard.yaml`

### Control

| Command | Returns | Description | Status |
|---------|---------|-------------|--------|
| `pump_on` | ack | Turn pump on (starts cooldown) | Profile, Firmware, Mock |
| `pump_off` | ack | Turn pump off (aborts regen if active) | Profile, Firmware, Mock |
| `pump_status` | `"0"` or `"1"` | Pump running status (0=off, 1=on) | Profile, Firmware, Mock |

### Temperature

| Command | Returns | Unit | Description | Status |
|---------|---------|------|-------------|--------|
| `get_temp_1st_stage` | numeric | Kelvin | 1st stage temperature | Profile, Firmware, Mock |
| `get_temp_2nd_stage` | numeric | Kelvin | 2nd stage temperature | Profile, Firmware, Mock |

Typical values: 1st stage ~65K when cold, 2nd stage ~15K when cold, both ~295K at room temperature.

### Pressure

| Command | Returns | Unit | Description | Status |
|---------|---------|------|-------------|--------|
| `get_pump_tc_pressure` | numeric (sci notation) | Torr | Pump thermocouple gauge pressure | Profile, Firmware, Mock |
| `get_aux_tc_pressure` | numeric (sci notation) | Torr | Auxiliary thermocouple gauge pressure | Profile, Firmware |

Typical values: ~1e-8 Torr when cold, ~1e-3 Torr at room temperature.

### Regeneration Control

Regeneration (regen) is the process of warming a cryopump to release trapped gases, then re-cooling it. A full regen cycle proceeds through these phases:

1. **warming** — Heaters on, purge valve open. Pump warms to ~310K.
2. **purge** — Extended nitrogen purge at warm temperature. Drives out trapped gases.
3. **roughing** — Rough valve open, pump down to base vacuum (~50 mTorr).
4. **rate_of_rise** — Valves closed, measure pressure rise. Tests if pump is clean. May retry (back to purge) if rate is too high.
5. **cooling** — Heaters off, pump cools back to operating temperature.

After cooling completes, the pump returns to normal cold operation. A fast regen skips some phases.

| Command | Returns | Description | Status |
|---------|---------|-------------|--------|
| `start_regen` | ack | Start normal (full) regeneration cycle | Profile, Firmware, Mock |
| `start_fast_regen` | ack | Start fast regeneration cycle | Profile, Firmware |
| `abort_regen` | ack | Abort regeneration in progress | Profile, Firmware, Mock |

### Regeneration Status

| Command | Returns | Description | Status |
|---------|---------|-------------|--------|
| `get_regen_step` | integer | Current regen phase number (0=none) | Profile, Firmware, Mock |
| `get_regen_status` | status value | Current regen state (see table below) | Profile, Firmware, Mock |
| `get_regen_error` | error value | Last regen error code (see table below) | Profile, Mock |

#### Regen Status Values

The HAL translates hardware-specific status codes into human-readable names:

| HAL Name | Meaning |
|----------|---------|
| `off` | Pump off, no regen activity |
| `warming` | Phase 1: Heating to warmup temperature |
| `purge` | Phase 2: Extended nitrogen purge |
| `roughing` | Phase 3: Rough pumping to base vacuum |
| `rate_of_rise` | Phase 4: Rate-of-rise leak test |
| `cooling` | Phase 5: Cooldown after successful test |
| `complete` | Regen finished, pump is cold |
| `aborted` | Regen was aborted (check `get_regen_error`) |

#### Regen Error Values

Only meaningful after regen status is `aborted`:

| HAL Name | Meaning |
|----------|---------|
| `none` | No error |
| `warmup_timeout` | Warmup phase exceeded time limit (>60 min) |
| `cooldown_timeout` | Cooldown phase exceeded time limit (>5 hours) |
| `roughing_rate_error` | Roughing pressure decrease too slow (dP/dt < 2%/min) |
| `ror_limit_exceeded` | Rate-of-rise retry limit exceeded |
| `manual_abort` | Operator or script aborted the regen |
| `rough_valve_timeout` | Rough valve operation exceeded time limit (>1 hour) |
| `illegal_state` | Internal state machine error |
| `pump_too_warm` | Pump too warm for fast regen |
| `second_stage_too_cold` | 2nd stage too cold to begin roughing |

### Valve Control

| Command | Returns | Description | Status |
|---------|---------|-------------|--------|
| `open_rough_valve` | ack | Open rough vacuum valve | Profile, Firmware, Mock |
| `close_rough_valve` | ack | Close rough vacuum valve | Profile, Firmware, Mock |
| `get_rough_valve` | `"0"` or `"1"` | Rough valve state (0=closed, 1=open) | Profile, Firmware, Mock |
| `open_purge_valve` | ack | Open nitrogen purge valve | Profile, Firmware, Mock |
| `close_purge_valve` | ack | Close nitrogen purge valve | Profile, Firmware, Mock |
| `get_purge_valve` | `"0"` or `"1"` | Purge valve state (0=closed, 1=open) | Profile, Firmware, Mock |

### Operating Data

| Command | Returns | Unit | Description | Status |
|---------|---------|------|-------------|--------|
| `get_operating_hours` | numeric | hours | Total pump operating hours (0-65535) | Profile, Mock |
| `get_regen_cycles` | integer | count | Total regeneration cycles completed | Profile |
| `get_time_since_regen` | numeric | hours | Hours since last full regeneration | Profile |
| `get_time_since_fast` | numeric | hours | Hours since last fast regeneration | Profile |

### System Information

| Command | Returns | Description | Status |
|---------|---------|-------------|--------|
| `identify` | string | Manufacturer, model, serial, version | Mock |
| `get_module_info` | string | Module type and software revision | Profile |
| `get_serial_number` | string | Serial number (first 8 characters) | Profile |
| `get_serial_suffix` | string | Serial number (last 3 characters) | Profile |

### Power Failure Recovery (PFR)

Controls how the pump behaves after a power interruption.

| Command | Returns | Description | Status |
|---------|---------|-------------|--------|
| `get_pfr_status` | `"0"`, `"1"`, or `"2"` | PFR mode: 0=off, 1=on, 2=temp-conditional | Profile |
| `set_pfr_off` | ack | Disable power failure recovery | Profile |
| `set_pfr_on` | ack | Enable power failure recovery | Profile |
| `set_pfr_temp` | ack | Enable PFR only when 2nd stage below temp limit | Profile |
| `get_pfr_flag` | integer | Power failure status flag (see table below) | Profile |
| `clear_pfr_flag` | ack | Clear the power failure flag | Profile |

#### PFR Flag Values

| Value | Meaning |
|-------|---------|
| `0` | No power failure |
| `1` | Was in regen, continuing cooldown |
| `2` | Is in regeneration |
| `3` | Pump on, attempting recovery to 17K |
| `4` | Recovered from power failure |
| `5` | Did not recover within 20 min |
| `6` | Above PFR temp setpoint, remains off |

### Regen Parameters

Configurable parameters that control regeneration behavior.

| Command | Returns | Unit | Description | Status |
|---------|---------|------|-------------|--------|
| `get_restart_delay` | integer | seconds | Pump restart delay after regen (0-59994) | Profile |
| `set_restart_delay` | ack | seconds | Set pump restart delay | Profile |
| `get_min_run_pressure` | integer | mTorr | Rough base pressure threshold (25-200) | Profile |
| `set_min_run_pressure` | ack | mTorr | Set rough base pressure threshold | Profile |
| `get_min_run_temp` | integer | Kelvin | Power fail recovery temperature (0-80) | Profile |

### Status Bytes

Low-level status bitmasks. Each returns an integer value where individual bits indicate system state.

| Command | Returns | Description | Status |
|---------|---------|-------------|--------|
| `get_status_1` | integer | Primary system status | Profile, Firmware, Mock |
| `get_status_2` | integer | Extended status | Profile, Firmware, Mock |
| `get_status_3` | integer | Pump phase status | Profile, Firmware, Mock |

#### Status 1 Bit Definitions

| Bit | Mask | Meaning |
|-----|------|---------|
| 0 | 0x01 | Pump on |
| 1 | 0x02 | Rough valve open |
| 2 | 0x04 | Purge valve open |
| 3 | 0x08 | Pump thermocouple gauge on |
| 4 | 0x10 | Aux thermocouple gauge on |
| 5 | 0x20 | Power status (0=failure occurred, 1=normal) |

#### Status 2 Bit Definitions

| Bit | Mask | Meaning |
|-----|------|---------|
| 0 | 0x01 | Setpoint relay 1 on |
| 1 | 0x02 | Setpoint relay 2 on |
| 3 | 0x08 | 1st stage temperature control on |

#### Status 3 Bit Definitions

| Bit | Mask | Meaning |
|-----|------|---------|
| 0 | 0x01 | Pump phase 1 check |
| 1 | 0x02 | Pump phase 2 check |

### Diagnostics

| Command | Returns | Description | Status |
|---------|---------|-------------|--------|
| `get_pump_failure` | code | Pump motor drive failure cause (0=OK) | Profile |
| `get_regen_flags` | bitmask | Regen flag conditions (see table below) | Profile |
| `get_memory_error` | code | Memory error code (0=no error) | Profile |

#### Pump Failure Values

| Value | Meaning |
|-------|---------|
| `0` | OK — no failure |
| `1` | Pump is on |
| `2` | No phase 1 voltage |
| `3` | No cryo power, both phases |
| `4` | No phase 2 voltage |

#### Regen Flag Bits

| Bit | Meaning |
|-----|---------|
| 0 | Waiting for rough valve |
| 1 | Purge gas failure detected |
| 2 | Heater failure detected |
| 3 | Fast regen recovered from purge gas failure |
| 4 | Fast regen reverted to full regen |
| 5 | Fast regen has started |

#### Memory Error Bits

| Bit | Meaning |
|-----|---------|
| 0 | Diode calibration data error |
| 1 | Regen parameters error |
| 2 | History data error |

---

## Temperature Controller

Omega CN7500. Profile: `profiles/controllers/omega_cn7500.yaml`

*Placeholder — commands defined in profile, not yet implemented in firmware or mock.*

| Command | Returns | Unit | Description |
|---------|---------|------|-------------|
| `read_temperature` | numeric | (per device config) | Read current process temperature |
| `read_setpoint` | numeric | (per device config) | Read current temperature setpoint |
| `write_setpoint` | ack | (per device config) | Write a new temperature setpoint |

---

## Relay Board

Generic 8-channel USB relay. Profile: `profiles/relays/usb_relay_8ch.yaml`

*Placeholder — commands defined in profile, not yet implemented in firmware or mock.*

| Command | Parameter | Returns | Description |
|---------|-----------|---------|-------------|
| `relay_on` | channel (1-8) | ack | Turn on a specific relay |
| `relay_off` | channel (1-8) | ack | Turn off a specific relay |
| `relay_toggle` | channel (1-8) | ack | Toggle a specific relay |
| `all_on` | — | ack | Turn on all relays |
| `all_off` | — | ack | Turn off all relays |
| `status` | — | bitmask | Get state of all relay channels |
| `identify` | — | string | Get device identification |
| `reset` | — | ack | Reset all relays to default state |

---

## DMM (Digital Multimeter)

Keysight 34461A and Fluke 8846A. Profiles: `profiles/testequipment/keysight_34461a.yaml`, `profiles/testequipment/fluke_8846a.yaml`

*Placeholder — commands defined in profiles, not yet implemented in firmware or mock.*

Both DMM profiles share a common command set:

| Command | Returns | Unit | Description |
|---------|---------|------|-------------|
| `identify` | string | — | Get device identification |
| `reset` | ack | — | Reset to power-on defaults |
| `clear` | ack | — | Clear error queue and status |
| `measure_dc_voltage` | numeric | Volts | Measure DC voltage |
| `measure_ac_voltage` | numeric | Volts | Measure AC voltage |
| `measure_resistance` | numeric | Ohms | Measure resistance |
| `measure_dc_current` | numeric | Amps | Measure DC current |
| `measure_ac_current` | numeric | Amps | Measure AC current |
| `check_errors` | string | — | Check instrument error queue |

The Keysight 34461A additionally supports:

| Command | Returns | Unit | Description |
|---------|---------|------|-------------|
| `measure_frequency` | numeric | Hz | Measure frequency |
| `measure_period` | numeric | seconds | Measure period |
| `self_test` | integer | — | Run self-test (0=pass) |

---

## Power Supply

Rigol DP832. Profile: `profiles/testequipment/rigol_dp832.yaml`

*Placeholder — commands defined in profile, not yet implemented in firmware or mock.*

| Command | Parameter | Returns | Unit | Description |
|---------|-----------|---------|------|-------------|
| `identify` | — | string | — | Get device identification |
| `reset` | — | ack | — | Reset to power-on defaults |
| `clear` | — | ack | — | Clear error queue and status |
| `set_voltage` | value | ack | Volts | Set output voltage |
| `set_current` | value | ack | Amps | Set output current limit |
| `output_on` | — | ack | — | Enable power output |
| `output_off` | — | ack | — | Disable power output |
| `measure_voltage` | — | numeric | Volts | Measure actual output voltage |
| `measure_current` | — | numeric | Amps | Measure actual output current |
| `get_voltage` | — | numeric | Volts | Get configured voltage setpoint |
| `get_current` | — | numeric | Amps | Get configured current limit |
| `check_errors` | — | string | — | Check instrument error queue |
