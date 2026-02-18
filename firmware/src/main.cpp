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

// Globals
arturo::WifiManager wifi;
arturo::RedisClient redis(REDIS_HOST, REDIS_PORT);
arturo::CommandHandler* cmdHandler = nullptr;

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

// Connect to Redis with optional AUTH
bool connectRedis() {
    const char* user = (strlen(REDIS_USERNAME) > 0) ? REDIS_USERNAME : nullptr;
    const char* pass = (strlen(REDIS_PASSWORD) > 0) ? REDIS_PASSWORD : nullptr;
    return redis.connect(user, pass);
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
    for (int i = 0; i < DEVICE_COUNT; i++) {
        devices[i] = DEVICE_IDS[i];
    }
    devices[DEVICE_COUNT] = nullptr;

    arturo::HeartbeatData data = {};
    data.status = status;
    data.uptimeSeconds = (int64_t)(millis() / 1000);
    data.devices = devices;
    data.deviceCount = DEVICE_COUNT;
    data.freeHeap = (int64_t)ESP.getFreeHeap();
    data.minFreeHeap = (int64_t)ESP.getMinFreeHeap();
    data.wifiRssi = wifi.rssi();
    data.wifiReconnects = wifi.reconnectCount();
    data.redisReconnects = redis.reconnectCount();
    data.commandsProcessed = cmdHandler ? cmdHandler->commandsProcessed() : 0;
    data.commandsFailed = cmdHandler ? cmdHandler->commandsFailed() : 0;
    data.lastError = nullptr;
    data.watchdogResets = 0;
    data.firmwareVersion = FIRMWARE_VERSION;

    bool ok = arturo::buildHeartbeat(doc, src, uuid, getTimestamp(), data);
    if (!ok) {
        LOG_ERROR("HEARTBEAT", "Failed to build heartbeat JSON");
        return false;
    }

    char buffer[512];
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

    // 1. WiFi connect (blocks with retry)
    while (!wifi.connect()) {
        LOG_ERROR("MAIN", "WiFi failed, retrying in 5s...");
        delay(5000);
    }

    // Start NTP sync (non-blocking, runs in background)
    configTime(0, 0, "pool.ntp.org");
    LOG_INFO("MAIN", "NTP sync started");

    // 2. Redis connect
    while (!connectRedis()) {
        LOG_ERROR("MAIN", "Redis failed, retrying in 5s...");
        delay(5000);
    }

    // 3. Set presence key
    refreshPresence();

    // 4. Create command handler
    static arturo::CommandHandler handler(redis, STATION_INSTANCE);
    cmdHandler = &handler;

    // 5. First heartbeat (status="starting")
    publishHeartbeat("starting");

    // 6. Log free heap
    LOG_INFO("MAIN", "Boot complete. Free heap: %lu bytes", (unsigned long)ESP.getFreeHeap());

    lastHeartbeatMs = millis();
}

void loop() {
    unsigned long now = millis();

    // Heartbeat every 30s
    if (now - lastHeartbeatMs >= HEARTBEAT_INTERVAL_MS) {
        lastHeartbeatMs = now;
        refreshPresence();
        publishHeartbeat("running");
    }

    // Check WiFi, reconnect if needed
    wifi.checkAndReconnect();

    // Check Redis, reconnect if needed
    if (!redis.isConnected()) {
        LOG_ERROR("MAIN", "Redis disconnected, reconnecting...");
        connectRedis();
    }

    // Poll for incoming commands (100ms block inside)
    if (cmdHandler && redis.isConnected()) {
        cmdHandler->poll();
    }

    delay(100);
}
