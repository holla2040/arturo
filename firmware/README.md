# Arturo ESP32 Firmware

## Overview

ESP32 station firmware that bridges physical instruments to Redis over WiFi. Each station handles one or more devices (SCPI instruments, serial devices, relays, Modbus) and communicates with the controller via Redis Streams and Pub/Sub.

## Technical Decisions

### Arduino Framework (not pure ESP-IDF)

We use the **Arduino framework** for ESP32 development. Arduino on ESP32 runs on top of ESP-IDF, which means we get:

- Simple APIs for WiFi, GPIO, Serial, SPI, I2C
- Direct access to ESP-IDF functions when needed (OTA, NVS, partitions)
- Large library ecosystem (ArduinoJson, etc.)
- Lower learning curve than pure ESP-IDF

We call ESP-IDF functions directly from Arduino code where needed:

```cpp
// Arduino for simple things
WiFi.begin(ssid, password);
Serial.println("connected");
digitalWrite(RELAY_PIN, HIGH);

// ESP-IDF directly for advanced things (same project)
#include <esp_ota_ops.h>
#include <esp_partition.h>
esp_ota_handle_t ota_handle;
const esp_partition_t *update = esp_ota_get_next_update_partition(NULL);
esp_ota_begin(update, OTA_SIZE_UNKNOWN, &ota_handle);
```

There is no need to switch to pure ESP-IDF. The OTA dual-partition rollback is a bootloader feature, not a framework feature. We get it for free with the right partition table.

### Arduino CLI (not the IDE)

We use the **Arduino CLI** for building and flashing. No IDE dependency.

```bash
# Build
arduino-cli compile --fqbn esp32:esp32:esp32s3 firmware/

# Flash
arduino-cli upload --fqbn esp32:esp32:esp32s3 --port /dev/ttyUSB0 firmware/

# Serial monitor
arduino-cli monitor --port /dev/ttyUSB0 --config baudrate=115200
```

### FreeRTOS Task Architecture

Arduino on ESP32 runs on FreeRTOS. The `setup()` and `loop()` functions execute on a FreeRTOS task pinned to Core 1. We create additional FreeRTOS tasks to keep the firmware modular and non-blocking.

**Task layout:**

| Task | Core | Priority | Stack | What it does |
|------|------|----------|-------|-------------|
| `heartbeatTask` | 0 | 1 (low) | 4 KB | Publish heartbeat every 30s, refresh presence key |
| `commandTask` | 0 | 2 (medium) | 8 KB | XREAD from Redis stream, dispatch commands |
| `deviceTask` | 1 | 2 (medium) | 4 KB | SCPI/serial/GPIO command execution |
| `watchdogTask` | 0 | 3 (high) | 2 KB | Feed hardware watchdog, check E-stop GPIO |
| `loop()` | 1 | 1 (low) | default | WiFi/Redis reconnection, status LED |

**Core assignment strategy:**
- **Core 0**: Network (Redis, WiFi, heartbeat). These tasks share the network stack.
- **Core 1**: Hardware I/O (device communication, GPIO). These tasks talk to physical instruments.

**Core assignment strategy:**
- **Core 0**: Network (Redis, WiFi, heartbeat). These tasks share the network stack.
- **Core 1**: Hardware I/O (device communication, GPIO). These tasks talk to physical instruments.

Network and hardware don't block each other.

**Task creation in setup():**

```cpp
void setup() {
    Serial.begin(115200);
    initGPIO();
    connectWiFi();
    connectRedis();

    xTaskCreatePinnedToCore(heartbeatTask, "heartbeat", 4096, NULL, 1, NULL, 0);
    xTaskCreatePinnedToCore(commandTask,   "commands",  8192, NULL, 2, NULL, 0);
    xTaskCreatePinnedToCore(deviceTask,    "device",    4096, NULL, 2, NULL, 1);
    xTaskCreatePinnedToCore(watchdogTask,  "watchdog",  2048, NULL, 3, NULL, 0);
}

void loop() {
    // WiFi/Redis reconnection if disconnected
    // Status LED update
    vTaskDelay(pdMS_TO_TICKS(100));
}
```

**Task communication** uses FreeRTOS queues between `commandTask` (receives from Redis) and `deviceTask` (executes on hardware):

