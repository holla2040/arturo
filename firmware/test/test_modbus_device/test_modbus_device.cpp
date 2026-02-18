#include <unity.h>
#include "devices/modbus_device.h"

using namespace arturo;

void setUp(void) {}
void tearDown(void) {}

// --- validateModbusConfig tests ---

void test_validate_default_config(void) {
    TEST_ASSERT_TRUE(validateModbusConfig(MODBUS_DEFAULT_CONFIG));
}

void test_validate_zero_slave(void) {
    ModbusDeviceConfig cfg = {0, 9600, 1000, 5};
    TEST_ASSERT_FALSE(validateModbusConfig(cfg));
}

void test_validate_slave_too_high(void) {
    ModbusDeviceConfig cfg = {248, 9600, 1000, 5};
    TEST_ASSERT_FALSE(validateModbusConfig(cfg));
}

void test_validate_max_slave(void) {
    ModbusDeviceConfig cfg = {247, 9600, 1000, 5};
    TEST_ASSERT_TRUE(validateModbusConfig(cfg));
}

void test_validate_zero_baud(void) {
    ModbusDeviceConfig cfg = {1, 0, 1000, 5};
    TEST_ASSERT_FALSE(validateModbusConfig(cfg));
}

void test_validate_zero_timeout(void) {
    ModbusDeviceConfig cfg = {1, 9600, 0, 5};
    TEST_ASSERT_FALSE(validateModbusConfig(cfg));
}

// --- modbusCharTimeoutUs tests ---

void test_char_timeout_9600(void) {
    unsigned long us = modbusCharTimeoutUs(9600);
    // 11 * 1500000 / 9600 = 1718.75, truncated to 1718
    TEST_ASSERT_EQUAL_UINT32(1718, us);
}

void test_char_timeout_2400(void) {
    unsigned long us = modbusCharTimeoutUs(2400);
    // 11 * 1500000 / 2400 = 6875
    TEST_ASSERT_EQUAL_UINT32(6875, us);
}

void test_char_timeout_19200(void) {
    unsigned long us = modbusCharTimeoutUs(19200);
    // 11 * 1500000 / 19200 = 859.375, truncated to 859
    TEST_ASSERT_EQUAL_UINT32(859, us);
}

void test_char_timeout_high_baud(void) {
    // For baud > 19200, fixed 750us per Modbus spec
    TEST_ASSERT_EQUAL_UINT32(750, modbusCharTimeoutUs(38400));
    TEST_ASSERT_EQUAL_UINT32(750, modbusCharTimeoutUs(115200));
}

void test_char_timeout_zero_baud(void) {
    TEST_ASSERT_EQUAL_UINT32(0, modbusCharTimeoutUs(0));
}

// --- modbusFrameSilenceUs tests ---

void test_frame_silence_9600(void) {
    unsigned long us = modbusFrameSilenceUs(9600);
    // 11 * 3500000 / 9600 = 4010.4, truncated to 4010
    TEST_ASSERT_EQUAL_UINT32(4010, us);
}

void test_frame_silence_high_baud(void) {
    // For baud > 19200, fixed 1750us per Modbus spec
    TEST_ASSERT_EQUAL_UINT32(1750, modbusFrameSilenceUs(38400));
}

void test_frame_silence_zero_baud(void) {
    TEST_ASSERT_EQUAL_UINT32(0, modbusFrameSilenceUs(0));
}

// --- Default config values ---

void test_default_config_values(void) {
    TEST_ASSERT_EQUAL_UINT8(1, MODBUS_DEFAULT_CONFIG.slaveAddr);
    TEST_ASSERT_EQUAL_UINT32(9600, MODBUS_DEFAULT_CONFIG.baudRate);
    TEST_ASSERT_EQUAL_UINT32(1000, MODBUS_DEFAULT_CONFIG.responseTimeoutMs);
    TEST_ASSERT_EQUAL_UINT32(5, MODBUS_DEFAULT_CONFIG.turnaroundDelayMs);
}

int main(int argc, char **argv) {
    UNITY_BEGIN();
    // Config validation
    RUN_TEST(test_validate_default_config);
    RUN_TEST(test_validate_zero_slave);
    RUN_TEST(test_validate_slave_too_high);
    RUN_TEST(test_validate_max_slave);
    RUN_TEST(test_validate_zero_baud);
    RUN_TEST(test_validate_zero_timeout);
    // Char timeout
    RUN_TEST(test_char_timeout_9600);
    RUN_TEST(test_char_timeout_2400);
    RUN_TEST(test_char_timeout_19200);
    RUN_TEST(test_char_timeout_high_baud);
    RUN_TEST(test_char_timeout_zero_baud);
    // Frame silence
    RUN_TEST(test_frame_silence_9600);
    RUN_TEST(test_frame_silence_high_baud);
    RUN_TEST(test_frame_silence_zero_baud);
    // Default config
    RUN_TEST(test_default_config_values);
    UNITY_END();
    return 0;
}
