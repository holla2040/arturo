# CTI/Brooks Command Reference

Complete command set for Brooks On-Board Cryopump modules. Derived from `~/pendant2/docs/protocol/required-commands.md` and `~/pendant2/PUMP_COMMANDS.md`.

## Command Format

```
Query:   $<command><checksum>\r       → $<code><data><checksum>\r
Set:     $<command><value><checksum>\r → $<code><checksum>\r
```

See [CTI_BROOKS_PROTOCOL.md](CTI_BROOKS_PROTOCOL.md) for wire format details.

---

## Priority Query Commands (0-5)

Polled at high frequency (~150ms cycle). These are the critical real-time parameters.

| # | Command | Description | Response Example | Units |
|---|---------|-------------|------------------|-------|
| 0 | `J` | Stage 1 temperature | `A15.3` | Kelvin |
| 1 | `K` | Stage 2 temperature | `A12.1` | Kelvin |
| 2 | `L` | Pump TC pressure | `A1.2e-5` | Torr |
| 3 | `S1` | System status byte 1 | `A39` | **Hex** |
| 4 | `O` | Regen state | `AW` | See regen states |
| 5 | `Y?` | Operating hours | `A1234` | Hours (0-65535) |

---

## Background Query Commands (6-37)

Polled at lower frequency (every 10-30 seconds).

| # | Command | Description | Response Example |
|---|---------|-------------|------------------|
| 6 | `@` | Module version / software revision | `AVGH4` |
| 7 | `A?` | Pump ON/OFF status | `A1` (1=ON, 0=OFF) |
| 8 | `M` | AUX TC pressure | `A1.5e-6` (Torr) |
| 9 | `Z?` | Regen cycle count | `A127` |
| 10 | `S2` | Status byte 2 | `A01` (**Hex**) |
| 11 | `S3` | Status byte 3 | `A00` (**Hex**) |
| 12 | `W` | Memory error code | `A@` (@=no errors) |
| 13 | `Q` | Rough valve interlock status | `A0` |
| 14 | `a` | Hours since last full regen | `A1234` |
| 15 | `a2` | Hours since last fast regen | `A42` |
| 16 | `b?` | Regen history | `A0` |
| 17 | `c` | Part of regen history (cycles) | `A5` |
| 18 | `k` | Time left in current regen step | `A45` (minutes) |
| 19 | `l` | Failed purge cycles count | `A2` |
| 20 | `m` | Failed rate-of-rise cycles count | `A0` |
| 21 | `n` | Last measured rate of rise | `A2.5` (mTorr/min) |
| 22 | `s` | Regen cycle event count (mod 256) | `A67` |
| 23 | `u` | Pump motor drive failure cause | `A0` (0=OK) |
| 24 | `v` | Regen flag condition (biased by 0x40) | `A40` |
| 25 | `B?` | Pump TC status | `A1` |
| 26 | `C?` | AUX TC status | `A1` |
| 27 | `D?` | Rough valve status | `A0` |
| 28 | `E?` | Purge valve status | `A0` |
| 29 | `F?` | 1st stage heater status | `A1` |
| 30 | `G?` | 2nd stage heater status | `A1` |
| 31 | `H?` | 1st stage temp control setpoint | `A100` (Kelvin) |
| 32 | `I?` | 2nd stage temp control setpoint | `A15` (Kelvin) |
| 33 | `i?` | Power fail recovery status | `A1` |
| 34 | `t?` | Power failure recovery flag | `A0` |
| 35 | `j?` | Pump delay (start of regen) | `A600` (seconds) |
| 36 | `z?` | Keypad lockout state | `A0` |
| 37 | `VA?` | Serial number (first 8 chars) | `ASPUMP001` |

---

## Configuration Commands (Regen Parameters)

Queried on-demand (not continuously polled). Set with value, query with `?`.

| Command | Query | Range | Description | Unit |
|---------|-------|-------|-------------|------|
| `P0` | `P0?` | 0-59994 | Pump restart delay | seconds |
| `P1` | `P1?` | 0-9990 | Extended purge time | seconds |
| `P2` | `P2?` | 0-200 | Repurge cycles | cycles |
| `P3` | `P3?` | 25-200 | Rough to pressure | mTorr |
| `P4` | `P4?` | 1-100 | Rate of rise limit | mTorr/min |
| `P5` | `P5?` | 0-200 | Rate of rise cycles | cycles |
| `P6` | `P6?` | 0-80 | Restart temperature | Kelvin |
| `PA` | `PA?` | 0-1 | Roughing interlock | boolean |
| `PC` | `PC?` | 1-3 | Pumps per compressor | pumps |
| `PG` | `PG?` | 0-9999 | Repurge time | seconds |
| `j` | `j?` | 0-59994 | Start delay | seconds |