```cpp
QueueHandle_t commandQueue = xQueueCreate(10, sizeof(DeviceCommand));

// commandTask receives from Redis, pushes to queue
void commandTask(void *param) {
    for (;;) {
        // XREAD BLOCK from Redis stream
        DeviceCommand cmd = parseCommand(redisMessage);
        xQueueSend(commandQueue, &cmd, portMAX_DELAY);
    }
}

// deviceTask reads from queue, executes on hardware
void deviceTask(void *param) {
    DeviceCommand cmd;
    for (;;) {
        if (xQueueReceive(commandQueue, &cmd, portMAX_DELAY)) {
            DeviceResponse resp = executeCommand(cmd);
            publishResponse(resp);  // XADD to Redis response stream
        }
    }
}
```

### OTA Firmware Updates

ESP32 has dual OTA partitions built into the bootloader. Partition A runs while B gets written. On reboot, it swaps. If the new firmware fails to connect to Redis within 30 seconds, the bootloader rolls back automatically.

**Update flow:**

1. Controller sends `system.ota.request` to station via Redis Stream
2. Station downloads `.bin` from controller over HTTP (`http://192.168.1.10:8080/firmware/arturo-relay-v1.1.0.bin`)
3. ESP32 writes to inactive partition using ESP-IDF OTA API
4. ESP32 verifies SHA256 checksum
5. ESP32 reboots to new partition
6. New firmware sends heartbeat with updated version
7. If firmware crashes or can't reach Redis, watchdog triggers rollback to previous partition

**OTA command message:**

```json
{
  "envelope": { "type": "system.ota.request" },
  "payload": {
    "firmware_url": "http://192.168.1.10:8080/firmware/arturo-relay-v1.1.0.bin",
    "version": "1.1.0",
    "sha256": "abc123...",
    "force": false
  }
}
```

The ESP32 checks: is `version` newer than what I'm running? Does `sha256` match after download? If yes, flash and reboot. If `force: true`, skip the version check.

### Serial Debug Output

Every station prints structured debug logs to USB serial at 115200 baud.

**Debug levels (compile-time in config.h):**

```cpp
#define DEBUG_LEVEL_NONE   0   // Production: no serial output
#define DEBUG_LEVEL_ERROR  1   // Errors only
#define DEBUG_LEVEL_INFO   2   // Lifecycle + commands + errors
#define DEBUG_LEVEL_DEBUG  3   // Everything including raw bytes
#define DEBUG_LEVEL_TRACE  4   // Hex dumps of SCPI/Modbus wire traffic
```

**Example output at INFO level:**

```
[12:04:00.000] [WIFI] Connected to "arturo-lab" rssi=-42
[12:04:00.150] [REDIS] Connected to 192.168.1.10:6379
[12:04:00.160] [REDIS] XREAD BLOCK commands:relay-board-01
[12:04:01.123] [CMD] Received: device.command.request corr=a1b2c3
[12:04:01.125] [RELAY] GPIO 17 -> HIGH (channel 3 ON)
[12:04:01.126] [CMD] Response: success=true duration=2ms
[12:04:30.000] [HEARTBEAT] Published #2 heap=244KB rssi=-42
```

**At TRACE level, raw wire traffic is visible:**

```
[12:04:05.002] [SCPI] TX >>> "MEAS:VOLT:DC?\n" (15 bytes)
[12:04:05.002] [SCPI] TX hex: 4d 45 41 53 3a 56 4f 4c 54 3a 44 43 3f 0a
[12:04:05.045] [SCPI] RX <<< "+1.23456789E+00\n" (17 bytes)
[12:04:05.045] [SCPI] RX hex: 2b 31 2e 32 33 34 35 36 37 38 39 45 2b 30 30 0a
```

## Hardware

| Variant | Board | Use Case | Interfaces |
|---------|-------|----------|-----------|
| TCP bridge | ESP32-S3 + W5500 Ethernet | SCPI instruments | Ethernet to instrument, WiFi to Redis |
| Serial bridge | ESP32-S3 + MAX3232 or MAX485 | Serial devices | UART to instrument, WiFi to Redis |
| Relay controller | ESP32-S3 + relay board | Power switching | GPIO to relays, WiFi to Redis |
| E-stop station | ESP32-S3 + button + LED | Safety | GPIO button/LED, WiFi to Redis |

