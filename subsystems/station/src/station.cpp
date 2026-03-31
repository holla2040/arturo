#ifdef ARDUINO

#include "station.h"
#include "debug_log.h"
#include "time_utils.h"
#include "messaging/envelope.h"
#include "messaging/heartbeat.h"
#include "safety/power_recovery.h"
#include <WiFi.h>
#include <esp_wifi.h>
#include <esp_random.h>
#include <time.h>

namespace arturo {

Station::Station()
    : _redis(REDIS_HOST, REDIS_PORT)
    , _redisSub(REDIS_HOST, REDIS_PORT)
    , _ctiSerial(CTI_UART_NUM)
{}

bool Station::begin() {
    Serial.println();
    Serial.println("============================");
    Serial.println("  Arturo Station v" FIRMWARE_VERSION);
    Serial.println("  Instance: " STATION_INSTANCE);
    Serial.println("============================");
    Serial.println();

    // 0a. Initialize display subsystem (I2C, IO expander, touch, LCD, LVGL)
    //     Must happen early — IO expander controls USB/CAN mux (IO5=0 for USB-CDC)
    _display.begin();

    // 0b. Check boot reason for power failure recovery
    BootReason reason = detectBootReason();
    LOG_INFO("MAIN", "Boot reason: %s", bootReasonToString(reason));
    strncpy(_bootReasonStr, bootReasonToString(reason), sizeof(_bootReasonStr) - 1);
    if (isAbnormalBoot(reason)) {
        LOG_ERROR("MAIN", "Abnormal boot detected — ensuring safe state");
    }

    // 1. Register WiFi event handlers for disconnect/reconnect tracking
    _wifi.registerEvents();

    // 2. WiFi connect (blocks with retry)
    while (!_wifi.connect()) {
        LOG_ERROR("MAIN", "WiFi failed, retrying in 5s...");
        delay(5000);
    }

    // Start NTP sync (non-blocking, runs in background)
    // Controller LAN IP first (always reachable), pool.ntp.org as internet fallback
    configTime(0, 0, NTP_SERVER, "pool.ntp.org");
    // US/Denver timezone for localtime() on station display
    setenv("TZ", "MST7MDT,M3.2.0,M11.1.0", 1);
    tzset();
    LOG_INFO("MAIN", "NTP sync started (server: %s)", NTP_SERVER);

    // 3. Redis connect — both clients
    while (!connectRedis()) {
        LOG_ERROR("MAIN", "Redis (main) failed, retrying in 5s...");
        delay(5000);
    }

    while (!connectRedisSub()) {
        LOG_ERROR("MAIN", "Redis (sub) failed, retrying in 5s...");
        delay(5000);
    }

    // 4. Set presence key
    refreshPresence();

    // 5. Create command handler — sub client for reading, main client for publishing
    static CommandHandler handler(_redisSub, _redis, STATION_INSTANCE);
    _cmdHandler = &handler;

    // 5a. Register OTA update handler
    handler.setOTAHandler(&_otaHandler);

    // 5b. Initialize CTI OnBoard serial port
    if (_ctiSerial.begin(SERIAL_CONFIG_CTI)) {
        LOG_INFO("MAIN", "CTI serial ready: UART%d (default pins)", CTI_UART_NUM);

        if (_ctiDevice.init(_ctiSerial)) {
            handler.setCtiOnBoardDevice(&_ctiDevice);
            LOG_INFO("MAIN", "CTI OnBoard device registered with command handler");
        } else {
            LOG_ERROR("MAIN", "CTI OnBoard device init failed");
        }
    } else {
        LOG_ERROR("MAIN", "CTI serial init failed on UART%d", CTI_UART_NUM);
    }

    // 5c. Create mutexes for shared CTI device and pump telemetry
    _ctiMutex = xSemaphoreCreateMutex();
    _pumpTelemetryMutex = xSemaphoreCreateMutex();
    _testStateMutex = xSemaphoreCreateMutex();

    // 5d. Create local command queue (Display → commTask) and pass to Display
    _localCmdQueue = xQueueCreate(LOCAL_CMD_QUEUE_SIZE, sizeof(LocalCommand));
    _display.setCommandQueue(_localCmdQueue);

    // 6. First heartbeat (status="starting")
    publishHeartbeat("starting");

    // 7. Watchdog initialized in commTask (must subscribe the feeding task, not setup)

    // 8. Screenshot server (debug tool — disable via config.h)
#ifdef ENABLE_SCREENSHOT_SERVER
    screenshot_server_init(&_display);
#endif

    // 9. Log free heap
    LOG_INFO("MAIN", "Boot complete. Free heap: %lu bytes", (unsigned long)ESP.getFreeHeap());

    _lastHeartbeatMs = millis();

    // 10. Create FreeRTOS tasks — all app tasks on Core 1 with LVGL
    //     Core 0: WiFi system tasks only (no application competition for PSRAM DMA)
    //     Core 1: LVGL (priority 10), comm (priority 5), display update (priority 3)
    xTaskCreatePinnedToCore(commTaskEntry, "tComm", 8192, this, 5, nullptr, 1);
    xTaskCreatePinnedToCore(pumpPollTaskEntry, "tPumpPoll", 4096, this, 4, nullptr, 1);
    xTaskCreatePinnedToCore(displayTaskEntry, "tDisplay", 4096, this, 3, nullptr, 1);
#ifdef ENABLE_SCREENSHOT_SERVER
    xTaskCreatePinnedToCore([](void*) {
        for (;;) {
            screenshot_server_update();
            vTaskDelay(pdMS_TO_TICKS(10));
        }
    }, "tScreenshot", 8192, nullptr, 2, nullptr, 1);
#endif

    LOG_INFO("MAIN", "FreeRTOS tasks started — loop() idling");
    return true;
}

void Station::loop() {
    // Empty — all work runs in FreeRTOS tasks (pendant2 pattern).
    // Keeps loop() off the CPU so it can't cause PSRAM bus contention with display DMA.
    vTaskDelay(portMAX_DELAY);
}

// --- FreeRTOS task entry points (static → instance) ---

void Station::commTaskEntry(void* param) {
    static_cast<Station*>(param)->commTask();
}

void Station::displayTaskEntry(void* param) {
    static_cast<Station*>(param)->displayTask();
}

void Station::pumpPollTaskEntry(void* param) {
    static_cast<Station*>(param)->pumpPollTask();
}

// Communication task — Core 1, priority 5
// Handles: watchdog, heartbeat, WiFi/Redis reconnection, command polling
void Station::commTask() {
    // Initialize watchdog HERE so it subscribes this task (not the setup task)
    if (!_watchdog.init()) {
        LOG_ERROR("WDT", "Watchdog init failed — continuing without HW watchdog");
    }

    const TickType_t xFrequency = pdMS_TO_TICKS(100);  // 10 Hz
    TickType_t xLastWakeTime = xTaskGetTickCount();

    for (;;) {
        unsigned long now = millis();

        // Feed watchdog
        if (watchdogIsLateFeed(_watchdog.lastFeedMs(), now, WATCHDOG_LATE_THRESHOLD_MS)) {
            LOG_ERROR("WDT", "Late feed! %lu ms since last feed",
                      watchdogElapsed(_watchdog.lastFeedMs(), now));
        }
        _watchdog.feed();

        // Log once when NTP sync completes
        if (!_ntpSynced && arturo::hasValidTime()) {
            _ntpSynced = true;
            LOG_INFO("NTP", "Time synced: %lld", (long long)arturo::getTimestamp());
        }

        // Heartbeat every HEARTBEAT_INTERVAL_MS
        if (now - _lastHeartbeatMs >= HEARTBEAT_INTERVAL_MS) {
            _lastHeartbeatMs = now;
            if (_wifi.isConnected() && _redis.isConnected()) {
                refreshPresence();
                publishHeartbeat("running");
            }
        }

        // Check WiFi, reconnect if needed
        _wifi.checkAndReconnect();

        // Check main Redis client, reconnect if needed
        if (_wifi.isConnected() && !_redis.isConnected()) {
            LOG_ERROR("MAIN", "Redis (main) disconnected, reconnecting...");
            connectRedis();
        }

        // Check subscribe Redis client, reconnect + re-subscribe if needed
        if (_wifi.isConnected() && !_redisSub.isConnected()) {
            LOG_ERROR("MAIN", "Redis (sub) disconnected, reconnecting...");
            connectRedisSub();
        }

        // Poll for incoming commands — only take mutex when data is waiting
        // Non-blocking socket check prevents starving pumpPollTask
        if (_cmdHandler && _redisSub.available()) {
            if (xSemaphoreTake(_ctiMutex, pdMS_TO_TICKS(50)) == pdTRUE) {
                if (_cmdHandler->poll(100)) {
                    while (_cmdHandler->poll(1)) {
                        _watchdog.feed();
                    }
                }
                xSemaphoreGive(_ctiMutex);
            }
        }

        // Update test state from CommandHandler (set by test.state.update messages)
        if (_cmdHandler) {
            if (xSemaphoreTake(_testStateMutex, pdMS_TO_TICKS(10)) == pdTRUE) {
                _testState = _cmdHandler->testState();
                xSemaphoreGive(_testStateMutex);
            }
        }

        // Drain local command queue (from Display UI controls)
        if (_cmdHandler && _localCmdQueue) {
            LocalCommand cmd;
            bool executed = false;
            while (xQueueReceive(_localCmdQueue, &cmd, 0) == pdTRUE) {
                if (xSemaphoreTake(_ctiMutex, pdMS_TO_TICKS(50)) == pdTRUE) {
                    char resp[256];
                    _cmdHandler->executeLocal(cmd.commandName, resp, sizeof(resp));
                    xSemaphoreGive(_ctiMutex);
                    executed = true;
                }
                _watchdog.feed();
            }
            if (executed) _pollNow = true;  // Trigger immediate poll cycle
        }

        vTaskDelayUntil(&xLastWakeTime, xFrequency);
    }
}

// Display update task — Core 1, priority 3
// Updates status labels once per second. Short mutex holds only.
void Station::displayTask() {
    // Wait 2s for LVGL to fully initialize (pendant2 pattern)
    vTaskDelay(pdMS_TO_TICKS(2000));

    const TickType_t xFrequency = pdMS_TO_TICKS(100);  // 10 Hz for clock
    TickType_t xLastWakeTime = xTaskGetTickCount();

    for (;;) {
        if (_wifi.isConnected()) {
            _display.setWifiStatus(true, WiFi.localIP().toString().c_str(), WiFi.RSSI());
        } else {
            _display.setWifiStatus(false, nullptr, 0);
        }
        _display.setRedisStatus(_redis.isConnected(), REDIS_HOST, REDIS_PORT);

        _display.setSystemStats(
            ESP.getFreeHeap() / 1024,
            ESP.getMinFreeHeap() / 1024,
            ESP.getFreePsram() / 1024,
            millis() / 1000,
            _bootReasonStr,
            _watchdog.resetCount()
        );

        _display.setOpsStats(
            _cmdHandler ? _cmdHandler->commandsProcessed() : 0,
            _cmdHandler ? _cmdHandler->commandsFailed() : 0,
            _ctiDevice.transactionCount(),
            _ctiDevice.errorCount(),
            _heartbeatCount,
            _wifi.reconnectCount(),
            _wifi.totalDisconnectedMs(),
            _redis.reconnectCount()
        );

        // Push pump telemetry snapshot to display
        if (xSemaphoreTake(_pumpTelemetryMutex, pdMS_TO_TICKS(10)) == pdTRUE) {
            _display.setPumpTelemetry(_pumpTelemetry);
            xSemaphoreGive(_pumpTelemetryMutex);
        }

        // Push test state to display
        if (xSemaphoreTake(_testStateMutex, pdMS_TO_TICKS(10)) == pdTRUE) {
            _display.setTestState(_testState);
            xSemaphoreGive(_testStateMutex);
        }

        _display.loop();

        vTaskDelayUntil(&xLastWakeTime, xFrequency);
    }
}

// Pump polling task — Core 1, priority 4
// Cycles through critical CTI queries to populate PumpTelemetry for the display.
// Each CTI command takes ~600ms, so a full cycle is ~4-5 seconds.
void Station::pumpPollTask() {
    // Wait for CTI device to be ready
    vTaskDelay(pdMS_TO_TICKS(3000));

    if (!_ctiDevice.isInitialized()) {
        LOG_ERROR("PUMP_POLL", "CTI device not initialized — task exiting");
        vTaskDelete(nullptr);
        return;
    }

    // Commands to cycle through
    struct PollCmd {
        const char* ctiCmd;
        enum { TEMP1, TEMP2, PRESSURE, STATUS1, ROUGH, PURGE, REGEN } field;
    };

    static const PollCmd cmds[] = {
        {"J",  PollCmd::TEMP1},
        {"K",  PollCmd::TEMP2},
        {"L",  PollCmd::PRESSURE},
        {"S1", PollCmd::STATUS1},
        {"D?", PollCmd::ROUGH},
        {"E?", PollCmd::PURGE},
        {"O",  PollCmd::REGEN},
    };
    static const int cmdCount = sizeof(cmds) / sizeof(cmds[0]);

    char responseBuf[64];
    int cmdIndex = 0;

    for (;;) {
        // Try to acquire CTI mutex — yield if commTask is busy
        if (xSemaphoreTake(_ctiMutex, pdMS_TO_TICKS(200)) != pdTRUE) {
            vTaskDelay(pdMS_TO_TICKS(100));
            continue;
        }

        bool ok = _ctiDevice.executeCommand(cmds[cmdIndex].ctiCmd, responseBuf, sizeof(responseBuf));
        xSemaphoreGive(_ctiMutex);

        if (ok && xSemaphoreTake(_pumpTelemetryMutex, pdMS_TO_TICKS(50)) == pdTRUE) {
            switch (cmds[cmdIndex].field) {
                case PollCmd::TEMP1:
                    _pumpTelemetry.stage1TempK = atof(responseBuf);
                    break;
                case PollCmd::TEMP2:
                    _pumpTelemetry.stage2TempK = atof(responseBuf);
                    break;
                case PollCmd::PRESSURE:
                    _pumpTelemetry.pressureTorr = atof(responseBuf);
                    break;
                case PollCmd::STATUS1:
                    _pumpTelemetry.status1 = (uint8_t)strtoul(responseBuf, nullptr, 16);
                    _pumpTelemetry.pumpOn = (_pumpTelemetry.status1 & 0x01) != 0;
                    break;
                case PollCmd::ROUGH:
                    _pumpTelemetry.roughValveOpen = (responseBuf[0] == '1');
                    break;
                case PollCmd::PURGE:
                    _pumpTelemetry.purgeValveOpen = (responseBuf[0] == '1');
                    break;
                case PollCmd::REGEN:
                    _pumpTelemetry.regenChar = responseBuf[0];
                    break;
            }
            _pumpTelemetry.staleCount = 0;
            _pumpTelemetry.lastUpdateMs = millis();
            xSemaphoreGive(_pumpTelemetryMutex);
        } else if (!ok) {
            // Increment stale count on failure
            if (xSemaphoreTake(_pumpTelemetryMutex, pdMS_TO_TICKS(50)) == pdTRUE) {
                if (_pumpTelemetry.staleCount < 255) _pumpTelemetry.staleCount++;
                xSemaphoreGive(_pumpTelemetryMutex);
            }
        }

        cmdIndex = (cmdIndex + 1) % cmdCount;

        // If a local command just executed, restart cycle immediately for fresh data
        if (_pollNow) {
            _pollNow = false;
            cmdIndex = 0;
        }

        // Yield between commands — executeCommand already blocks for full serial
        // round-trip, so bus is idle when we get here. Just yield for task scheduling.
        vTaskDelay(pdMS_TO_TICKS(20));
    }
}

// --- Private methods ---

bool Station::connectRedis() {
    const char* user = (strlen(REDIS_USERNAME) > 0) ? REDIS_USERNAME : nullptr;
    const char* pass = (strlen(REDIS_PASSWORD) > 0) ? REDIS_PASSWORD : nullptr;
    return _redis.connect(user, pass);
}

bool Station::connectRedisSub() {
    const char* user = (strlen(REDIS_USERNAME) > 0) ? REDIS_USERNAME : nullptr;
    const char* pass = (strlen(REDIS_PASSWORD) > 0) ? REDIS_PASSWORD : nullptr;
    if (!_redisSub.connect(user, pass)) {
        return false;
    }
    char channel[64];
    snprintf(channel, sizeof(channel), "%s%s", CHANNEL_COMMANDS_PREFIX, STATION_INSTANCE);
    if (!_redisSub.subscribe(channel)) {
        LOG_ERROR("MAIN", "Failed to subscribe to %s", channel);
        _redisSub.disconnect();
        return false;
    }
    return true;
}

bool Station::refreshPresence() {
    char key[64];
    buildPresenceKey(key, sizeof(key));
    return _redis.set(key, "online", PRESENCE_TTL_SECONDS);
}

bool Station::publishHeartbeat(const char* status) {
    char uuid[37];
    generateUUID(uuid, sizeof(uuid));

    JsonDocument doc;
    Source src = {STATION_SERVICE, STATION_INSTANCE, STATION_VERSION};

    const char* devices[DEVICE_COUNT + 1];
    const char* deviceTypes[DEVICE_COUNT + 1];
    for (int i = 0; i < DEVICE_COUNT; i++) {
        devices[i] = DEVICE_IDS[i];
        const DeviceInfo* info = getDevice(DEVICE_IDS[i]);
        deviceTypes[i] = (info && info->pumpType) ? info->pumpType : nullptr;
    }
    devices[DEVICE_COUNT] = nullptr;
    deviceTypes[DEVICE_COUNT] = nullptr;

    HeartbeatData data = {};
    data.status = status;
    data.uptimeSeconds = (int64_t)(millis() / 1000);
    data.devices = devices;
    data.deviceCount = DEVICE_COUNT;
    data.deviceTypes = deviceTypes;
    data.freeHeap = (int64_t)ESP.getFreeHeap();
    data.minFreeHeap = (int64_t)ESP.getMinFreeHeap();
    data.wifiRssi = _wifi.rssi();
    data.wifiReconnects = _wifi.reconnectCount();
    data.redisReconnects = _redis.reconnectCount();
    data.commandsProcessed = _cmdHandler ? _cmdHandler->commandsProcessed() : 0;
    data.commandsFailed = _cmdHandler ? _cmdHandler->commandsFailed() : 0;
    data.lastError = nullptr;
    data.watchdogResets = _watchdog.resetCount();
    data.firmwareVersion = FIRMWARE_VERSION;
    data.timeSynced = arturo::hasValidTime();

    bool ok = buildHeartbeat(doc, src, uuid, arturo::getTimestamp(), data);
    if (!ok) {
        LOG_ERROR("HEARTBEAT", "Failed to build heartbeat JSON");
        return false;
    }

    char buffer[768];
    serializeJson(doc, buffer, sizeof(buffer));

    if (!_redis.publish(CHANNEL_HEARTBEAT, buffer)) {
        LOG_ERROR("HEARTBEAT", "Failed to publish heartbeat");
        return false;
    }

    _heartbeatCount++;
    LOG_INFO("HEARTBEAT", "Published heartbeat #%d heap=%luKB",
             _heartbeatCount, (unsigned long)(ESP.getFreeHeap() / 1024));
    return true;
}

void Station::generateUUID(char* buf, size_t len) {
    uint32_t r1 = esp_random();
    uint32_t r2 = esp_random();
    uint32_t r3 = esp_random();
    uint32_t r4 = esp_random();

    snprintf(buf, len, "%08lx-%04lx-4%03lx-%04lx-%04lx%08lx",
             (unsigned long)r1,
             (unsigned long)(r2 >> 16),
             (unsigned long)(r2 & 0x0FFF),
             (unsigned long)(((r3 >> 16) & 0x3FFF) | 0x8000),
             (unsigned long)(r3 & 0xFFFF),
             (unsigned long)r4);
}

void Station::buildPresenceKey(char* buf, size_t len) {
    snprintf(buf, len, "%s%s%s", PRESENCE_KEY_PREFIX, STATION_INSTANCE, PRESENCE_KEY_SUFFIX);
}

} // namespace arturo

#endif
