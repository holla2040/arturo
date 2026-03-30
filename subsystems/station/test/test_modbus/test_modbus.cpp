#include <unity.h>
#include <cstring>
#include "protocols/modbus.h"

using namespace arturo;

void setUp(void) {}
void tearDown(void) {}

// --- CRC16 tests ---
// Known CRC values verified against online Modbus CRC calculators

void test_crc16_empty(void) {
    uint16_t crc = modbusCrc16(nullptr, 0);
    TEST_ASSERT_EQUAL_HEX16(0xFFFF, crc);
}

void test_crc16_known_value(void) {
    // Slave=1, FC=03, Start=0x0000, Count=0x0001
    // On wire: CRC low=0x84, CRC high=0x0A -> uint16_t = 0x0A84
    uint8_t data[] = {0x01, 0x03, 0x00, 0x00, 0x00, 0x01};
    uint16_t crc = modbusCrc16(data, sizeof(data));
    TEST_ASSERT_EQUAL_HEX16(0x0A84, crc);
}

void test_crc16_single_byte(void) {
    uint8_t data[] = {0x01};
    uint16_t crc = modbusCrc16(data, 1);
    // CRC of single byte 0x01 starting from 0xFFFF
    TEST_ASSERT_EQUAL_HEX16(0x807E, crc);
}

// --- Read Holding Registers (FC 0x03) tests ---

void test_build_read_holding_basic(void) {
    uint8_t buf[32];
    int len = modbusBuildReadHolding(1, 0x1000, 1, buf, sizeof(buf));
    TEST_ASSERT_EQUAL_INT(8, len);
    TEST_ASSERT_EQUAL_UINT8(0x01, buf[0]);  // slave
    TEST_ASSERT_EQUAL_UINT8(0x03, buf[1]);  // FC
    TEST_ASSERT_EQUAL_UINT8(0x10, buf[2]);  // start hi
    TEST_ASSERT_EQUAL_UINT8(0x00, buf[3]);  // start lo
    TEST_ASSERT_EQUAL_UINT8(0x00, buf[4]);  // count hi
    TEST_ASSERT_EQUAL_UINT8(0x01, buf[5]);  // count lo
    // CRC is last 2 bytes (lo, hi)
    uint16_t crc = modbusCrc16(buf, 6);
    TEST_ASSERT_EQUAL_UINT8(crc & 0xFF, buf[6]);
    TEST_ASSERT_EQUAL_UINT8((crc >> 8) & 0xFF, buf[7]);
}

void test_build_read_holding_multiple(void) {
    uint8_t buf[32];
    int len = modbusBuildReadHolding(1, 0x0000, 5, buf, sizeof(buf));
    TEST_ASSERT_EQUAL_INT(8, len);
    TEST_ASSERT_EQUAL_UINT8(0x00, buf[4]);  // count hi
    TEST_ASSERT_EQUAL_UINT8(0x05, buf[5]);  // count lo
}

void test_build_read_holding_buffer_too_small(void) {
    uint8_t buf[4];
    int len = modbusBuildReadHolding(1, 0x0000, 1, buf, sizeof(buf));
    TEST_ASSERT_EQUAL_INT(-1, len);
}

void test_build_read_holding_zero_count(void) {
    uint8_t buf[32];
    int len = modbusBuildReadHolding(1, 0x0000, 0, buf, sizeof(buf));
    TEST_ASSERT_EQUAL_INT(-1, len);
}

void test_build_read_holding_exceeds_max(void) {
    uint8_t buf[32];
    int len = modbusBuildReadHolding(1, 0x0000, MODBUS_MAX_REGISTERS + 1, buf, sizeof(buf));
    TEST_ASSERT_EQUAL_INT(-1, len);
}

// --- Write Single Register (FC 0x06) tests ---

void test_build_write_single_basic(void) {
    uint8_t buf[32];
    int len = modbusBuildWriteSingle(1, 0x1001, 0x00C8, buf, sizeof(buf));
    TEST_ASSERT_EQUAL_INT(8, len);
    TEST_ASSERT_EQUAL_UINT8(0x01, buf[0]);  // slave
    TEST_ASSERT_EQUAL_UINT8(0x06, buf[1]);  // FC
    TEST_ASSERT_EQUAL_UINT8(0x10, buf[2]);  // reg hi
    TEST_ASSERT_EQUAL_UINT8(0x01, buf[3]);  // reg lo
    TEST_ASSERT_EQUAL_UINT8(0x00, buf[4]);  // value hi
    TEST_ASSERT_EQUAL_UINT8(0xC8, buf[5]);  // value lo
}

// --- Write Multiple Registers (FC 0x10) tests ---

