# ESP32-S3 WiFi + RGB LCD Jitter — Research & Fix Guide

Complete reference for the PSRAM bus contention problem between WiFi DMA and
RGB LCD DMA on the ESP32-S3. Written from two projects (pendant2, Arturo) that
hit the same issue on the same board. The pendant2 project solved it by
eliminating WiFi. Arturo requires WiFi, so the problem remains partially open.

**Board**: Waveshare ESP32-S3-Touch-LCD-7B
**Display**: 7" 1024x600 RGB LCD, 16-bit RGB565, 30 MHz pixel clock
**Touch**: GT911 capacitive (I2C)
**MCU**: ESP32-S3 dual-core 240 MHz, 16 MB QIO flash, 8 MB OPI PSRAM
**Framework**: Arduino ESP32 3.3.7 (pioarduino), ESP-IDF 5.5.0/5.5.2
**LVGL**: 8.4.0

---

## 1. The Hardware Problem

The RGB LCD peripheral continuously DMAs the **entire framebuffer** (1,228,800
bytes = 1024 x 600 x 2) from PSRAM to the LCD panel at ~60 Hz. This is
~73 MB/sec of uninterruptible PSRAM bus traffic.

WiFi also uses DMA for packet TX/RX, and WiFi buffers are allocated in PSRAM
by default (`CONFIG_SPIRAM_TRY_ALLOCATE_WIFI_LWIP=y`). Every WiFi operation
generates PSRAM DMA bursts that collide with the display DMA.

The ESP32-S3 OPI PSRAM theoretical bandwidth is ~120 MB/sec. Display alone
uses ~73 MB/sec (61%). WiFi adds variable load. CPU access to PSRAM
(framebuffer rendering, data structures) adds more. The bus is near capacity
even without WiFi.

**Key insight**: This is a hardware-level DMA bus contention problem. No amount
of FreeRTOS mutex locking, task priority tuning, or LVGL optimization can
prevent two DMA engines from colliding on the same physical bus. Software
mitigations reduce the *frequency* of collisions but cannot eliminate them.

### Bounce Buffer Mechanism

The ESP-IDF RGB LCD driver uses bounce buffers to mitigate PSRAM contention:
- Two small bounce buffers allocated in internal SRAM
- CPU copies PSRAM framebuffer data → internal SRAM bounce buffer (via DCache)
- LCD DMA reads from internal SRAM (no PSRAM contention for LCD output)
- The `on_bounce_frame_finish` ISR callback triggers the next copy

The bottleneck moves from "LCD DMA vs WiFi DMA on PSRAM bus" to "CPU PSRAM
copy in ISR vs WiFi DMA on PSRAM bus." The ISR copy can be delayed by:
- WiFi DMA monopolizing PSRAM bus
- Flash cache misses stalling the CPU
- Other ISRs or high-priority tasks preempting the bounce buffer ISR

---

## 2. What pendant2 Did (No WiFi — Full Fix)

pendant2 is a pump HMI on the same board. It communicates with its pump via
UART serial, not WiFi. The only WiFi feature was a screenshot server for
debugging, which was disabled as the "critical fix."

### 7 Fixes Applied (October 2025)

| # | Fix | Commit | Impact |
|---|-----|--------|--------|
| 1 | Reduce chart update frequency (4 Hz → 30s) | `80840c9` | Fewer LVGL renders |
| 2 | Only update active tab (`lv_tabview_get_tab_act`) | `0b2c0b4` | 5x CPU reduction |
| 3 | LVGL refresh 16ms/60fps, touch polling 5ms/200Hz | `5535e12` | Smoother rendering |
| 4 | Fixed positioning instead of flex layout | `9c4d108` | Zero layout recalc |
| 5 | **Disable screenshot server (WiFi)** | `9a309ff` | **Critical fix** |
| 6 | Timestamp-protected switch widgets | `3d72e52` | No stale-state blink |
| 7 | LVGL mutex timeout 10ms → 100ms | documented | Fewer skipped updates |

