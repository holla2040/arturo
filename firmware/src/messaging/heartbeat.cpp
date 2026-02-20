#include "heartbeat.h"

namespace arturo {

bool buildHeartbeat(JsonDocument& doc, const Source& source,
                    const char* id, int64_t timestamp,
                    const HeartbeatData& data) {
    if (!buildEnvelope(doc, source, "service.heartbeat", id, timestamp)) {
        return false;
    }

    JsonObject payload = doc["payload"].to<JsonObject>();

    payload["status"] = data.status;
    payload["uptime_seconds"] = data.uptimeSeconds;

    JsonArray devArray = payload["devices"].to<JsonArray>();
    for (int i = 0; i < data.deviceCount; i++) {
        devArray.add(data.devices[i]);
    }

    if (data.deviceTypes) {
        JsonObject typesObj = payload["device_types"].to<JsonObject>();
        for (int i = 0; i < data.deviceCount; i++) {
            if (data.deviceTypes[i]) {
                typesObj[data.devices[i]] = data.deviceTypes[i];
            }
        }
    }

    payload["free_heap"] = data.freeHeap;
    payload["min_free_heap"] = data.minFreeHeap;
    payload["wifi_rssi"] = data.wifiRssi;
    payload["wifi_reconnects"] = data.wifiReconnects;
    payload["redis_reconnects"] = data.redisReconnects;
    payload["commands_processed"] = data.commandsProcessed;
    payload["commands_failed"] = data.commandsFailed;

    if (data.lastError != nullptr) {
        payload["last_error"] = data.lastError;
    } else {
        payload["last_error"] = (const char*)nullptr;
    }

    payload["watchdog_resets"] = data.watchdogResets;
    payload["firmware_version"] = data.firmwareVersion;

    return true;
}

bool parseHeartbeatPayload(JsonObjectConst payload, HeartbeatData& data) {
    if (payload.isNull()) return false;

    if (!payload["status"].is<const char*>()) return false;
    if (!payload["uptime_seconds"].is<int64_t>()) return false;
    if (!payload["devices"].is<JsonArrayConst>()) return false;
    if (!payload["free_heap"].is<int64_t>()) return false;
    if (!payload["wifi_rssi"].is<int>()) return false;
    if (!payload["firmware_version"].is<const char*>()) return false;

    data.status = payload["status"].as<const char*>();
    data.uptimeSeconds = payload["uptime_seconds"].as<int64_t>();

    JsonArrayConst devArray = payload["devices"].as<JsonArrayConst>();
    data.deviceCount = devArray.size();
    // Caller must be aware: devices pointers point into the JsonDocument
    // We store a pointer to the first element's string via static array approach
    // For parsing, we set devices to nullptr — caller should iterate the JSON array directly
    // or use the deviceCount with the original document
    static const char* parsedDevices[16];
    int count = 0;
    for (JsonVariantConst v : devArray) {
        if (count < 15 && v.is<const char*>()) {
            parsedDevices[count++] = v.as<const char*>();
        }
    }
    parsedDevices[count] = nullptr;
    data.devices = parsedDevices;
    data.deviceCount = count;

    data.freeHeap = payload["free_heap"].as<int64_t>();
    data.minFreeHeap = payload["min_free_heap"] | (int64_t)0;
    data.wifiRssi = payload["wifi_rssi"].as<int>();
    data.wifiReconnects = payload["wifi_reconnects"] | 0;
    data.redisReconnects = payload["redis_reconnects"] | 0;
    data.commandsProcessed = payload["commands_processed"] | 0;
    data.commandsFailed = payload["commands_failed"] | 0;

    if (payload["last_error"].isNull()) {
        data.lastError = nullptr;
    } else {
        data.lastError = payload["last_error"].as<const char*>();
    }

    data.watchdogResets = payload["watchdog_resets"] | 0;
    data.firmwareVersion = payload["firmware_version"].as<const char*>();

    return true;
}

} // namespace arturo
