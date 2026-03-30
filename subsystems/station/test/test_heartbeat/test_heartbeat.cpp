#include <unity.h>
#include <ArduinoJson.h>
#include "messaging/heartbeat.h"
#include "messaging/envelope.h"

using namespace arturo;

void setUp(void) {}
void tearDown(void) {}

static HeartbeatData makeTestData() {
    static const char* devices[] = {"DMM-01", "PSU-01", nullptr};
    HeartbeatData data;
    data.status = "running";
    data.uptimeSeconds = 3600;
    data.devices = devices;
    data.deviceCount = 2;
    data.freeHeap = 180000;
    data.minFreeHeap = 150000;
    data.wifiRssi = -45;
    data.wifiReconnects = 0;
    data.redisReconnects = 1;
    data.commandsProcessed = 42;
    data.commandsFailed = 2;
    data.lastError = nullptr;
    data.watchdogResets = 0;
    data.firmwareVersion = "1.0.0";
    data.deviceTypes = nullptr;
    return data;
}

static Source makeTestSource() {
    Source src;
    src.service = "station";
    src.instance = "station-001";
    src.version = "1.0.0";
    return src;
}

void test_build_heartbeat_has_envelope(void) {
    JsonDocument doc;
    Source src = makeTestSource();
    HeartbeatData data = makeTestData();

    bool result = buildHeartbeat(doc, src, "msg-001", 1700000000, data);
    TEST_ASSERT_TRUE(result);
    TEST_ASSERT_FALSE(doc["envelope"].isNull());
    TEST_ASSERT_EQUAL_STRING("service.heartbeat",
                             doc["envelope"]["type"].as<const char*>());
}

void test_build_heartbeat_required_payload_fields(void) {
    JsonDocument doc;
    Source src = makeTestSource();
    HeartbeatData data = makeTestData();

    buildHeartbeat(doc, src, "msg-002", 1700000000, data);

    JsonObject payload = doc["payload"];
    TEST_ASSERT_FALSE(payload.isNull());
    TEST_ASSERT_TRUE(payload["status"].is<const char*>());
    TEST_ASSERT_TRUE(payload["uptime_seconds"].is<int64_t>());
    TEST_ASSERT_TRUE(payload["devices"].is<JsonArray>());
    TEST_ASSERT_TRUE(payload["free_heap"].is<int64_t>());
    TEST_ASSERT_TRUE(payload["wifi_rssi"].is<int>());
    TEST_ASSERT_TRUE(payload["firmware_version"].is<const char*>());
}

void test_build_heartbeat_optional_fields(void) {
    JsonDocument doc;
    Source src = makeTestSource();
    HeartbeatData data = makeTestData();

    buildHeartbeat(doc, src, "msg-003", 1700000000, data);

    JsonObject payload = doc["payload"];
    TEST_ASSERT_TRUE(payload["min_free_heap"].is<int64_t>());
    TEST_ASSERT_TRUE(payload["wifi_reconnects"].is<int>());
    TEST_ASSERT_TRUE(payload["redis_reconnects"].is<int>());
    TEST_ASSERT_TRUE(payload["commands_processed"].is<int>());
    TEST_ASSERT_TRUE(payload["commands_failed"].is<int>());
    TEST_ASSERT_TRUE(payload["watchdog_resets"].is<int>());
}

void test_build_heartbeat_devices_array(void) {
    JsonDocument doc;
    Source src = makeTestSource();
    HeartbeatData data = makeTestData();

    buildHeartbeat(doc, src, "msg-004", 1700000000, data);

    JsonArray devices = doc["payload"]["devices"];
    TEST_ASSERT_FALSE(devices.isNull());
    TEST_ASSERT_EQUAL(2, devices.size());
    TEST_ASSERT_EQUAL_STRING("DMM-01", devices[0].as<const char*>());
    TEST_ASSERT_EQUAL_STRING("PSU-01", devices[1].as<const char*>());
}

void test_build_heartbeat_null_last_error(void) {
    JsonDocument doc;
    Source src = makeTestSource();
    HeartbeatData data = makeTestData();
    data.lastError = nullptr;

    buildHeartbeat(doc, src, "msg-005", 1700000000, data);

    TEST_ASSERT_TRUE(doc["payload"]["last_error"].isNull());
}

void test_build_heartbeat_with_last_error(void) {
    JsonDocument doc;
    Source src = makeTestSource();
    HeartbeatData data = makeTestData();
    data.lastError = "timeout on SCPI";

    buildHeartbeat(doc, src, "msg-006", 1700000000, data);

    TEST_ASSERT_EQUAL_STRING("timeout on SCPI",
                             doc["payload"]["last_error"].as<const char*>());
}

void test_build_heartbeat_status_values(void) {
    const char* statuses[] = {"starting", "running", "degraded", "stopping"};
    Source src = makeTestSource();

    for (int i = 0; i < 4; i++) {
        JsonDocument doc;
        HeartbeatData data = makeTestData();
        data.status = statuses[i];

        bool result = buildHeartbeat(doc, src, "msg-007", 1700000000, data);
        TEST_ASSERT_TRUE(result);
        TEST_ASSERT_EQUAL_STRING(statuses[i],
                                 doc["payload"]["status"].as<const char*>());
    }
}