**Fix #5 was the critical one.** With WiFi disabled, all other fixes reduced
rendering overhead but the screen was already stable without WiFi traffic.

### pendant2 Working Configuration

| Parameter | Value | File |
|-----------|-------|------|
| Avoid-tear mode | 3 (direct + double-buffer) | `lvgl_port.h` |
| Display refresh | 16 ms (60 fps) | `lv_conf.h` |
| Touch polling | 5 ms (200 Hz) | `lv_conf.h` |
| LVGL task priority | 10 (highest) | `lvgl_port.h` |
| LVGL task core | Core 1 | `lvgl_port.h` |
| LVGL min/max delay | 1 ms / 500 ms | `lvgl_port.h` |
| Perf monitor | Disabled | `lv_conf.h` |
| Theme transitions | 0 ms | `lv_conf.h` |
| Framebuffers | 2x in PSRAM | `lvgl_port.h` |
| Bounce buffer | 10,240 px (10 rows) | `rgb_lcd_port.h` |
| WiFi traffic | **None** | — |

### pendant2 Task Layout

| Priority | Task | Core |
|----------|------|------|
| 10 | LVGL | Core 1 |
| 5 | Proxy/comms (UART serial) | Any |
| 3 | UI update | Core 0 |
| 2 | Heartbeat | Any |
| 1 | loop() (idle) | Core 1 |

---

## 3. What Arturo Did (WiFi Required — Partial Fix)

Arturo is an industrial test automation system that requires WiFi for Redis
communication. The jumpy screen appeared as soon as WiFi + Redis were active.

### Fixes Applied

All pendant2 LVGL/display fixes were ported:

| Fix | Before | After |
|-----|--------|-------|
| Label positioning | `lv_obj_center()`, `lv_obj_align()` | `lv_obj_set_pos()` |
| Lock timeout | 50 ms | 100 ms |
| Label updates | Every call | strcmp guard, skip if unchanged |
| RSSI in status text | Yes (changed every second) | Removed |
| Perf monitor | Enabled | Disabled |
| Theme transitions | 80 ms | 0 ms |
| loop() | 100 Hz with all work | `vTaskDelay(portMAX_DELAY)` |
| Task structure | Everything in loop() | FreeRTOS tasks |
| LVGL task core | Core 0 | Core 1 |

WiFi-specific mitigations:

| Fix | Before | After | Effect |
|-----|--------|-------|--------|
| Heartbeat interval | 3 s | 10 s | 70% fewer Redis bursts |
| Comm poll rate | 10 Hz | 4 Hz | 60% fewer WiFi socket reads |
| WiFi modem sleep | `WIFI_PS_NONE` | `WIFI_PS_MIN_MODEM` | Radio sleeps between AP beacons (~100ms), eliminates idle WiFi DMA |
| Bounce buffer | 10 rows | 20 rows | More headroom for PSRAM copy ISR |
| Watchdog | Subscribed in setup() | Subscribed in commTask | Fix: feed from correct FreeRTOS task |

### Arturo Task Layout (Final)

| Priority | Task | Core |
|----------|------|------|
| 10 | LVGL | Core 1 |
| 5 | commTask (Redis, watchdog, heartbeat) | Core 1 |
| 3 | displayTask (1 Hz label updates) | Core 1 |
| 1 | loop() (idle) | Core 1 |
| — | WiFi system tasks | Core 0 |

Core 0 is reserved exclusively for WiFi system tasks. All application tasks
on Core 1 with LVGL at highest priority.

### Result

Jitter reduced from constant to occasional glitches. Not eliminated. The
remaining glitches correlate with WiFi traffic (heartbeat publishes, Redis
poll responses).

---

## 4. The Misconfiguration Discovery

Investigation of the pre-compiled framework defaults revealed the PSRAM is
running at **half its available bandwidth**.

### Current sdkconfig (Pre-compiled Arduino ESP32 3.3.7)

