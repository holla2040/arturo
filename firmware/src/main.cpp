#include <Arduino.h>
#include <WiFi.h>
#include <esp_random.h>
#include <time.h>
#include "config.h"
#include "debug_log.h"
#include "messaging/envelope.h"
#include "messaging/heartbeat.h"
#include "network/wifi_manager.h"
#include "network/redis_client.h"
#include "commands/command_handler.h"
#include "commands/device_registry.h"
#include "devices/serial_device.h"
#include "devices/cti_onboard_device.h"
#include "safety/watchdog.h"
#include "safety/wifi_reconnect.h"
#include "safety/power_recovery.h"
#include "safety/ota_update.h"

// Globals — two Redis clients: one for subscribe, one for everything else
arturo::WifiManager wifi;
arturo::RedisClient redis(REDIS_HOST, REDIS_PORT);
arturo::RedisClient redisSub(REDIS_HOST, REDIS_PORT);
arturo::CommandHandler* cmdHandler = nullptr;
arturo::Watchdog watchdog;

unsigned long lastHeartbeatMs = 0;
int heartbeatCount = 0;

// Generate UUID v4 using hardware RNG
void generateUUID(char* buf, size_t len) {
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

// Get timestamp — epoch seconds from NTP, or millis fallback
int64_t getTimestamp() {
    time_t now = time(nullptr);
    if (now > 1700000000) {
        return (int64_t)now;
    }
    return (int64_t)(millis() / 1000);
}

// Build presence key: "device:{instance}:alive"
void buildPresenceKey(char* buf, size_t len) {
    snprintf(buf, len, "%s%s%s", PRESENCE_KEY_PREFIX, STATION_INSTANCE, PRESENCE_KEY_SUFFIX);
}

// Connect main Redis client (for publish, SET, etc.)
bool connectRedis() {
    const char* user = (strlen(REDIS_USERNAME) > 0) ? REDIS_USERNAME : nullptr;
    const char* pass = (strlen(REDIS_PASSWORD) > 0) ? REDIS_PASSWORD : nullptr;
    return redis.connect(user, pass);
}

// Connect subscribe Redis client and subscribe to command channel
bool connectRedisSub() {
    const char* user = (strlen(REDIS_USERNAME) > 0) ? REDIS_USERNAME : nullptr;
    const char* pass = (strlen(REDIS_PASSWORD) > 0) ? REDIS_PASSWORD : nullptr;
    if (!redisSub.connect(user, pass)) {
        return false;
    }
    char channel[64];
    snprintf(channel, sizeof(channel), "%s%s", CHANNEL_COMMANDS_PREFIX, STATION_INSTANCE);
    if (!redisSub.subscribe(channel)) {
        LOG_ERROR("MAIN", "Failed to subscribe to %s", channel);
        redisSub.disconnect();
        return false;
    }
    return true;
}

// Refresh presence key with TTL
bool refreshPresence() {
    char key[64];
    buildPresenceKey(key, sizeof(key));
    return redis.set(key, "online", PRESENCE_TTL_SECONDS);
}

// Build and publish a heartbeat message
bool publishHeartbeat(const char* status) {
    char uuid[37];
    generateUUID(uuid, sizeof(uuid));

    JsonDocument doc;
    arturo::Source src = {STATION_SERVICE, STATION_INSTANCE, STATION_VERSION};

    const char* devices[DEVICE_COUNT + 1];
    const char* deviceTypes[DEVICE_COUNT + 1];
    for (int i = 0; i < DEVICE_COUNT; i++) {
        devices[i] = DEVICE_IDS[i];
        const arturo::DeviceInfo* info = arturo::getDevice(DEVICE_IDS[i]);
        deviceTypes[i] = (info && info->pumpType) ? info->pumpType : nullptr;
    }
    devices[DEVICE_COUNT] = nullptr;
    deviceTypes[DEVICE_COUNT] = nullptr;

    arturo::HeartbeatData data = {};
    data.status = status;
    data.uptimeSeconds = (int64_t)(millis() / 1000);
    data.devices = devices;
    data.deviceCount = DEVICE_COUNT;
    data.deviceTypes = deviceTypes;
    data.freeHeap = (int64_t)ESP.getFreeHeap();
    data.minFreeHeap = (int64_t)ESP.getMinFreeHeap();
    data.wifiRssi = wifi.rssi();
    data.wifiReconnects = wifi.reconnectCount();
    data.redisReconnects = redis.reconnectCount();
    data.commandsProcessed = cmdHandler ? cmdHandler->commandsProcessed() : 0;
    data.commandsFailed = cmdHandler ? cmdHandler->commandsFailed() : 0;
    data.lastError = nullptr;
    data.watchdogResets = watchdog.resetCount();
    data.firmwareVersion = FIRMWARE_VERSION;

    bool ok = arturo::buildHeartbeat(doc, src, uuid, getTimestamp(), data);
    if (!ok) {
        LOG_ERROR("HEARTBEAT", "Failed to build heartbeat JSON");
        return false;
    }

    char buffer[768];
    serializeJson(doc, buffer, sizeof(buffer));

    if (!redis.publish(CHANNEL_HEARTBEAT, buffer)) {
        LOG_ERROR("HEARTBEAT", "Failed to publish heartbeat");
        return false;
    }

    heartbeatCount++;
    LOG_INFO("HEARTBEAT", "Published heartbeat #%d heap=%luKB",
             heartbeatCount, (unsigned long)(ESP.getFreeHeap() / 1024));
    return true;
}

void setup() {
    Serial.begin(115200);
    delay(2000);

    Serial.println();
    Serial.println("============================");
    Serial.println("  Arturo Station v" FIRMWARE_VERSION);
    Serial.println("  Instance: " STATION_INSTANCE);
    Serial.println("============================");
    Serial.println();

    // 0. Check boot reason for power failure recovery
    arturo::BootReason reason = arturo::detectBootReason();
    LOG_INFO("MAIN", "Boot reason: %s", arturo::bootReasonToString(reason));
    if (arturo::isAbnormalBoot(reason)) {
        LOG_ERROR("MAIN", "Abnormal boot detected — ensuring safe state");
    }

    // 1. Register WiFi event handlers for disconnect/reconnect tracking
    wifi.registerEvents();

    // 2. WiFi connect (blocks with retry)
    while (!wifi.connect()) {
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
    static arturo::CommandHandler handler(redisSub, redis, STATION_INSTANCE);
    cmdHandler = &handler;

    // 5a. Register OTA update handler
    static arturo::OTAUpdateHandler otaHandler;
    handler.setOTAHandler(&otaHandler);

    // 5b. Initialize CTI OnBoard serial port (UART1, 2400 7E1 via MAX3232)
    static arturo::SerialDevice ctiSerial(CTI_UART_NUM);
    if (ctiSerial.begin(arturo::SERIAL_CONFIG_CTI, CTI_RX_PIN, CTI_TX_PIN)) {
        LOG_INFO("MAIN", "CTI serial ready: UART%d, pins RX=%d TX=%d",
                 CTI_UART_NUM, CTI_RX_PIN, CTI_TX_PIN);

        static arturo::CtiOnBoardDevice ctiOnBoardDevice;
        if (ctiOnBoardDevice.init(ctiSerial)) {
            handler.setCtiOnBoardDevice(&ctiOnBoardDevice);
            LOG_INFO("MAIN", "CTI OnBoard device registered with command handler");
        } else {
            LOG_ERROR("MAIN", "CTI OnBoard device init failed");
        }
    } else {
        LOG_ERROR("MAIN", "CTI serial init failed on UART%d", CTI_UART_NUM);
    }

    // 6. First heartbeat (status="starting", include boot reason)
    publishHeartbeat("starting");

    // 7. Initialize hardware watchdog (8s timeout, fed every 4s from loop)
    if (!watchdog.init()) {
        LOG_ERROR("MAIN", "Watchdog init failed — continuing without HW watchdog");
    }

    // 8. Log free heap
    LOG_INFO("MAIN", "Boot complete. Free heap: %lu bytes", (unsigned long)ESP.getFreeHeap());

    lastHeartbeatMs = millis();
}

void loop() {
    unsigned long now = millis();

    // Feed watchdog — must happen every loop iteration to prevent reset
    if (arturo::watchdogIsLateFeed(watchdog.lastFeedMs(), now,
                                    arturo::WATCHDOG_LATE_THRESHOLD_MS)) {
        LOG_ERROR("WDT", "Late feed! %lu ms since last feed",
                  arturo::watchdogElapsed(watchdog.lastFeedMs(), now));
    }
    watchdog.feed();

    // Heartbeat every 30s
    if (now - lastHeartbeatMs >= HEARTBEAT_INTERVAL_MS) {
        lastHeartbeatMs = now;
        if (wifi.isConnected() && redis.isConnected()) {
            refreshPresence();
            publishHeartbeat("running");
        }
    }

    // Check WiFi, reconnect if needed
    wifi.checkAndReconnect();

    // Check main Redis client, reconnect if needed
    if (wifi.isConnected() && !redis.isConnected()) {
        LOG_ERROR("MAIN", "Redis (main) disconnected, reconnecting...");
        connectRedis();
    }

    // Check subscribe Redis client, reconnect + re-subscribe if needed
    if (wifi.isConnected() && !redisSub.isConnected()) {
        LOG_ERROR("MAIN", "Redis (sub) disconnected, reconnecting...");
        connectRedisSub();
    }

    // Poll for incoming commands — drain all queued commands back-to-back
    if (cmdHandler && redisSub.isConnected()) {
        if (cmdHandler->poll(100)) {
            // Got one — drain remaining without blocking
            while (cmdHandler->poll(1)) {
                watchdog.feed();
            }
        }
    }

    delay(10);
}