void test_build_write_multiple_basic(void) {
    uint16_t values[] = {0x0064, 0x00C8};
    uint8_t buf[32];
    int len = modbusBuildWriteMultiple(1, 0x1000, 2, values, buf, sizeof(buf));
    // Header(7) + data(4) + CRC(2) = 13
    TEST_ASSERT_EQUAL_INT(13, len);
    TEST_ASSERT_EQUAL_UINT8(0x01, buf[0]);  // slave
    TEST_ASSERT_EQUAL_UINT8(0x10, buf[1]);  // FC
    TEST_ASSERT_EQUAL_UINT8(0x10, buf[2]);  // start hi
    TEST_ASSERT_EQUAL_UINT8(0x00, buf[3]);  // start lo
    TEST_ASSERT_EQUAL_UINT8(0x00, buf[4]);  // count hi
    TEST_ASSERT_EQUAL_UINT8(0x02, buf[5]);  // count lo
    TEST_ASSERT_EQUAL_UINT8(0x04, buf[6]);  // byte count
    TEST_ASSERT_EQUAL_UINT8(0x00, buf[7]);  // val1 hi
    TEST_ASSERT_EQUAL_UINT8(0x64, buf[8]);  // val1 lo
    TEST_ASSERT_EQUAL_UINT8(0x00, buf[9]);  // val2 hi
    TEST_ASSERT_EQUAL_UINT8(0xC8, buf[10]); // val2 lo
}

void test_build_write_multiple_null_values(void) {
    uint8_t buf[32];
    int len = modbusBuildWriteMultiple(1, 0x1000, 2, nullptr, buf, sizeof(buf));
    TEST_ASSERT_EQUAL_INT(-1, len);
}

// --- Parse Response tests ---

void test_parse_read_holding_response(void) {
    // Simulate: slave=1, FC=03, byteCount=2, data=[0x00, 0xC8], CRC
    uint8_t frame[16];
    frame[0] = 0x01;  // slave
    frame[1] = 0x03;  // FC
    frame[2] = 0x02;  // byte count
    frame[3] = 0x00;  // data hi
    frame[4] = 0xC8;  // data lo
    uint16_t crc = modbusCrc16(frame, 5);
    frame[5] = crc & 0xFF;
    frame[6] = (crc >> 8) & 0xFF;

    ModbusResponse resp;
    bool ok = modbusParseResponse(frame, 7, resp);
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_EQUAL_UINT8(1, resp.slaveAddr);
    TEST_ASSERT_EQUAL_UINT8(MODBUS_FC_READ_HOLDING, resp.functionCode);
    TEST_ASSERT_FALSE(resp.isException);
    TEST_ASSERT_TRUE(resp.crcValid);
    TEST_ASSERT_EQUAL(2, resp.dataLen);
    TEST_ASSERT_EQUAL_UINT8(0x00, resp.data[0]);
    TEST_ASSERT_EQUAL_UINT8(0xC8, resp.data[1]);
}

void test_parse_exception_response(void) {
    // Exception: slave=1, FC=0x83, exCode=0x02, CRC
    uint8_t frame[16];
    frame[0] = 0x01;
    frame[1] = 0x83;  // FC 03 with exception bit
    frame[2] = 0x02;  // illegal data address
    uint16_t crc = modbusCrc16(frame, 3);
    frame[3] = crc & 0xFF;
    frame[4] = (crc >> 8) & 0xFF;

    ModbusResponse resp;
    bool ok = modbusParseResponse(frame, 5, resp);
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_TRUE(resp.isException);
    TEST_ASSERT_EQUAL_UINT8(MODBUS_FC_READ_HOLDING, resp.functionCode);
    TEST_ASSERT_EQUAL_UINT8(MODBUS_EX_ILLEGAL_ADDRESS, resp.exceptionCode);
    TEST_ASSERT_TRUE(resp.crcValid);
}

void test_parse_bad_crc(void) {
    uint8_t frame[] = {0x01, 0x03, 0x02, 0x00, 0xC8, 0xFF, 0xFF};
    ModbusResponse resp;
    bool ok = modbusParseResponse(frame, 7, resp);
    TEST_ASSERT_TRUE(ok);  // structure valid
    TEST_ASSERT_FALSE(resp.crcValid);  // CRC doesn't match
}

void test_parse_write_single_response(void) {
    // Echo: slave=1, FC=06, reg=0x1001, value=0x00C8, CRC
    uint8_t frame[16];
    frame[0] = 0x01;
    frame[1] = 0x06;
    frame[2] = 0x10;
    frame[3] = 0x01;
    frame[4] = 0x00;
    frame[5] = 0xC8;
    uint16_t crc = modbusCrc16(frame, 6);
    frame[6] = crc & 0xFF;
    frame[7] = (crc >> 8) & 0xFF;

    ModbusResponse resp;
    bool ok = modbusParseResponse(frame, 8, resp);
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_EQUAL_UINT8(MODBUS_FC_WRITE_SINGLE, resp.functionCode);
    TEST_ASSERT_FALSE(resp.isException);
    TEST_ASSERT_TRUE(resp.crcValid);
    TEST_ASSERT_EQUAL(4, resp.dataLen);
}

void test_parse_too_short(void) {
    uint8_t frame[] = {0x01, 0x03, 0xFF};
    ModbusResponse resp;
    TEST_ASSERT_FALSE(modbusParseResponse(frame, 3, resp));
}

void test_parse_null(void) {
    ModbusResponse resp;
    TEST_ASSERT_FALSE(modbusParseResponse(nullptr, 0, resp));
}