ESP32-S3 for all variants: dual-core 240MHz, 512KB SRAM, WiFi, plenty of GPIO.

## Memory Budget (ESP32-S3, 512KB SRAM)

| Component | Estimate | Notes |
|-----------|----------|-------|
| FreeRTOS + WiFi | ~80 KB | Fixed overhead |
| Redis client + buffers | ~20 KB | RESP protocol, 2KB tx/rx buffers |
| ArduinoJson | ~10 KB | Static document, 4KB buffer |
| FreeRTOS tasks (5 tasks) | ~22 KB | Stack allocations from table above |
| Packetizer | ~5 KB | One active protocol |
| Application logic | ~15 KB | Command dispatch, state, queues |
| **Total** | **~152 KB** | Leaves ~360KB headroom |

## Source Structure

```
firmware/
├── src/
│   ├── main.cpp                    # setup() + loop(), task creation
│   ├── config.h                    # WiFi, Redis IP, instance ID, debug level
│   │
│   ├── network/
│   │   ├── wifi_manager.cpp        # Connect, reconnect, exponential backoff
│   │   └── redis_client.cpp        # XREAD, XADD, PUBLISH, SUBSCRIBE
│   │
│   ├── messaging/
│   │   ├── envelope.cpp            # Build/parse Protocol v1.0.0 JSON envelopes
│   │   ├── command_handler.cpp     # Parse commands, push to FreeRTOS queue
│   │   └── heartbeat.cpp           # 30-second heartbeat with diagnostics
│   │
│   ├── protocols/
│   │   ├── packetizer.h            # Abstract interface
│   │   ├── scpi.cpp                # SCPI command/response
│   │   ├── modbus.cpp              # Modbus RTU over RS485
│   │   ├── cti.cpp                 # CTI cryopump protocol
│   │   └── ascii.cpp               # Generic ASCII text protocol
│   │
│   ├── devices/
│   │   ├── tcp_device.cpp          # TCP socket to SCPI instruments
│   │   ├── serial_device.cpp       # HardwareSerial to UART instruments
│   │   ├── relay_controller.cpp    # GPIO relay control
│   │   └── modbus_device.cpp       # RS485 Modbus RTU
│   │
│   └── safety/
│       ├── watchdog.cpp            # Hardware watchdog, feeds from watchdogTask
│       ├── estop.cpp               # E-stop GPIO, local power cut
│       └── interlock.cpp           # Local safety checks
│
└── test/
    ├── test_envelope.cpp
    └── test_scpi.cpp
```

## Boot Sequence

```
1. Init GPIO (E-stop button, relays to safe state, status LED)
2. Init hardware watchdog (8-second timeout)
3. Load config from NVS (WiFi creds, Redis IP, instance ID, device profile)
4. Connect WiFi (retry with exponential backoff, status LED blinks)
5. Connect Redis (retry with backoff)
6. Set presence key: SET device:{instance}:alive EX 90
7. Create FreeRTOS tasks (heartbeat, commands, device, watchdog)
8. Publish first heartbeat (acts as service.started announcement)
9. Main loop: handle WiFi/Redis reconnection, update status LED
```

## Redis Communication

Each station gets its own command stream. No shared channels for commands.

```
commands:relay-board-01      <- only relay-board-01 reads this (Stream)
commands:dmm-station-01      <- only dmm-station-01 reads this (Stream)
events:heartbeat             <- all stations publish here (Pub/Sub)
events:emergency_stop        <- all stations subscribe and publish (Pub/Sub)
device:{instance}:alive      <- presence key with 90s TTL
```

## Heartbeat Diagnostics

Every heartbeat includes fields for monitoring station health:

```json
{
  "payload": {
    "status": "running",
    "uptime_seconds": 3600,
    "devices": ["fluke-8846a"],
    "free_heap": 245000,
    "min_free_heap": 180000,
    "wifi_rssi": -42,
    "wifi_reconnects": 0,
    "redis_reconnects": 0,
    "commands_processed": 1547,
    "commands_failed": 3,
    "last_error": "E_DEVICE_TIMEOUT on fluke-8846a at 12:03:45",
    "watchdog_resets": 0,
    "firmware_version": "1.0.0"
  }
}
```
