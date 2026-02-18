#pragma once
#include <ArduinoJson.h>
#include "envelope.h"

namespace arturo {

struct HeartbeatData {
    const char* status;       // "starting", "running", "degraded", "stopping"
    int64_t uptimeSeconds;
    const char** devices;     // null-terminated array of device ID strings
    int deviceCount;
    int64_t freeHeap;
    int64_t minFreeHeap;
    int wifiRssi;
    int wifiReconnects;
    int redisReconnects;
    int commandsProcessed;
    int commandsFailed;
    const char* lastError;    // null if no error
    int watchdogResets;
    const char* firmwareVersion;
};

// Build a complete heartbeat message (envelope + payload)
// id and timestamp are passed in for testability
bool buildHeartbeat(JsonDocument& doc, const Source& source,
                    const char* id, int64_t timestamp,
                    const HeartbeatData& data);

// Parse heartbeat payload fields from a message
bool parseHeartbeatPayload(JsonObjectConst payload, HeartbeatData& data);

} // namespace arturo
