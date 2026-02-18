#include <unity.h>
#include <cstring>
#include "protocols/scpi_client.h"
#include "commands/device_registry.h"

using namespace arturo;

void setUp(void) {}
void tearDown(void) {}

// --- formatScpiCommand tests ---

void test_format_scpi_command_basic(void) {
    char buf[64];
    int len = formatScpiCommand("*IDN?", buf, sizeof(buf));
    TEST_ASSERT_EQUAL_INT(6, len);
    TEST_ASSERT_EQUAL_STRING("*IDN?\n", buf);
}

void test_format_scpi_command_with_cr_lf(void) {
    char buf[64];
    int len = formatScpiCommand("*IDN?", buf, sizeof(buf), "\r\n");
    TEST_ASSERT_EQUAL_INT(7, len);
    TEST_ASSERT_EQUAL_STRING("*IDN?\r\n", buf);
}

void test_format_scpi_command_buffer_too_small(void) {
    char buf[4];
    int len = formatScpiCommand("*IDN?", buf, sizeof(buf));
    TEST_ASSERT_EQUAL_INT(-1, len);
}

void test_format_scpi_measurement(void) {
    char buf[64];
    int len = formatScpiCommand("MEAS:VOLT:DC?", buf, sizeof(buf));
    TEST_ASSERT_EQUAL_INT(14, len);
    TEST_ASSERT_EQUAL_STRING("MEAS:VOLT:DC?\n", buf);
}

// --- parseScpiResponse tests ---

void test_parse_response_numeric(void) {
    const char* raw = "1.23456789\n";
    char out[64];
    bool isError = true;
    bool ok = parseScpiResponse(raw, strlen(raw), out, sizeof(out), isError);
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_EQUAL_STRING("1.23456789", out);
    TEST_ASSERT_FALSE(isError);
}

void test_parse_response_string(void) {
    const char* raw = "FLUKE,8846A,12345,1.0\n";
    char out[64];
    bool isError = true;
    bool ok = parseScpiResponse(raw, strlen(raw), out, sizeof(out), isError);
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_EQUAL_STRING("FLUKE,8846A,12345,1.0", out);
    TEST_ASSERT_FALSE(isError);
}

void test_parse_response_error(void) {
    const char* raw = "-100,\"Command error\"\n";
    char out[64];
    bool isError = false;
    bool ok = parseScpiResponse(raw, strlen(raw), out, sizeof(out), isError);
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_EQUAL_STRING("-100,\"Command error\"", out);
    TEST_ASSERT_TRUE(isError);
}

void test_parse_response_empty(void) {
    char out[64];
    bool isError = false;
    bool ok = parseScpiResponse(nullptr, 0, out, sizeof(out), isError);
    TEST_ASSERT_FALSE(ok);

    ok = parseScpiResponse("", 0, out, sizeof(out), isError);
    TEST_ASSERT_FALSE(ok);
}

// --- parseScpiError tests ---

void test_parse_scpi_error_format(void) {
    int code = 0;
    char msg[64];
    bool ok = parseScpiError("-100,\"Command error\"", code, msg, sizeof(msg));
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_EQUAL_INT(-100, code);
    TEST_ASSERT_EQUAL_STRING("Command error", msg);
}

void test_parse_scpi_error_no_error(void) {
    int code = -1;
    char msg[64];
    bool ok = parseScpiError("0,\"No error\"", code, msg, sizeof(msg));
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_EQUAL_INT(0, code);
    TEST_ASSERT_EQUAL_STRING("No error", msg);
}

// --- DeviceRegistry tests ---

void test_device_registry_find(void) {
    const DeviceInfo* dev = getDevice("DMM-01");
    TEST_ASSERT_NOT_NULL(dev);
    TEST_ASSERT_EQUAL_STRING("DMM-01", dev->deviceId);
    TEST_ASSERT_EQUAL_STRING("192.168.1.100", dev->host);
    TEST_ASSERT_EQUAL_UINT16(5025, dev->port);
    TEST_ASSERT_EQUAL_STRING("scpi", dev->protocolType);
}

void test_device_registry_not_found(void) {
    const DeviceInfo* dev = getDevice("UNKNOWN");
    TEST_ASSERT_NULL(dev);
}

void test_device_registry_null(void) {
    const DeviceInfo* dev = getDevice(nullptr);
    TEST_ASSERT_NULL(dev);
}

void test_device_registry_get_all(void) {
    int count = 0;
    const DeviceInfo* devs = getDevices(count);
    TEST_ASSERT_NOT_NULL(devs);
    TEST_ASSERT_GREATER_OR_EQUAL(1, count);
}

int main(int argc, char **argv) {
    UNITY_BEGIN();
    RUN_TEST(test_format_scpi_command_basic);
    RUN_TEST(test_format_scpi_command_with_cr_lf);
    RUN_TEST(test_format_scpi_command_buffer_too_small);
    RUN_TEST(test_format_scpi_measurement);
    RUN_TEST(test_parse_response_numeric);
    RUN_TEST(test_parse_response_string);
    RUN_TEST(test_parse_response_error);
    RUN_TEST(test_parse_response_empty);
    RUN_TEST(test_parse_scpi_error_format);
    RUN_TEST(test_parse_scpi_error_no_error);
    RUN_TEST(test_device_registry_find);
    RUN_TEST(test_device_registry_not_found);
    RUN_TEST(test_device_registry_null);
    RUN_TEST(test_device_registry_get_all);
    UNITY_END();
    return 0;
}
