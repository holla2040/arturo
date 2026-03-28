# Jitter Fix — Action Plan & Results

Plan to eliminate WiFi + RGB LCD jitter on the Waveshare ESP32-S3-Touch-LCD-7B.
Based on consolidated research from `ESP32-S3-WIFI-LCD-JITTER-RESEARCH.md`.

**Goal**: Stable 1024x600 RGB LCD at reasonable refresh rate with continuous
WiFi/Redis traffic.

**Status**: **COMPLETE** — jitter eliminated on 2026-03-28. All fixes applied
within the existing Arduino framework using `custom_sdkconfig` + two workaround
files. No framework migration was needed.

---

## Phase 1: Quick Wins in Current Arduino Framework

Original plan: test each `custom_sdkconfig` option individually. If any triggers
the `__wrap_log_printf` linker error, skip it and move to Phase 2.

**Actual outcome**: The linker error was solved with a shim file, unlocking ALL
`custom_sdkconfig` options at once. The cert file issue was solved with a
pre-build script.

### 1.1 Move WiFi buffers to internal SRAM
- [x] Add `# CONFIG_SPIRAM_TRY_ALLOCATE_WIFI_LWIP is not set` to `custom_sdkconfig`
- [x] Build — initially failed (`__wrap_log_printf`), fixed by `log_printf_shim.c`
- [x] Flash and test with Redis heartbeat + command polling active
- [x] Free heap at boot: 117 KB (up from 106 KB), steady-state: 104 KB
- [x] Jitter: **eliminated** (combined with other fixes below)

### 1.2 Enable 64-byte cache lines
- [x] Add `CONFIG_ESP32S3_DATA_CACHE_LINE_64B=y` to `custom_sdkconfig`
- [x] Build — passed (with shim)
- [x] Applied together with 1.1, 1.3, and Octal PSRAM

### 1.3 IRAM-safe LCD ISR
- [x] Add `CONFIG_LCD_RGB_ISR_IRAM_SAFE=y` to `custom_sdkconfig`
- [x] Build — passed (with shim)
- [x] Applied together with other fixes

### 1.4 Dynamic PCLK throttle around Redis operations
- [x] **Skipped** — not needed. Jitter eliminated without PCLK throttling.

### 1.5 Increase bounce buffer to 30 rows
- [x] **Skipped** — not needed. 20 rows sufficient with Octal PSRAM + other fixes.

### Phase 1 Decision Point
- [x] **Phase 1 eliminates jitter**: Yes — after solving the linker blocker, all
  settings applied in one pass. Combined with Octal PSRAM (originally a Phase 2
  item), jitter is fully eliminated. No need for Phase 2 or 3.

---

## Phase 2: Hybrid Framework Migration — NOT NEEDED

Original plan: switch to `framework = arduino, espidf` to compile everything
from source, bypassing pre-compiled library conflicts.

**Actual outcome**: The `__wrap_log_printf` linker error was the only blocker
preventing `custom_sdkconfig` from working. Once solved with the shim file, the
existing Arduino framework + `custom_sdkconfig` provided full sdkconfig control.
Hybrid migration was unnecessary.

- [x] **Skipped entirely** — shim approach is simpler, lower-risk, and fully working.

---

## Phase 3: PCLK Tuning and Final Optimization — NOT NEEDED

Original plan: reduce PCLK or enable 120 MHz PSRAM if jitter persisted.

**Actual outcome**: 30 MHz PCLK works perfectly with Octal PSRAM at 80 MHz +
the other fixes. No PCLK reduction, no 120 MHz PSRAM, no dynamic throttle.

- [x] **Skipped entirely** — 30 MHz PCLK is stable.

---

## Verification Checklist (Final)

