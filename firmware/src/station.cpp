#ifdef ARDUINO

#include "station.h"
#include "debug_log.h"
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
    configTime(0, 0, "pool.ntp.org");
    LOG_INFO("MAIN", "NTP sync started");

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

    // 6. First heartbeat (status="starting")
    publishHeartbeat("starting");

    // 7. Watchdog initialized in commTask (must subscribe the feeding task, not setup)

    // 8. Screenshot server (debug tool — disable via config.h)
#ifdef ENABLE_SCREENSHOT_SERVER
    screenshot_server_init();
#endif

    // 9. Log free heap
    LOG_INFO("MAIN", "Boot complete. Free heap: %lu bytes", (unsigned long)ESP.getFreeHeap());

    _lastHeartbeatMs = millis();

    // 10. Create FreeRTOS tasks — all app tasks on Core 1 with LVGL
    //     Core 0: WiFi system tasks only (no application competition for PSRAM DMA)
    //     Core 1: LVGL (priority 10), comm (priority 5), display update (priority 3)
    xTaskCreatePinnedToCore(commTaskEntry, "tComm", 4096, this, 5, nullptr, 1);
    xTaskCreatePinnedToCore(displayTaskEntry, "tDisplay", 3072, this, 3, nullptr, 1);
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

        // Poll for incoming commands — drain all queued commands back-to-back
        if (_cmdHandler && _redisSub.isConnected()) {
            if (_cmdHandler->poll(100)) {
                while (_cmdHandler->poll(1)) {
                    _watchdog.feed();
                }
            }
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

        _display.loop();

        vTaskDelayUntil(&xLastWakeTime, xFrequency);
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

    bool ok = buildHeartbeat(doc, src, uuid, getTimestamp(), data);
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

int64_t Station::getTimestamp() {
    time_t now = time(nullptr);
    if (now > 1700000000) {
        return (int64_t)now;
    }
    return (int64_t)(millis() / 1000);
}

void Station::buildPresenceKey(char* buf, size_t len) {
    snprintf(buf, len, "%s%s%s", PRESENCE_KEY_PREFIX, STATION_INSTANCE, PRESENCE_KEY_SUFFIX);
}

} // namespace arturo

#endif
