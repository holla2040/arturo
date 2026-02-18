#include "envelope.h"
#include <cstring>

namespace arturo {

static const char* SCHEMA_VERSION = "v1.0.0";

static const char* VALID_TYPES[] = {
    "device.command.request",
    "device.command.response",
    "service.heartbeat",
    "system.emergency_stop",
    "system.ota.request"
};

static const int NUM_VALID_TYPES = 5;

bool buildEnvelope(JsonDocument& doc, const Source& source, const char* type,
                   const char* id, int64_t timestamp,
                   const char* correlationId,
                   const char* replyTo) {
    JsonObject envelope = doc["envelope"].to<JsonObject>();

    envelope["id"] = id;
    envelope["timestamp"] = timestamp;

    JsonObject src = envelope["source"].to<JsonObject>();
    src["service"] = source.service;
    src["instance"] = source.instance;
    src["version"] = source.version;

    envelope["schema_version"] = SCHEMA_VERSION;
    envelope["type"] = type;

    if (correlationId != nullptr) {
        envelope["correlation_id"] = correlationId;
    }

    if (replyTo != nullptr) {
        envelope["reply_to"] = replyTo;
    }

    return true;
}

bool parseEnvelope(JsonObjectConst envelope, const char*& id,
                   int64_t& timestamp, const char*& service,
                   const char*& instance, const char*& version,
                   const char*& schemaVersion, const char*& type) {
    if (envelope.isNull()) return false;

    if (!envelope["id"].is<const char*>()) return false;
    if (!envelope["timestamp"].is<int64_t>()) return false;
    if (!envelope["schema_version"].is<const char*>()) return false;
    if (!envelope["type"].is<const char*>()) return false;

    JsonObjectConst src = envelope["source"];
    if (src.isNull()) return false;
    if (!src["service"].is<const char*>()) return false;
    if (!src["instance"].is<const char*>()) return false;
    if (!src["version"].is<const char*>()) return false;

    id = envelope["id"].as<const char*>();
    timestamp = envelope["timestamp"].as<int64_t>();
    schemaVersion = envelope["schema_version"].as<const char*>();
    type = envelope["type"].as<const char*>();
    service = src["service"].as<const char*>();
    instance = src["instance"].as<const char*>();
    version = src["version"].as<const char*>();

    return true;
}

bool validateEnvelopeType(const char* type) {
    if (type == nullptr) return false;

    for (int i = 0; i < NUM_VALID_TYPES; i++) {
        if (strcmp(type, VALID_TYPES[i]) == 0) {
            return true;
        }
    }

    return false;
}

} // namespace arturo