```
CONFIG_SPIRAM_MODE_QUAD=y              # !! OPI hardware running in Quad mode
# CONFIG_SPIRAM_MODE_OCT is not set    # !! Should be OCT for 8MB OPI PSRAM
CONFIG_ESP32S3_DATA_CACHE_LINE_32B=y   # !! Should be 64B for bounce buffers
# CONFIG_ESP32S3_DATA_CACHE_LINE_64B is not set
# CONFIG_SPIRAM_XIP_FROM_PSRAM is not set   # !! Code runs from flash, stalls CPU
# CONFIG_SPIRAM_FETCH_INSTRUCTIONS is not set
# CONFIG_SPIRAM_RODATA is not set
CONFIG_SPIRAM_TRY_ALLOCATE_WIFI_LWIP=y # !! WiFi buffers in PSRAM = more contention
# CONFIG_LCD_RGB_ISR_IRAM_SAFE is not set   # !! LCD ISR not protected from flash ops
CONFIG_COMPILER_OPTIMIZATION_SIZE=y    # -Os, not -O2
CONFIG_LCD_RGB_RESTART_IN_VSYNC=y      # OK — safety net for DMA desync
CONFIG_ESP32S3_DATA_CACHE_SIZE=0x8000  # 32 KB
CONFIG_ESP32S3_INSTRUCTION_CACHE_SIZE=0x4000  # 16 KB
```

The board JSON (`boards/waveshare_esp32s3_touch_lcd_7b.json`) correctly
specifies `"memory_type": "qio_opi"` (QIO flash + OPI PSRAM), but the
pre-compiled framework libraries ignore this and use Quad PSRAM mode.

### What Should Be Set

| Option | Correct Value | Why |
|--------|--------------|-----|
| `CONFIG_SPIRAM_MODE_OCT=y` | **Enable** | OPI PSRAM in Octal mode = ~2x bandwidth |
| `CONFIG_SPIRAM_XIP_FROM_PSRAM=y` | **Enable** | Execute code from PSRAM, free flash bus |
| `CONFIG_SPIRAM_FETCH_INSTRUCTIONS=y` | **Enable** | (Enabled by XIP) |
| `CONFIG_SPIRAM_RODATA=y` | **Enable** | (Enabled by XIP) |
| `CONFIG_ESP32S3_DATA_CACHE_LINE_64B=y` | **Enable** | Larger cache lines = efficient PSRAM→SRAM copy |
| `CONFIG_ESP32S3_DATA_CACHE_64KB=y` | **Enable** | Fewer cache misses |
| `CONFIG_ESP32S3_INSTRUCTION_CACHE_32KB=y` | **Enable** | Fewer instruction fetches |
| `CONFIG_LCD_RGB_ISR_IRAM_SAFE=y` | **Enable** | LCD ISR runs even during flash ops |
| `CONFIG_SPIRAM_TRY_ALLOCATE_WIFI_LWIP=n` | **Disable** | WiFi buffers in internal SRAM, not PSRAM |
| `CONFIG_COMPILER_OPTIMIZATION_PERF=y` | **Enable** | Faster memcpy in bounce buffer ISR |
| `CONFIG_IDF_EXPERIMENTAL_FEATURES=y` | **Enable** | Required for OCT PSRAM mode |

### Why This Matters

Switching from Quad to Octal PSRAM mode roughly doubles available bandwidth:
- Quad mode: ~60 MB/sec usable
- Octal mode: ~120 MB/sec usable

