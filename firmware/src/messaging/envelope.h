#pragma once
#include <ArduinoJson.h>

namespace arturo {

struct Source {
    const char* service;
    const char* instance;
    const char* version;
};

// Build a complete envelope JsonObject inside the given document
// Returns true on success
bool buildEnvelope(JsonDocument& doc, const Source& source, const char* type,
                   const char* id, int64_t timestamp,
                   const char* correlationId = nullptr,
                   const char* replyTo = nullptr);

// Parse envelope fields from a JsonObject
// Returns true if all required fields present
bool parseEnvelope(JsonObjectConst envelope, const char*& id,
                   int64_t& timestamp, const char*& service,
                   const char*& instance, const char*& version,
                   const char*& schemaVersion, const char*& type);

// Validate that an envelope has correct schema_version and valid type
bool validateEnvelopeType(const char* type);

} // namespace arturo