void test_parse_heartbeat_payload(void) {
    JsonDocument doc;
    Source src = makeTestSource();
    HeartbeatData original = makeTestData();
    original.lastError = "some error";

    buildHeartbeat(doc, src, "msg-008", 1700000000, original);

    HeartbeatData parsed;
    bool result = parseHeartbeatPayload(doc["payload"].as<JsonObjectConst>(), parsed);
    TEST_ASSERT_TRUE(result);

    TEST_ASSERT_EQUAL_STRING(original.status, parsed.status);
    TEST_ASSERT_EQUAL_INT64(original.uptimeSeconds, parsed.uptimeSeconds);
    TEST_ASSERT_EQUAL(original.deviceCount, parsed.deviceCount);
    TEST_ASSERT_EQUAL_STRING("DMM-01", parsed.devices[0]);
    TEST_ASSERT_EQUAL_STRING("PSU-01", parsed.devices[1]);
    TEST_ASSERT_EQUAL_INT64(original.freeHeap, parsed.freeHeap);
    TEST_ASSERT_EQUAL_INT64(original.minFreeHeap, parsed.minFreeHeap);
    TEST_ASSERT_EQUAL(original.wifiRssi, parsed.wifiRssi);
    TEST_ASSERT_EQUAL(original.wifiReconnects, parsed.wifiReconnects);
    TEST_ASSERT_EQUAL(original.redisReconnects, parsed.redisReconnects);
    TEST_ASSERT_EQUAL(original.commandsProcessed, parsed.commandsProcessed);
    TEST_ASSERT_EQUAL(original.commandsFailed, parsed.commandsFailed);
    TEST_ASSERT_EQUAL_STRING("some error", parsed.lastError);
    TEST_ASSERT_EQUAL(original.watchdogResets, parsed.watchdogResets);
    TEST_ASSERT_EQUAL_STRING(original.firmwareVersion, parsed.firmwareVersion);
}

void test_roundtrip_heartbeat(void) {
    JsonDocument doc;
    Source src = makeTestSource();
    HeartbeatData original = makeTestData();

    buildHeartbeat(doc, src, "msg-009", 1700000000, original);

    char buffer[2048];
    size_t len = serializeJson(doc, buffer, sizeof(buffer));
    TEST_ASSERT_TRUE(len > 0);

    JsonDocument doc2;
    DeserializationError err = deserializeJson(doc2, buffer);
    TEST_ASSERT_TRUE(err == DeserializationError::Ok);

    HeartbeatData parsed;
    bool result = parseHeartbeatPayload(doc2["payload"].as<JsonObjectConst>(), parsed);
    TEST_ASSERT_TRUE(result);

    TEST_ASSERT_EQUAL_STRING(original.status, parsed.status);
    TEST_ASSERT_EQUAL_INT64(original.uptimeSeconds, parsed.uptimeSeconds);
    TEST_ASSERT_EQUAL(original.deviceCount, parsed.deviceCount);
    TEST_ASSERT_EQUAL_STRING("DMM-01", parsed.devices[0]);
    TEST_ASSERT_EQUAL_STRING("PSU-01", parsed.devices[1]);
    TEST_ASSERT_EQUAL_INT64(original.freeHeap, parsed.freeHeap);
    TEST_ASSERT_EQUAL_INT64(original.minFreeHeap, parsed.minFreeHeap);
    TEST_ASSERT_EQUAL(original.wifiRssi, parsed.wifiRssi);
    TEST_ASSERT_EQUAL(original.wifiReconnects, parsed.wifiReconnects);
    TEST_ASSERT_EQUAL(original.redisReconnects, parsed.redisReconnects);
    TEST_ASSERT_EQUAL(original.commandsProcessed, parsed.commandsProcessed);
    TEST_ASSERT_EQUAL(original.commandsFailed, parsed.commandsFailed);
    TEST_ASSERT_NULL(parsed.lastError);
    TEST_ASSERT_EQUAL(original.watchdogResets, parsed.watchdogResets);
    TEST_ASSERT_EQUAL_STRING(original.firmwareVersion, parsed.firmwareVersion);
}

void test_heartbeat_json_size(void) {
    JsonDocument doc;
    Source src = makeTestSource();
    HeartbeatData data = makeTestData();

    buildHeartbeat(doc, src, "msg-010", 1700000000, data);

    char buffer[2048];
    size_t len = serializeJson(doc, buffer, sizeof(buffer));
    TEST_ASSERT_TRUE(len < 1024);
}

int main(int argc, char **argv) {
    UNITY_BEGIN();
    RUN_TEST(test_build_heartbeat_has_envelope);
    RUN_TEST(test_build_heartbeat_required_payload_fields);
    RUN_TEST(test_build_heartbeat_optional_fields);
    RUN_TEST(test_build_heartbeat_devices_array);
    RUN_TEST(test_build_heartbeat_null_last_error);
    RUN_TEST(test_build_heartbeat_with_last_error);
    RUN_TEST(test_build_heartbeat_status_values);
    RUN_TEST(test_parse_heartbeat_payload);
    RUN_TEST(test_roundtrip_heartbeat);
    RUN_TEST(test_heartbeat_json_size);
    UNITY_END();
    return 0;
}