With 73 MB/sec used by display, Quad mode leaves only ~87 MB/sec headroom (if
we're lucky — actual usable bandwidth is lower due to access patterns). Octal
mode would leave ~47 MB/sec headroom, which should comfortably accommodate
WiFi DMA bursts.

Disabling WiFi PSRAM allocation (`SPIRAM_TRY_ALLOCATE_WIFI_LWIP=n`) moves
WiFi packet buffers to internal SRAM, eliminating PSRAM bus contention from
WiFi DMA entirely. The tradeoff is using ~50 KB of internal SRAM for WiFi
buffers.

XIP from PSRAM (`SPIRAM_XIP_FROM_PSRAM=y`) eliminates flash cache misses that
stall the CPU during the bounce buffer ISR. Without XIP, a flash cache miss
during the ISR can delay the PSRAM→SRAM copy long enough for the LCD DMA to
exhaust the bounce buffer, causing a visible glitch.

---

## 5. Attempted Fix: pioarduino custom_sdkconfig

pioarduino (the PlatformIO ESP32 platform) supports `custom_sdkconfig` in
`platformio.ini` for Arduino framework projects. This triggers a source rebuild
of all ESP-IDF components with the custom settings.

### What We Tried

```ini
[env:esp32s3]
; ... existing settings ...
custom_sdkconfig =
    CONFIG_IDF_EXPERIMENTAL_FEATURES=y
    CONFIG_SPIRAM_MODE_OCT=y
    # CONFIG_SPIRAM_MODE_QUAD is not set
    CONFIG_SPIRAM_XIP_FROM_PSRAM=y
    CONFIG_SPIRAM_FETCH_INSTRUCTIONS=y
    CONFIG_SPIRAM_RODATA=y
    CONFIG_ESP32S3_DATA_CACHE_LINE_64B=y
    # CONFIG_ESP32S3_DATA_CACHE_LINE_32B is not set
    CONFIG_ESP32S3_DATA_CACHE_64KB=y
    CONFIG_ESP32S3_INSTRUCTION_CACHE_32KB=y
    CONFIG_LCD_RGB_ISR_IRAM_SAFE=y
    # CONFIG_SPIRAM_TRY_ALLOCATE_WIFI_LWIP is not set
    CONFIG_COMPILER_OPTIMIZATION_PERF=y
    # CONFIG_COMPILER_OPTIMIZATION_SIZE is not set
```

### Blockers Encountered

#### Blocker 1: Missing Certificate Files

The source rebuild compiles ESP-IDF components that weren't needed with
pre-compiled libs. The HTTPS server and ESP Rainmaker components require
certificate `.S` assembly files that don't exist:

```
*** Source `.pio/build/esp32s3/https_server.crt.S' not found
*** Source `.pio/build/esp32s3/rmaker_mqtt_server.crt.S' not found
*** Source `.pio/build/esp32s3/rmaker_claim_service_server.crt.S' not found
*** Source `.pio/build/esp32s3/rmaker_ota_server.crt.S' not found
```

Creating dummy files (`echo "/* dummy */" > file.S`) gets past this, but the
build system deletes them during cmake configure, so they must be recreated
between configure and build phases.

#### Blocker 2: `__wrap_log_printf` Linker Error

The pre-compiled Arduino WiFi library (`STA.cpp.o`) references
`__wrap_log_printf`, a linker-level wrapper defined in ESP-IDF. When ESP-IDF
components are recompiled with different settings, the wrapper definition
changes or disappears:

```
undefined reference to `__wrap_log_printf'
```

This happens even without `CONFIG_COMPILER_OPTIMIZATION_PERF` — the XIP and
cache settings alone are enough to trigger it. The pre-compiled Arduino `.a`
files (WiFi, BT, etc.) were compiled against a specific ESP-IDF configuration,
and any change that affects symbol wrapping breaks the link.

#### Minimal Config Also Fails

Even with just two options:
```ini
custom_sdkconfig =
    CONFIG_IDF_EXPERIMENTAL_FEATURES=y
    CONFIG_SPIRAM_MODE_OCT=y
    # CONFIG_SPIRAM_MODE_QUAD is not set
    CONFIG_LCD_RGB_ISR_IRAM_SAFE=y
```

The `__wrap_log_printf` error persists. Changing the PSRAM mode apparently
affects enough of the ESP-IDF internals to break compatibility with pre-compiled
Arduino libraries.

### Possible Solutions (Not Yet Attempted)

1. **Switch from Arduino to ESP-IDF framework** — gives full sdkconfig control
   without pre-compiled library compatibility issues. Requires rewriting
   Arduino-specific code (WiFi, Serial, etc.) to use ESP-IDF APIs.

2. **Build Arduino ESP32 from source** — clone arduino-esp32, modify sdkconfig,
   rebuild all libraries. Point PlatformIO at the local build.

3. **Use `framework = espidf, arduino`** — PlatformIO's hybrid mode that
   compiles ESP-IDF from source while providing Arduino compatibility. May
   resolve the wrapper issue since everything is compiled together.

4. **File pioarduino issue** — the `custom_sdkconfig` feature should handle
   the cert generation and wrapper compatibility automatically. This may be
   a known bug.

5. **Use ESP-IDF component manager** — replace just the lcd and spiram
   components with correctly configured ones, leaving the rest pre-compiled.

6. **Patch the pre-compiled `.a` files** — extract, modify, re-archive the
   specific libraries. Fragile but avoids full rebuild.

---

## 6. Other Avenues to Investigate

### ESP-IDF RGB LCD APIs (5.5.0+)

- **`dma_burst_size` field** — replaces deprecated `sram_trans_align` /
  `psram_trans_align` in `esp_lcd_rgb_panel_config_t`. Set to 64 for optimal
  alignment with 64-byte cache lines.

- **`refresh_on_demand` flag** — defers LCD refresh to explicit calls. Could
  batch display updates to known-quiet WiFi windows. Risk: visible refresh
  pauses.

- **`esp_lcd_rgb_panel_set_pclk()`** — dynamic pixel clock adjustment. Lower
  PCLK during WiFi-heavy operations, raise it afterward. Reduces PSRAM
  bandwidth requirement proportionally.

- **`CONFIG_LCD_RGB_RESTART_IN_VSYNC`** — already enabled. Auto-restarts DMA
  during VBlank if it gets desynchronized. Safety net, not a fix.

### GDMA Priority

There is **no public API** in ESP-IDF 5.x to set GDMA channel priority for
the LCD peripheral. The `rx_weight`/`tx_weight` registers exist in hardware
but are managed internally by the LCD driver. Would need to patch the driver
or use HAL-level register writes.

### Bounce Buffer Size

Current: 10 rows (20,480 bytes per buffer, two buffers = 40 KB internal SRAM)
Changed to: 20 rows (40,960 bytes per buffer, two buffers = 82 KB internal SRAM)

Larger bounce buffers give the ISR more time to complete the PSRAM→SRAM copy
before the LCD exhausts the other buffer. Each additional 10 rows uses ~20 KB
more internal SRAM. The ESP32-S3 has 320 KB total SRAM, so 82 KB for bounce
buffers is significant but feasible.

Worth testing: 30 rows, 40 rows. Monitor free heap to ensure other subsystems
aren't starved.

### WiFi Modem Sleep

`WIFI_PS_MIN_MODEM` (currently enabled) puts the radio to sleep between DTIM
beacons (~100 ms). This eliminates idle WiFi DMA but adds up to one beacon
interval of latency on incoming data.

`WIFI_PS_MAX_MODEM` sleeps more aggressively. Could further reduce DMA
contention at the cost of higher latency.

**Important**: Modem sleep must be enabled AFTER all initial connections are
established. Enabling it during Redis SUBSCRIBE causes 2-second response
timeouts.

### SPIRAM Speed

`CONFIG_SPIRAM_SPEED_120M=y` — runs PSRAM at 120 MHz instead of 80 MHz.
Marked as experimental, requires `CONFIG_IDF_EXPERIMENTAL_FEATURES=y`. Could
increase bandwidth by 50%, but has temperature stability requirements.

### Alternative Display Approaches

- **LVGL full-refresh mode (Mode 1)** instead of direct mode (Mode 3) — the
  LCD driver handles buffer swaps internally, may have different PSRAM access
  patterns that are more tolerant of contention.

- **Reduce pixel clock** — lowering from 30 MHz to 20 MHz reduces display
  bandwidth requirement by 33% but also reduces refresh rate to ~40 Hz.

- **Use DPI (MIPI-DPI) interface** if hardware supports it — different DMA
  path that may have better arbitration.

---

## 7. Rules for New Code (from pendant2, applies to Arturo)

These rules come from hard-won experience. Violating any of them reintroduces
jitter on this hardware.

1. **Never poll in `loop()` at high frequency.** If `loop()` runs faster than
   ~100 ms, it competes with display DMA. Move work to FreeRTOS tasks.

2. **Never run WiFi-heavy operations on Core 0.** WiFi system tasks already
   run on Core 0. Additional WiFi-triggering work starves them.

3. **Only update visible content.** Use `lv_tabview_get_tab_act()` or
   equivalent to skip updates for invisible tabs/screens.

4. **Use fixed positioning (`lv_obj_set_pos`).** Avoid flex/grid layouts for
   status displays that update frequently.

5. **Protect interactive widgets from stale data.** Any widget the user can
   toggle needs a cooldown window before accepting external state updates.

6. **LVGL task at highest FreeRTOS priority.** Nothing should preempt display
   rendering.

7. **Keep lock hold times short.** Acquire LVGL mutex, update widgets, release.
   Never do I/O while holding the lock.

8. **Don't call `lv_chart_refresh()` manually.** LVGL auto-invalidates on
   widget value changes.

9. **Test with WiFi active.** Jitter that doesn't appear without WiFi will
   appear in production.

10. **Disable `LV_USE_PERF_MONITOR`.** Creates a permanent dirty area that
    forces re-rendering every frame.

11. **Minimize WiFi traffic frequency.** Every Redis operation = WiFi DMA burst
    = PSRAM contention spike. Batch operations, use longer intervals.

12. **Enable WiFi modem sleep.** Eliminates idle WiFi DMA between transactions.

---

## 8. File Inventory

### Arturo

| File | What |
|------|------|
| `firmware/platformio.ini` | Build config, sdkconfig TODO |
| `firmware/boards/waveshare_esp32s3_touch_lcd_7b.json` | Board def (`qio_opi`) |
| `firmware/src/display/lvgl_port.h` | LVGL task params, avoid-tear mode |
| `firmware/src/display/lvgl_port.cpp` | Flush callback, vsync, mutex |
| `firmware/src/display/rgb_lcd_port.h` | LCD timing, bounce buffer size |
| `firmware/src/display/rgb_lcd_port.cpp` | RGB panel init, vsync callbacks |
| `firmware/src/display/display.cpp` | Display class, label management |
| `firmware/src/lv_conf.h` | LVGL config (refresh rate, perf monitor) |
| `firmware/src/station.cpp` | FreeRTOS tasks, modem sleep, watchdog |
| `firmware/src/config.h` | Heartbeat interval |
| `firmware/src/network/wifi_manager.cpp` | WiFi connect, modem sleep |

### pendant2 (Reference)

| File | What |
|------|------|
| `~/pendant2/DISPLAY-LIMITATIONS.md` | Original jitter documentation |
| `~/pendant2/src/firmware/src/drivers/lvgl_port.h` | LVGL port config |
| `~/pendant2/src/firmware/src/drivers/lvgl_port.cpp` | Flush callbacks |
| `~/pendant2/src/firmware/src/drivers/rgb_lcd_port.cpp` | RGB panel init |
| `~/pendant2/src/firmware/lv_conf.h` | LVGL config |
| `~/pendant2/src/firmware/firmware.ino` | Task layout |
| `~/pendant2/src/firmware/src/config.h` | Screenshot server toggle |

### Framework Defaults (Read-Only Reference)

| File | What |
|------|------|
| `~/.platformio/packages/framework-arduinoespressif32-libs/esp32s3/sdkconfig` | Pre-compiled defaults |
| `~/.platformio/packages/framework-arduinoespressif32-libs/esp32s3/include/esp_lcd/rgb/include/esp_lcd_panel_rgb.h` | RGB panel API |

---

## 9. Key Commits (Arturo)

| Hash | Description |
|------|-------------|
| `b4ae62a` | Reduce display jitter (LVGL fixes, FreeRTOS tasks, poll rates) |
| `0a11fa7` | WiFi modem sleep, remove RSSI from display |
| `31444a8` | Bounce buffer 10→20 rows, document sdkconfig fix |

### Key Commits (pendant2 — Reference)

| Hash | Date | Fix |
|------|------|-----|
| `80840c9` | 2025-10-19 | Reduce chart update frequency |
| `0b2c0b4` | 2025-10-20 | Active-tab-only rendering |
| `5535e12` | 2025-10-20 | Optimize refresh rate and touch polling |
| `3d72e52` | 2025-10-20 | Timestamp-protected switch widgets |
| `9c4d108` | 2025-10-26 | Fixed positioning instead of flex |
| `9a309ff` | 2025-10-27 | Disable screenshot server (critical fix) |
| `26eab4a` | 2025-10-16 | WiFi-DMA interference research |

---

## 10. Build Warning: ESP-IDF Source Rebuild Requires Certs

Any approach that rebuilds ESP-IDF from source (custom_sdkconfig, hybrid
framework, full IDF migration) will hit missing certificate files. The HTTPS
server and ESP Rainmaker components embed TLS certificates as assembly files
during the build. These files don't exist in the default project:

```
.pio/build/esp32s3/https_server.crt.S
.pio/build/esp32s3/rmaker_mqtt_server.crt.S
.pio/build/esp32s3/rmaker_claim_service_server.crt.S
.pio/build/esp32s3/rmaker_ota_server.crt.S
```

Creating dummy files works (`echo "/* dummy */" > file.S`) but the cmake
configure step deletes them. A pre-build script or cmake hook is needed to
regenerate them between configure and compile phases. Alternatively, disable
the HTTPS server and Rainmaker components via sdkconfig if they aren't needed.

---

## 11. The Core Question for Further Research

The ESP32-S3 is one of the most popular embedded WiFi chips. Thousands of
products use it with RGB LCDs and WiFi simultaneously. There **must** be a
proven configuration or technique that makes this work. Specific questions:

1. **Is `CONFIG_SPIRAM_MODE_OCT` the missing piece?** If the PSRAM is running
   in Quad mode (half bandwidth), switching to Octal mode could provide enough
   headroom for WiFi + LCD coexistence. The pioarduino `custom_sdkconfig` build
   fails — is there another way to set this?

2. **Does `CONFIG_SPIRAM_TRY_ALLOCATE_WIFI_LWIP=n` eliminate WiFi DMA on the
   PSRAM bus?** If WiFi buffers move to internal SRAM, WiFi DMA should stop
   touching PSRAM entirely. This alone might fix the jitter.

3. **What do commercial products (smart displays, HMI panels) do?** Espressif
   sells the ESP32-S3-LCD-EV-Board and similar dev kits with RGB LCD + WiFi.
   What sdkconfig do they ship? Check `esp-idf/examples/peripherals/lcd/` and
   `espressif/esp-bsp` repositories for reference configurations.

4. **Is the `framework = espidf, arduino` hybrid mode a viable path?** This
   compiles everything from source, avoiding pre-compiled library conflicts.
   What's the migration cost?

5. **Can the bounce buffer ISR be made faster?** IRAM placement
   (`CONFIG_LCD_RGB_ISR_IRAM_SAFE`), larger cache lines, `-O2` optimization —
   do these measurably reduce the ISR execution time?

6. **Is there an ESP-IDF GDMA priority mechanism** (even undocumented) that
   gives LCD DMA precedence over WiFi DMA?

7. **Does ESP-IDF 5.5+ have any new LCD+WiFi coexistence features** not
   documented in earlier versions?
