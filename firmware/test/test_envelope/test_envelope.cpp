#include <unity.h>
#include <ArduinoJson.h>
#include "messaging/envelope.h"

using namespace arturo;

void setUp(void) {}
void tearDown(void) {}

void test_build_envelope_has_required_fields(void) {
    JsonDocument doc;
    Source src = {"station", "station-01", "1.0.0"};

    bool result = buildEnvelope(doc, src, "service.heartbeat", "test-id-123", 1700000000);
    TEST_ASSERT_TRUE(result);

    JsonObject envelope = doc["envelope"];
    TEST_ASSERT_FALSE(envelope.isNull());
    TEST_ASSERT_EQUAL_STRING("test-id-123", envelope["id"].as<const char*>());
    TEST_ASSERT_EQUAL_INT64(1700000000, envelope["timestamp"].as<int64_t>());
    TEST_ASSERT_EQUAL_STRING("service.heartbeat", envelope["type"].as<const char*>());

    JsonObject source = envelope["source"];
    TEST_ASSERT_FALSE(source.isNull());
    TEST_ASSERT_EQUAL_STRING("station", source["service"].as<const char*>());
    TEST_ASSERT_EQUAL_STRING("station-01", source["instance"].as<const char*>());
    TEST_ASSERT_EQUAL_STRING("1.0.0", source["version"].as<const char*>());
}

void test_build_envelope_schema_version(void) {
    JsonDocument doc;
    Source src = {"station", "station-01", "1.0.0"};

    buildEnvelope(doc, src, "service.heartbeat", "test-id", 1700000000);

    TEST_ASSERT_EQUAL_STRING("v1.0.0", doc["envelope"]["schema_version"].as<const char*>());
}

void test_build_envelope_with_correlation(void) {
    JsonDocument doc;
    Source src = {"station", "station-01", "1.0.0"};

    buildEnvelope(doc, src, "device.command.response", "test-id", 1700000000,
                  "corr-123", "reply-456");

    JsonObject envelope = doc["envelope"];
    TEST_ASSERT_EQUAL_STRING("corr-123", envelope["correlation_id"].as<const char*>());
    TEST_ASSERT_EQUAL_STRING("reply-456", envelope["reply_to"].as<const char*>());
}

void test_build_envelope_without_correlation(void) {
    JsonDocument doc;
    Source src = {"station", "station-01", "1.0.0"};

    buildEnvelope(doc, src, "service.heartbeat", "test-id", 1700000000);

    JsonObject envelope = doc["envelope"];
    TEST_ASSERT_TRUE(envelope["correlation_id"].isNull());
    TEST_ASSERT_TRUE(envelope["reply_to"].isNull());
}

void test_parse_envelope_valid(void) {
    JsonDocument doc;
    JsonObject envelope = doc["envelope"].to<JsonObject>();
    envelope["id"] = "parse-test-id";
    envelope["timestamp"] = (int64_t)1700000000;
    envelope["schema_version"] = "v1.0.0";
    envelope["type"] = "service.heartbeat";
    JsonObject source = envelope["source"].to<JsonObject>();
    source["service"] = "station";
    source["instance"] = "station-02";
    source["version"] = "1.0.0";

    const char* id = nullptr;
    int64_t timestamp = 0;
    const char* service = nullptr;
    const char* instance = nullptr;
    const char* version = nullptr;
    const char* schemaVersion = nullptr;
    const char* type = nullptr;

    bool result = parseEnvelope(doc["envelope"].as<JsonObjectConst>(),
                                id, timestamp, service, instance, version,
                                schemaVersion, type);

    TEST_ASSERT_TRUE(result);
    TEST_ASSERT_EQUAL_STRING("parse-test-id", id);
    TEST_ASSERT_EQUAL_INT64(1700000000, timestamp);
    TEST_ASSERT_EQUAL_STRING("station", service);
    TEST_ASSERT_EQUAL_STRING("station-02", instance);
    TEST_ASSERT_EQUAL_STRING("1.0.0", version);
    TEST_ASSERT_EQUAL_STRING("v1.0.0", schemaVersion);
    TEST_ASSERT_EQUAL_STRING("service.heartbeat", type);
}