**Set example**: `P0600` sets pump restart delay to 600 seconds.
**Query example**: `P0?` returns current pump restart delay.

---

## Control Commands

### Pump ON/OFF

| Command | Description |
|---------|-------------|
| `A0` | Turn pump OFF |
| `A1` | Turn pump ON |
| `A?` | Query pump status |

### Regeneration

| Command | Description |
|---------|-------------|
| `N0` | Abort regen |
| `N1` | Start full regen |
| `N2` | Start fast regen |
| `O` | Query current regen state |
| `e` | Get regen error code (only after `O` returns `V`) |

### Valve Control

| Command | Description |
|---------|-------------|
| `B0` / `B1` / `B?` | Pump TC OFF / ON / Query |
| `C0` / `C1` / `C?` | AUX TC OFF / ON / Query |
| `D0` / `D1` / `D?` | Rough valve CLOSE / OPEN / Query |
| `E0` / `E1` / `E?` | Purge valve CLOSE / OPEN / Query |

### Heater Control (Undocumented)

| Command | Description |
|---------|-------------|
| `F0` / `F1` / `F?` | 1st stage heater OFF / ON / Query |
| `G0` / `G1` / `G?` | 2nd stage heater OFF / ON / Query |

### Temperature Control

| Command | Range | Description |
|---------|-------|-------------|
| `H<val>` | 0-320 | Set 1st stage temp setpoint (0=off), Kelvin |
| `I<val>` | 0-30 | Set 2nd stage temp setpoint (0=off), Kelvin (undocumented) |

### Power Failure Recovery

| Command | Description |
|---------|-------------|
| `i0` | Power fail recovery OFF |
| `i1` | Power fail recovery ON |
| `i2` | Power fail recovery when T2 < limit |
| `i?` | Query PFR status |
| `t?` | Query power failure flag |
| `t=` | Clear power failure flag |

### Gauge Operations

| Command | Description |
|---------|-------------|
| `g` | Autozero pump TC gauge |
| `h` | Autozero AUX TC gauge |

### System Information

| Command | Description |
|---------|-------------|
| `@` | Module type and software revision |
| `VA?` | Serial number (first 8 chars) |
| `VQ?` | Serial number (last 3 chars) |
| `VD?` | Pump name |

### Keypad Lockout

| Command | Description |
|---------|-------------|
| `z0` | Clear keypad lockout |
| `z1` | Set keypad lockout |
| `z?` | Query lockout state |

### Voltage Readings (Undocumented)

| Command | Description |
|---------|-------------|
| `R0` | Pump TC volts |
| `R1` | AUX TC volts |
| `R2` | T1 (stage 1) volts |
| `R3` | T2 (stage 2) volts |

### Dangerous Commands

| Command | Description | WARNING |
|---------|-------------|---------|
| `d` | Erase all regen history | **PERMANENT DATA LOSS** |

---

## Status Byte Bit Definitions

### S1 — System Status (Hex)

```
Bit 0 (0x01) = Pump ON
Bit 1 (0x02) = Rough valve ON
Bit 2 (0x04) = Purge valve ON
Bit 3 (0x08) = Cryo TC ON
Bit 4 (0x10) = AUX TC ON
Bit 5 (0x20) = Power fail status (0=occurred, 1=normal)
```

### S2 — Extended Status (Hex)

```
Bit 0 (0x01) = Setpoint 1 ON
Bit 1 (0x02) = Setpoint 2 ON
Bit 3 (0x08) = 1st stage temp control ON
```

### S3 — Pump Phase (Hex)

```
Bit 0 (0x01) = Pump phase 1 check
Bit 1 (0x02) = Pump phase 2 check
```

---

## Regen States (Response to `O`)

