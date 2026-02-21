#include <unity.h>
#include <ArduinoJson.h>
#include "commands/command_handler.h"
#include "messaging/envelope.h"

using namespace arturo;

void setUp(void) {}
void tearDown(void) {}

// Valid command request JSON matching the schema
static const char* VALID_REQUEST =
    "{\"envelope\":{\"id\":\"550e8400-e29b-41d4-a716-446655440000\","
    "\"timestamp\":1771329600,"
    "\"source\":{\"service\":\"controller\",\"instance\":\"ctrl-01\",\"version\":\"1.0.0\"},"
    "\"schema_version\":\"v1.0.0\","
    "\"type\":\"device.command.request\","
    "\"correlation_id\":\"7c9e6679-7425-40de-944b-e07fc1f90ae7\","
    "\"reply_to\":\"responses:server-01\"},"
    "\"payload\":{\"device_id\":\"fluke-8846a\","
    "\"command_name\":\"*IDN?\","
    "\"parameters\":{},"
    "\"timeout_ms\":5000}}";

void test_parse_command_request_valid(void) {
    JsonDocument doc;
    CommandRequest req;

    bool result = parseCommandRequest(VALID_REQUEST, doc, req);
    TEST_ASSERT_TRUE(result);
    TEST_ASSERT_EQUAL_STRING("7c9e6679-7425-40de-944b-e07fc1f90ae7", req.correlationId);
    TEST_ASSERT_EQUAL_STRING("responses:server-01", req.replyTo);
    TEST_ASSERT_EQUAL_STRING("fluke-8846a", req.deviceId);
    TEST_ASSERT_EQUAL_STRING("*IDN?", req.commandName);
    TEST_ASSERT_EQUAL_INT(5000, req.timeoutMs);
}

void test_parse_command_request_missing_fields(void) {
    // Missing payload.device_id
    const char* json =
        "{\"envelope\":{\"id\":\"test-id\","
        "\"timestamp\":1700000000,"
        "\"source\":{\"service\":\"controller\",\"instance\":\"ctrl-01\",\"version\":\"1.0.0\"},"
        "\"schema_version\":\"v1.0.0\","
        "\"type\":\"device.command.request\","
        "\"correlation_id\":\"corr-123\","
        "\"reply_to\":\"responses:ctrl-01\"},"
        "\"payload\":{\"command_name\":\"*IDN?\",\"timeout_ms\":5000}}";

    JsonDocument doc;
    CommandRequest req;

    bool result = parseCommandRequest(json, doc, req);
    TEST_ASSERT_FALSE(result);
}

void test_parse_command_request_wrong_type(void) {
    // Type is service.heartbeat instead of device.command.request
    const char* json =
        "{\"envelope\":{\"id\":\"test-id\","
        "\"timestamp\":1700000000,"
        "\"source\":{\"service\":\"controller\",\"instance\":\"ctrl-01\",\"version\":\"1.0.0\"},"
        "\"schema_version\":\"v1.0.0\","
        "\"type\":\"service.heartbeat\","
        "\"correlation_id\":\"corr-123\","
        "\"reply_to\":\"responses:ctrl-01\"},"
        "\"payload\":{\"device_id\":\"fluke-8846a\",\"command_name\":\"*IDN?\",\"timeout_ms\":5000}}";

    JsonDocument doc;
    CommandRequest req;

    bool result = parseCommandRequest(json, doc, req);
    TEST_ASSERT_FALSE(result);
}

void test_build_command_response_success(void) {
    JsonDocument doc;
    Source src = {"station", "station-01", "1.0.0"};

    bool result = buildCommandResponse(doc, src,
                                        "resp-001", 1700000000,
                                        "corr-123",
                                        "fluke-8846a", "*IDN?",
                                        true, "FLUKE,8846A,12345,1.0",
                                        nullptr, nullptr,
                                        42);
    TEST_ASSERT_TRUE(result);

    // Check envelope
    JsonObject envelope = doc["envelope"];
    TEST_ASSERT_EQUAL_STRING("resp-001", envelope["id"].as<const char*>());
    TEST_ASSERT_EQUAL_STRING("device.command.response", envelope["type"].as<const char*>());
    TEST_ASSERT_EQUAL_STRING("corr-123", envelope["correlation_id"].as<const char*>());

    // Check payload
    JsonObject payload = doc["payload"];
    TEST_ASSERT_EQUAL_STRING("fluke-8846a", payload["device_id"].as<const char*>());
    TEST_ASSERT_EQUAL_STRING("*IDN?", payload["command_name"].as<const char*>());
    TEST_ASSERT_TRUE(payload["success"].as<bool>());
    TEST_ASSERT_EQUAL_STRING("FLUKE,8846A,12345,1.0", payload["response"].as<const char*>());
    TEST_ASSERT_EQUAL_INT(42, payload["duration_ms"].as<int>());
    TEST_ASSERT_TRUE(payload["error"].isNull());
}