void test_parse_envelope_missing_field(void) {
    JsonDocument doc;
    JsonObject envelope = doc["envelope"].to<JsonObject>();
    envelope["id"] = "parse-test-id";
    envelope["timestamp"] = (int64_t)1700000000;
    envelope["schema_version"] = "v1.0.0";
    // "type" is intentionally missing
    JsonObject source = envelope["source"].to<JsonObject>();
    source["service"] = "station";
    source["instance"] = "station-02";
    source["version"] = "1.0.0";

    const char* id = nullptr;
    int64_t timestamp = 0;
    const char* service = nullptr;
    const char* instance = nullptr;
    const char* version = nullptr;
    const char* schemaVersion = nullptr;
    const char* type = nullptr;

    bool result = parseEnvelope(doc["envelope"].as<JsonObjectConst>(),
                                id, timestamp, service, instance, version,
                                schemaVersion, type);

    TEST_ASSERT_FALSE(result);
}

void test_validate_envelope_type_valid(void) {
    TEST_ASSERT_TRUE(validateEnvelopeType("device.command.request"));
    TEST_ASSERT_TRUE(validateEnvelopeType("device.command.response"));
    TEST_ASSERT_TRUE(validateEnvelopeType("service.heartbeat"));
    TEST_ASSERT_TRUE(validateEnvelopeType("system.emergency_stop"));
    TEST_ASSERT_TRUE(validateEnvelopeType("system.ota.request"));
}

void test_validate_envelope_type_invalid(void) {
    TEST_ASSERT_FALSE(validateEnvelopeType("unknown.type"));
    TEST_ASSERT_FALSE(validateEnvelopeType(nullptr));
}

void test_roundtrip_build_parse(void) {
    // Build
    JsonDocument buildDoc;
    Source src = {"station", "station-01", "1.0.0"};
    buildEnvelope(buildDoc, src, "device.command.request", "roundtrip-id", 1700000099,
                  "corr-rt", "reply-rt");

    // Serialize
    char buffer[512];
    serializeJson(buildDoc, buffer, sizeof(buffer));

    // Deserialize into new document
    JsonDocument parseDoc;
    DeserializationError err = deserializeJson(parseDoc, buffer);
    TEST_ASSERT_TRUE(err == DeserializationError::Ok);

    // Parse
    const char* id = nullptr;
    int64_t timestamp = 0;
    const char* service = nullptr;
    const char* instance = nullptr;
    const char* version = nullptr;
    const char* schemaVersion = nullptr;
    const char* type = nullptr;

    bool result = parseEnvelope(parseDoc["envelope"].as<JsonObjectConst>(),
                                id, timestamp, service, instance, version,
                                schemaVersion, type);

    TEST_ASSERT_TRUE(result);
    TEST_ASSERT_EQUAL_STRING("roundtrip-id", id);
    TEST_ASSERT_EQUAL_INT64(1700000099, timestamp);
    TEST_ASSERT_EQUAL_STRING("station", service);
    TEST_ASSERT_EQUAL_STRING("station-01", instance);
    TEST_ASSERT_EQUAL_STRING("1.0.0", version);
    TEST_ASSERT_EQUAL_STRING("v1.0.0", schemaVersion);
    TEST_ASSERT_EQUAL_STRING("device.command.request", type);

    // Verify optional fields survived roundtrip
    JsonObject envelope = parseDoc["envelope"];
    TEST_ASSERT_EQUAL_STRING("corr-rt", envelope["correlation_id"].as<const char*>());
    TEST_ASSERT_EQUAL_STRING("reply-rt", envelope["reply_to"].as<const char*>());
}

int main(int argc, char **argv) {
    UNITY_BEGIN();
    RUN_TEST(test_build_envelope_has_required_fields);
    RUN_TEST(test_build_envelope_schema_version);
    RUN_TEST(test_build_envelope_with_correlation);
    RUN_TEST(test_build_envelope_without_correlation);
    RUN_TEST(test_parse_envelope_valid);
    RUN_TEST(test_parse_envelope_missing_field);
    RUN_TEST(test_validate_envelope_type_valid);
    RUN_TEST(test_validate_envelope_type_invalid);
    RUN_TEST(test_roundtrip_build_parse);
    UNITY_END();
    return 0;
}