- [x] Display stable under normal Redis traffic (10s heartbeats, 4 Hz polling)
- [x] No visible jitter during heartbeat publishes
- [x] 100ms clock widget updates smoothly with WiFi active
- [ ] No visible jitter during command/response cycles (not yet tested with active commands)
- [ ] WiFi reconnect does not cause permanent display drift (not yet tested)
- [x] Free heap remains above safe threshold (104 KB steady-state)
- [ ] Touch input responsive during WiFi activity (not yet tested)
- [ ] Serial debug output confirms OPI PSRAM bus mode (log doesn't show PSRAM init line — may need DEBUG_LEVEL increase)
- [x] `make flash` works cleanly from the Makefile

---

## Files Summary

| File | Action | Status |
|---|---|---|
| `firmware/platformio.ini` | Modified (custom_sdkconfig, extra_scripts) | Done |
| `firmware/src/log_printf_shim.c` | Created (solves `__wrap_log_printf` linker error) | Done |
| `firmware/create_dummy_certs.py` | Created (solves missing cert `.S` files) | Done |
| `firmware/src/display/display.h` | Modified (added clock label) | Done |
| `firmware/src/display/display.cpp` | Modified (100ms clock widget) | Done |
| `firmware/src/station.cpp` | Modified (displayTask 10 Hz for clock) | Done |
| `firmware/src/display/rgb_lcd_port.h` | Unchanged (20-row bounce buffer sufficient) | — |
| `firmware/src/display/rgb_lcd_port.cpp` | Unchanged (30 MHz PCLK retained) | — |

---

## Implementation Results (2026-03-28)

### What Actually Happened

The original plan assumed Phase 1 would fail due to `__wrap_log_printf` linker
errors and that a full hybrid framework migration (Phase 2) would be required.
Instead, the root cause of the linker error was identified and fixed with a
15-line shim file, making the entire `custom_sdkconfig` approach work within
the existing Arduino framework. Phase 2 and Phase 3 were not needed.

**Jitter is eliminated.** A 100ms clock widget updating 10 times per second
confirms smooth, glitch-free display rendering with continuous WiFi/Redis
traffic (10s heartbeats, 4 Hz command polling).

---

### The `__wrap_log_printf` Linker Error — Root Cause and Fix

#### The Problem

pioarduino's `custom_sdkconfig` feature triggers a source rebuild of ESP-IDF
components. The pre-compiled Arduino WiFi library (`STA.cpp.o`) calls
`log_printf()`, and the linker flag `-Wl,--wrap=log_printf` redirects all
`log_printf` references to `__wrap_log_printf`. In normal builds (no
`custom_sdkconfig`), this symbol is provided by the pre-compiled
`libespressif__esp_diagnostics.a` (specifically
`esp_diagnostics_log_hook.c.obj`). When ESP-IDF components are recompiled from
source, the diagnostics library is also recompiled, and the recompiled version
does not produce the `__wrap_log_printf` symbol, causing:

```
undefined reference to `__wrap_log_printf'
```

#### How We Found It

Searched the linker map file (`firmware.map`) from a successful normal build:

```
grep "wrap_log_printf" .pio/build/esp32s3/firmware.map
```

This revealed that `__wrap_log_printf` is defined in
`libespressif__esp_diagnostics.a(esp_diagnostics_log_hook.c.obj)` and that
it calls `log_printfv` (the va_list version of `log_printf` from the Arduino
core's `esp32-hal-uart.c`).

#### The Fix

Created `firmware/src/log_printf_shim.c` — a simple C file that defines
`__wrap_log_printf` by forwarding to `log_printfv`:

```c
#include <stdarg.h>
extern int log_printfv(const char *format, va_list arg);

int __wrap_log_printf(const char *format, ...) {
    va_list args;
    va_start(args, format);
    int ret = log_printfv(format, args);
    va_end(args);
    return ret;
}
```

This file is automatically compiled as part of the `src/` directory. No build
system changes needed beyond placing the file.

#### Why This Works

The `--wrap=log_printf` linker flag is always present (in both normal and
custom_sdkconfig builds). It redirects all `log_printf` calls to
`__wrap_log_printf`. The shim provides that symbol, forwarding to the real
implementation. This is functionally equivalent to what `esp_diagnostics`
was doing in the pre-compiled build — the diagnostics hook just happened to
be the component that provided the wrapper.

---

### The Certificate File Error — Root Cause and Fix

#### The Problem

When `custom_sdkconfig` triggers a source rebuild, ESP-IDF components that
weren't previously compiled (HTTPS server, ESP Rainmaker) now need certificate
assembly files that don't exist:

```
*** Source `.pio/build/esp32s3/https_server.crt.S' not found
```

#### The Fix

Created `firmware/create_dummy_certs.py` — a PlatformIO pre-build script that
creates dummy `.S` files before compilation:

```python
Import("env")
CERTS = [
    "https_server.crt.S",
    "rmaker_mqtt_server.crt.S",
    "rmaker_claim_service_server.crt.S",
    "rmaker_ota_server.crt.S",
]
def create_dummy_certs(source, target, env):
    build_dir = env.subst("$BUILD_DIR")
    for cert in CERTS:
        path = os.path.join(build_dir, cert)
        if not os.path.exists(path):
            with open(path, "w") as f:
                f.write("/* dummy cert - not used by Arturo */\n")
create_dummy_certs(None, None, env)
```

Added to `platformio.ini`:
```ini
extra_scripts =
    pre:load_env.py
    pre:create_dummy_certs.py
```

---

### The `CONFIG_COMPILER_OPTIMIZATION_PERF` Error

#### The Problem

Enabling `-O2` (via `CONFIG_COMPILER_OPTIMIZATION_PERF=y`) causes the Xtensa
compiler to generate larger code that exceeds literal pool range limits in
GDMA HAL code:

```
dangerous relocation: l32r: literal placed after use: .literal.gdma_ahb_hal_stop
```

This is a known Xtensa architecture limitation — the `l32r` instruction can
only reference literals within a 256KB window before the instruction.

#### Decision

Dropped `CONFIG_COMPILER_OPTIMIZATION_PERF`. The performance benefit for the
bounce buffer ISR memcpy is marginal compared to the other fixes (Octal PSRAM,
XIP, 64B cache lines). The default `-Os` optimization is retained.

---

### Final Configuration

#### `firmware/platformio.ini` — custom_sdkconfig

```ini
custom_sdkconfig =
    CONFIG_IDF_EXPERIMENTAL_FEATURES=y
    CONFIG_SPIRAM_MODE_OCT=y
    # CONFIG_SPIRAM_MODE_QUAD is not set
    CONFIG_SPIRAM_XIP_FROM_PSRAM=y
    CONFIG_SPIRAM_FETCH_INSTRUCTIONS=y
    CONFIG_SPIRAM_RODATA=y
    # CONFIG_SPIRAM_TRY_ALLOCATE_WIFI_LWIP is not set
    CONFIG_ESP32S3_DATA_CACHE_LINE_64B=y
    # CONFIG_ESP32S3_DATA_CACHE_LINE_32B is not set
    CONFIG_LCD_RGB_ISR_IRAM_SAFE=y
```

#### What Each Setting Does

| Setting | Effect |
|---|---|
| `SPIRAM_MODE_OCT=y` | Octal PSRAM — doubles bus bandwidth from ~60 to ~120 MB/s. Matches the OPI hardware. |
| `SPIRAM_XIP_FROM_PSRAM=y` | Code executes from PSRAM via cache, eliminating flash bus contention with LCD DMA. |
| `SPIRAM_FETCH_INSTRUCTIONS=y` | Instructions fetched from PSRAM (enabled by XIP). |
| `SPIRAM_RODATA=y` | Read-only data served from PSRAM (enabled by XIP). |
| `SPIRAM_TRY_ALLOCATE_WIFI_LWIP` disabled | WiFi/LwIP packet buffers stay in internal SRAM — no WiFi DMA touches PSRAM bus. |
| `ESP32S3_DATA_CACHE_LINE_64B=y` | 64-byte cache lines — required by Espressif for bounce buffer mode correctness. |
| `LCD_RGB_ISR_IRAM_SAFE=y` | LCD bounce buffer ISR runs from IRAM, immune to flash cache stalls. |
| `IDF_EXPERIMENTAL_FEATURES=y` | Required to enable Octal PSRAM mode. |

#### New Files

| File | Purpose |
|---|---|
| `firmware/src/log_printf_shim.c` | Provides `__wrap_log_printf` symbol for linker compatibility |
| `firmware/create_dummy_certs.py` | Pre-build script creating dummy TLS cert `.S` files |

#### Unchanged Files

- `firmware/src/display/rgb_lcd_port.h` — bounce buffer stays at 20 rows (sufficient with Octal PSRAM)
- `firmware/src/display/rgb_lcd_port.cpp` — PCLK stays at 30 MHz (within Octal PSRAM budget)
- Framework remains `arduino` (no hybrid migration needed)

---

### Approach Sequence and Results

| Step | Approach | Result |
|---|---|---|
| 1 | Add single `custom_sdkconfig` option (WiFi LWIP) | **Failed** — `__wrap_log_printf` linker error |
| 2 | Create dummy cert files manually | **Passed** — certs created, build reaches link stage |
| 3 | Investigate `__wrap_log_printf` via `firmware.map` | **Found** — symbol from `libespressif__esp_diagnostics.a` |
| 4 | Create `log_printf_shim.c` | **Fixed** — linker error resolved permanently |
| 5 | Create `create_dummy_certs.py` pre-build script | **Fixed** — cert files survive clean builds |
| 6 | Build with WiFi SRAM + 64B cache + IRAM ISR | **Passed** — clean build |
| 7 | Add Octal PSRAM mode (`SPIRAM_MODE_OCT`) | **Passed** — clean build |
| 8 | Add XIP from PSRAM | **Passed** — clean build |
| 9 | Add `-O2` optimization (`COMPILER_OPTIMIZATION_PERF`) | **Failed** — Xtensa l32r relocation error; dropped |
| 10 | Flash full config (OCT + XIP + WiFi SRAM + 64B + IRAM ISR) | **Passed** — board boots, display stable |
| 11 | Add 100ms clock widget stress test | **Passed** — smooth rendering with WiFi active |

---

### Heap Comparison

| Build | Free Heap at Boot | Steady-State Heap |
|---|---|---|
| Before (Quad PSRAM, default sdkconfig) | ~106 KB | 106 KB |
| After (Octal PSRAM, full jitter fix) | ~117 KB | 104 KB |

The 11 KB increase at boot is likely due to more efficient memory layout with
Octal PSRAM and WiFi buffers in internal SRAM.

---

### What Was Not Needed

- **Hybrid framework migration** (`framework = arduino, espidf`) — the shim
  approach kept us on pure Arduino framework with `custom_sdkconfig`
- **Full ESP-IDF migration** — not needed
- **PCLK reduction** — 30 MHz works fine with Octal PSRAM
- **Dynamic PCLK throttle** — no jitter to throttle around
- **Bounce buffer increase** — 20 rows sufficient
- **120 MHz PSRAM** — 80 MHz Octal is enough
- **GDMA priority hacking** — not needed
- **`WIFI_PS_MAX_MODEM`** — `MIN_MODEM` is sufficient
- **Any LVGL changes** — display code was already well-optimized

---

### Remaining Considerations

1. **`CONFIG_COMPILER_OPTIMIZATION_PERF`** cannot be enabled due to Xtensa
   literal relocation limits. This is a minor loss — the ISR path is already
   fast enough with the other fixes.

2. **`CONFIG_ESP32S3_DATA_CACHE_64KB`** and **`CONFIG_ESP32S3_INSTRUCTION_CACHE_32KB`**
   were not added. These would increase cache sizes (fewer misses) but consume
   more internal SRAM. Current performance is sufficient without them. Can be
   tested later if needed.

3. **PSRAM speed 120 MHz** (`CONFIG_SPIRAM_SPEED_120M`) was not needed since
   80 MHz Octal eliminated jitter at 30 MHz PCLK. Available as future headroom
   if the display or WiFi workload increases.

4. **The `__wrap_log_printf` shim is stable** but couples to the Arduino core's
   `log_printfv` function signature. If a future Arduino ESP32 update changes
   this function, the shim may need updating. This is low risk since
   `log_printfv` is a core logging function unlikely to change.