// --- Extract Registers tests ---

void test_extract_registers(void) {
    ModbusResponse resp;
    resp.functionCode = MODBUS_FC_READ_HOLDING;
    resp.isException = false;
    resp.data[0] = 0x00; resp.data[1] = 0xC8;  // 200
    resp.data[2] = 0x01; resp.data[3] = 0x90;  // 400
    resp.dataLen = 4;

    uint16_t values[4];
    int count = modbusExtractRegisters(resp, values, 4);
    TEST_ASSERT_EQUAL_INT(2, count);
    TEST_ASSERT_EQUAL_UINT16(0x00C8, values[0]);
    TEST_ASSERT_EQUAL_UINT16(0x0190, values[1]);
}

void test_extract_registers_exception(void) {
    ModbusResponse resp;
    resp.functionCode = MODBUS_FC_READ_HOLDING;
    resp.isException = true;

    uint16_t values[4];
    int count = modbusExtractRegisters(resp, values, 4);
    TEST_ASSERT_EQUAL_INT(-1, count);
}

void test_extract_registers_wrong_fc(void) {
    ModbusResponse resp;
    resp.functionCode = MODBUS_FC_WRITE_SINGLE;
    resp.isException = false;

    uint16_t values[4];
    int count = modbusExtractRegisters(resp, values, 4);
    TEST_ASSERT_EQUAL_INT(-1, count);
}

// --- Expected Response Length tests ---

void test_expected_len_read_holding(void) {
    TEST_ASSERT_EQUAL(7, modbusExpectedResponseLen(MODBUS_FC_READ_HOLDING, 1));
    TEST_ASSERT_EQUAL(15, modbusExpectedResponseLen(MODBUS_FC_READ_HOLDING, 5));
}

void test_expected_len_write_single(void) {
    TEST_ASSERT_EQUAL(8, modbusExpectedResponseLen(MODBUS_FC_WRITE_SINGLE, 0));
}

void test_expected_len_write_multiple(void) {
    TEST_ASSERT_EQUAL(8, modbusExpectedResponseLen(MODBUS_FC_WRITE_MULTIPLE, 0));
}

// --- Round-trip test ---

void test_roundtrip_read_holding(void) {
    // Build request
    uint8_t req[32];
    int reqLen = modbusBuildReadHolding(1, 0x1000, 2, req, sizeof(req));
    TEST_ASSERT_EQUAL_INT(8, reqLen);

    // Simulate response: 2 registers = 4 data bytes
    uint8_t resp[32];
    resp[0] = 0x01;  // slave
    resp[1] = 0x03;  // FC
    resp[2] = 0x04;  // 4 bytes
    resp[3] = 0x01; resp[4] = 0xF4;  // reg1 = 500
    resp[5] = 0x03; resp[6] = 0xE8;  // reg2 = 1000
    uint16_t crc = modbusCrc16(resp, 7);
    resp[7] = crc & 0xFF;
    resp[8] = (crc >> 8) & 0xFF;

    ModbusResponse parsed;
    bool ok = modbusParseResponse(resp, 9, parsed);
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_TRUE(parsed.crcValid);

    uint16_t values[4];
    int numRegs = modbusExtractRegisters(parsed, values, 4);
    TEST_ASSERT_EQUAL_INT(2, numRegs);
    TEST_ASSERT_EQUAL_UINT16(500, values[0]);
    TEST_ASSERT_EQUAL_UINT16(1000, values[1]);
}

int main(int argc, char **argv) {
    UNITY_BEGIN();
    // CRC
    RUN_TEST(test_crc16_empty);
    RUN_TEST(test_crc16_known_value);
    RUN_TEST(test_crc16_single_byte);
    // Build Read Holding
    RUN_TEST(test_build_read_holding_basic);
    RUN_TEST(test_build_read_holding_multiple);
    RUN_TEST(test_build_read_holding_buffer_too_small);
    RUN_TEST(test_build_read_holding_zero_count);
    RUN_TEST(test_build_read_holding_exceeds_max);
    // Build Write Single
    RUN_TEST(test_build_write_single_basic);
    // Build Write Multiple
    RUN_TEST(test_build_write_multiple_basic);
    RUN_TEST(test_build_write_multiple_null_values);
    // Parse Response
    RUN_TEST(test_parse_read_holding_response);
    RUN_TEST(test_parse_exception_response);
    RUN_TEST(test_parse_bad_crc);
    RUN_TEST(test_parse_write_single_response);
    RUN_TEST(test_parse_too_short);
    RUN_TEST(test_parse_null);
    // Extract Registers
    RUN_TEST(test_extract_registers);
    RUN_TEST(test_extract_registers_exception);
    RUN_TEST(test_extract_registers_wrong_fc);
    // Expected Response Length
    RUN_TEST(test_expected_len_read_holding);
    RUN_TEST(test_expected_len_write_single);
    RUN_TEST(test_expected_len_write_multiple);
    // Round-trip
    RUN_TEST(test_roundtrip_read_holding);
    UNITY_END();
    return 0;
}