| Response | State |
|----------|-------|
| `A`, `\` | Pump OFF |
| `B`, `C`, `E`, `^`, `]` | Warmup |
| `D`, `F`, `G`, `Q`, `R` | Purge gas failure detected |
| `H` | Extended purge |
| `S` | Repurge cycle |
| `I`, `J`, `K`, `T`, `a`, `b`, `j`, `n` | Rough to base pressure |
| `L` | Rate of rise test |
| `M`, `N`, `c`, `d`, `o` | Cooldown |
| `P` | Regen complete |
| `U` | Beginning of fast regen |
| `V` | **Regen aborted** (query `e` for error code) |
| `W` | Delay restart |
| `X`, `Y` | Power failure |
| `Z` | Delay start |
| `O`, `[` | Zeroing TC gauge |
| `f` | Share regen wait (multi-pump fast regen) |
| `e` | Repurge during fast regen |
| `h` | Purge coordinate wait |
| `i` | Rough coordinate wait |
| `k` | Purge gas fail, recovering |

Some codes have fast-regen-specific variants.

---

## Regen Error Codes (Response to `e`)

Only query after `O` returns `V` (regen aborted).

| Code | Error |
|------|-------|
| `@` | No error |
| `B` | Warmup timeout (>60 min) |
| `C` | Cooldown timeout (>5 hours) |
| `D` | Roughing rate error (dP/dt < 2%/min) |
| `E` | Rate-of-rise cycle limit exceeded |
| `F` | Manual abort |
| `G` | Rough valve timeout (>1 hour) |
| `H` | Illegal state / software bug |
| `I` | Pump too warm for fast regen |
| `J` | 2nd stage too cold to rough |

---

## Power Failure Status (Response to `t?`)

| Value | Meaning |
|-------|---------|
| 0 | No power failure |
| 1 | Was in regen, continuing cooldown |
| 2 | Is in regeneration |
| 3 | Pump ON, attempting recovery to 17K |
| 4 | Recovered from power failure |
| 5 | Did not recover to 17K within 20 min |
| 6 | Above PFR temp setpoint, remains OFF |

Clear with `t=`.

---

## Pump Motor Failure Codes (Response to `u`)

| Response | Meaning |
|----------|---------|
| `0` (0x30) | OK |
| `@` (0x40) | Pump is ON |
| `P` (0x50) | No phase 1 volts |
| `` ` `` (0x60) | No cryo power 2, both phases |
| `p` (0x70) | No phase 2 volts |

---

## Regen Flag Bits (Response to `v`)

Response is biased by 0x40.

```
Bit 0 (0x01) = Waiting for rough valve
Bit 1 (0x02) = Purge gas fail detected
Bit 2 (0x04) = Heater failure detected
Bit 3 (0x08) = Fast regen recovered from purge gas fail
Bit 4 (0x10) = Fast regen reverted to full regen
Bit 5 (0x20) = Fast regen has started
```

---

## Memory Error Code (Response to `W`)

Response `@` = no errors. Otherwise, bit field:

```
Bit 0 = Diode data error
Bit 1 = Regen params error
Bit 2 = History data error
```

---

## Setpoint Relay Programming

Format: `T<relay><selector><value>` or `T<relay><selector>?`

Relays: `1` or `2`

| Selector | Range | Description |
|----------|-------|-------------|
| `0` | 0-9999 | 1st stage lower limit (K) |
| `1` | 0-9999 | 1st stage upper limit (K) |
| `2` | 0-9999 | 2nd stage lower limit (K) |
| `3` | 0-9999 | 2nd stage upper limit (K) |
| `4` | 0-9999 | Pump TC lower limit (microns) |
| `5` | 0-9999 | Pump TC upper limit (microns) |
| `6` | 0-9999 | AUX TC lower limit (microns) |
| `7` | 0-9999 | AUX TC upper limit (microns) |
| `8` | 0-9999 | Time relay (seconds) |
| `A` | — | Unconditional ON |
| `B` | — | Unconditional OFF |
| `C` | — | Regen tracking relay |
| `D` | — | Rough valve tracking relay |
| `F` | — | Pump motor ON/OFF tracking relay |

Example: `T10123` = Set relay 1, stage 1 lower limit to 123K.

---

## Diode Calibration

Format: `U<point><value>` or `U<point>?`

| Point | Temperature | Typical mV |
|-------|-------------|------------|
| `U1` | 11K | ~1400 |
| `U2` | 15K | — |
| `U3` | 17K | — |
| `U4` | 19K | — |
| `U5` | 21K | — |
| `U6` | 25K | — |
| `U7` | 45K | — |
| `U8` | 77.4K | — |
| `U9` | 290K | ~390 |

---

## Safe Testing Order

When first connecting to a pump:

```
1. Read-only queries first:
   @       Module info
   VA?     Serial number
   J       Stage 1 temp
   K       Stage 2 temp
   L       Pressure
   S1      Status byte
   A?      Pump on/off
   Y?      Operating hours

2. Once communication verified, add background queries:
   M, Z?, S2, S3, O, etc.

3. Control commands only after read-only works:
   A1/A0   Pump on/off
   N1/N2   Start regen
```
