#include <unity.h>
#include <cstring>
#include "devices/serial_device.h"

using namespace arturo;

void setUp(void) {}
void tearDown(void) {}

// --- parseSerialConfig tests ---

void test_parse_config_9600_8N1(void) {
    SerialConfig cfg;
    bool ok = parseSerialConfig("9600-8N1", cfg);
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_EQUAL_UINT32(9600, cfg.baudRate);
    TEST_ASSERT_EQUAL_UINT8(8, cfg.dataBits);
    TEST_ASSERT_EQUAL_CHAR('N', cfg.parity);
    TEST_ASSERT_EQUAL_UINT8(1, cfg.stopBits);
}

void test_parse_config_2400_7E1(void) {
    SerialConfig cfg;
    bool ok = parseSerialConfig("2400-7E1", cfg);
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_EQUAL_UINT32(2400, cfg.baudRate);
    TEST_ASSERT_EQUAL_UINT8(7, cfg.dataBits);
    TEST_ASSERT_EQUAL_CHAR('E', cfg.parity);
    TEST_ASSERT_EQUAL_UINT8(1, cfg.stopBits);
}

void test_parse_config_115200_8N1(void) {
    SerialConfig cfg;
    bool ok = parseSerialConfig("115200-8N1", cfg);
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_EQUAL_UINT32(115200, cfg.baudRate);
    TEST_ASSERT_EQUAL_UINT8(8, cfg.dataBits);
    TEST_ASSERT_EQUAL_CHAR('N', cfg.parity);
    TEST_ASSERT_EQUAL_UINT8(1, cfg.stopBits);
}

void test_parse_config_odd_parity(void) {
    SerialConfig cfg;
    bool ok = parseSerialConfig("19200-8O2", cfg);
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_EQUAL_UINT32(19200, cfg.baudRate);
    TEST_ASSERT_EQUAL_UINT8(8, cfg.dataBits);
    TEST_ASSERT_EQUAL_CHAR('O', cfg.parity);
    TEST_ASSERT_EQUAL_UINT8(2, cfg.stopBits);
}

void test_parse_config_null(void) {
    SerialConfig cfg;
    TEST_ASSERT_FALSE(parseSerialConfig(nullptr, cfg));
}

void test_parse_config_no_dash(void) {
    SerialConfig cfg;
    TEST_ASSERT_FALSE(parseSerialConfig("9600", cfg));
}

void test_parse_config_bad_mode(void) {
    SerialConfig cfg;
    // Invalid parity
    TEST_ASSERT_FALSE(parseSerialConfig("9600-8X1", cfg));
    // Invalid data bits
    TEST_ASSERT_FALSE(parseSerialConfig("9600-4N1", cfg));
    // Invalid stop bits
    TEST_ASSERT_FALSE(parseSerialConfig("9600-8N3", cfg));
    // Too short mode
    TEST_ASSERT_FALSE(parseSerialConfig("9600-8N", cfg));
}

void test_parse_config_zero_baud(void) {
    SerialConfig cfg;
    TEST_ASSERT_FALSE(parseSerialConfig("0-8N1", cfg));
}

// --- serialConfigToMode tests (native packing) ---

void test_mode_8N1(void) {
    SerialConfig cfg = {9600, 8, 'N', 1};
    uint32_t mode = serialConfigToMode(cfg);
    // Native: (8 << 16) | ('N' << 8) | 1
    TEST_ASSERT_EQUAL_UINT32((8 << 16) | ('N' << 8) | 1, mode);
}

void test_mode_7E1(void) {
    SerialConfig cfg = {2400, 7, 'E', 1};
    uint32_t mode = serialConfigToMode(cfg);
    TEST_ASSERT_EQUAL_UINT32((7 << 16) | ('E' << 8) | 1, mode);
}

void test_mode_8O2(void) {
    SerialConfig cfg = {19200, 8, 'O', 2};
    uint32_t mode = serialConfigToMode(cfg);
    TEST_ASSERT_EQUAL_UINT32((8 << 16) | ('O' << 8) | 2, mode);
}

// --- Default config constants ---

void test_default_cti_config(void) {
    TEST_ASSERT_EQUAL_UINT32(2400, SERIAL_CONFIG_CTI.baudRate);
    TEST_ASSERT_EQUAL_UINT8(7, SERIAL_CONFIG_CTI.dataBits);
    TEST_ASSERT_EQUAL_CHAR('E', SERIAL_CONFIG_CTI.parity);
    TEST_ASSERT_EQUAL_UINT8(1, SERIAL_CONFIG_CTI.stopBits);
}

void test_default_modbus_config(void) {
    TEST_ASSERT_EQUAL_UINT32(9600, SERIAL_CONFIG_MODBUS.baudRate);
    TEST_ASSERT_EQUAL_UINT8(8, SERIAL_CONFIG_MODBUS.dataBits);
    TEST_ASSERT_EQUAL_CHAR('N', SERIAL_CONFIG_MODBUS.parity);
    TEST_ASSERT_EQUAL_UINT8(1, SERIAL_CONFIG_MODBUS.stopBits);
}

void test_default_ascii_config(void) {
    TEST_ASSERT_EQUAL_UINT32(115200, SERIAL_CONFIG_ASCII.baudRate);
    TEST_ASSERT_EQUAL_UINT8(8, SERIAL_CONFIG_ASCII.dataBits);
    TEST_ASSERT_EQUAL_CHAR('N', SERIAL_CONFIG_ASCII.parity);
    TEST_ASSERT_EQUAL_UINT8(1, SERIAL_CONFIG_ASCII.stopBits);
}

int main(int argc, char **argv) {
    UNITY_BEGIN();
    RUN_TEST(test_parse_config_9600_8N1);
    RUN_TEST(test_parse_config_2400_7E1);
    RUN_TEST(test_parse_config_115200_8N1);
    RUN_TEST(test_parse_config_odd_parity);
    RUN_TEST(test_parse_config_null);
    RUN_TEST(test_parse_config_no_dash);
    RUN_TEST(test_parse_config_bad_mode);
    RUN_TEST(test_parse_config_zero_baud);
    RUN_TEST(test_mode_8N1);
    RUN_TEST(test_mode_7E1);
    RUN_TEST(test_mode_8O2);
    RUN_TEST(test_default_cti_config);
    RUN_TEST(test_default_modbus_config);
    RUN_TEST(test_default_ascii_config);
    UNITY_END();
    return 0;
}