void test_build_command_response_error(void) {
    JsonDocument doc;
    Source src = {"station", "station-01", "1.0.0"};

    bool result = buildCommandResponse(doc, src,
                                        "resp-002", 1700000000,
                                        "corr-456",
                                        "fluke-8846a", "*IDN?",
                                        false, nullptr,
                                        "TIMEOUT", "Device did not respond within 5000ms",
                                        5000);
    TEST_ASSERT_TRUE(result);

    JsonObject payload = doc["payload"];
    TEST_ASSERT_FALSE(payload["success"].as<bool>());
    TEST_ASSERT_TRUE(payload["response"].isNull());

    JsonObject error = payload["error"];
    TEST_ASSERT_FALSE(error.isNull());
    TEST_ASSERT_EQUAL_STRING("TIMEOUT", error["code"].as<const char*>());
    TEST_ASSERT_EQUAL_STRING("Device did not respond within 5000ms",
                             error["message"].as<const char*>());
    TEST_ASSERT_EQUAL_INT(5000, payload["duration_ms"].as<int>());
}

void test_roundtrip_response(void) {
    // Build a response
    JsonDocument buildDoc;
    Source src = {"station", "station-01", "1.0.0"};

    buildCommandResponse(buildDoc, src,
                         "resp-rt", 1700000000,
                         "corr-rt",
                         "psu-01", "MEAS:VOLT?",
                         true, "12.345",
                         nullptr, nullptr,
                         15);

    // Serialize to JSON
    char buffer[2048];
    size_t len = serializeJson(buildDoc, buffer, sizeof(buffer));
    TEST_ASSERT_TRUE(len > 0);

    // Deserialize into a new document
    JsonDocument parseDoc;
    DeserializationError err = deserializeJson(parseDoc, buffer);
    TEST_ASSERT_TRUE(err == DeserializationError::Ok);

    // Verify envelope
    JsonObject envelope = parseDoc["envelope"];
    TEST_ASSERT_EQUAL_STRING("resp-rt", envelope["id"].as<const char*>());
    TEST_ASSERT_EQUAL_STRING("device.command.response", envelope["type"].as<const char*>());
    TEST_ASSERT_EQUAL_STRING("corr-rt", envelope["correlation_id"].as<const char*>());
    TEST_ASSERT_EQUAL_STRING("v1.0.0", envelope["schema_version"].as<const char*>());

    // Verify source
    JsonObject source = envelope["source"];
    TEST_ASSERT_EQUAL_STRING("station", source["service"].as<const char*>());
    TEST_ASSERT_EQUAL_STRING("station-01", source["instance"].as<const char*>());
    TEST_ASSERT_EQUAL_STRING("1.0.0", source["version"].as<const char*>());

    // Verify payload
    JsonObject payload = parseDoc["payload"];
    TEST_ASSERT_EQUAL_STRING("psu-01", payload["device_id"].as<const char*>());
    TEST_ASSERT_EQUAL_STRING("MEAS:VOLT?", payload["command_name"].as<const char*>());
    TEST_ASSERT_TRUE(payload["success"].as<bool>());
    TEST_ASSERT_EQUAL_STRING("12.345", payload["response"].as<const char*>());
    TEST_ASSERT_EQUAL_INT(15, payload["duration_ms"].as<int>());
}

int main(int argc, char **argv) {
    UNITY_BEGIN();
    RUN_TEST(test_parse_command_request_valid);
    RUN_TEST(test_parse_command_request_missing_fields);
    RUN_TEST(test_parse_command_request_wrong_type);
    RUN_TEST(test_build_command_response_success);
    RUN_TEST(test_build_command_response_error);
    RUN_TEST(test_roundtrip_response);
    UNITY_END();
    return 0;
}
