#include <unity.h>
#include <cstring>
#include <cstdio>
#include "protocols/cti.h"

using namespace arturo;

void setUp(void) {}
void tearDown(void) {}

// --- ctiChecksum tests ---

void test_checksum_single_char_J(void) {
    // 'J' = 0x4A = 74
    // sum=74, d7d6=1, d1d0=2, xor=3
    // ((74 & 0xFC) + 3) & 0x3F + 0x30 = (72+3)&63 + 48 = 75&63+48 = 11+48 = 59
    uint8_t chk = ctiChecksum("J", 1);
    TEST_ASSERT_EQUAL_UINT8(59, chk);  // ASCII ';'
}

void test_checksum_empty(void) {
    // sum=0, d7d6=0, d1d0=0, xor=0
    // ((0 & 0xFC) + 0) & 0x3F + 0x30 = 0 + 48 = 48
    uint8_t chk = ctiChecksum("", 0);
    TEST_ASSERT_EQUAL_UINT8(0x30, chk);  // ASCII '0'
}

void test_checksum_multi_char_A1(void) {
    // 'A'=65, '1'=49 -> sum=114
    // d7d6 = 114>>6 = 1, d1d0 = 114&3 = 2, xor = 3
    // ((114&0xFC)+3)&0x3F + 0x30 = (112+3)&63+48 = 115&63+48 = 51+48 = 99
    uint8_t chk = ctiChecksum("A1", 2);
    TEST_ASSERT_EQUAL_UINT8(99, chk);  // ASCII 'c'
}

void test_checksum_in_printable_range(void) {
    // Every checksum must be in range 0x30-0x6F
    const char* testStrs[] = {"J", "K", "A1", "A0", "S1", "S2", "S3", "N1", "N2", "D0", "D1"};
    for (int i = 0; i < 11; i++) {
        uint8_t chk = ctiChecksum(testStrs[i], strlen(testStrs[i]));
        TEST_ASSERT_GREATER_OR_EQUAL_UINT8(0x30, chk);
        TEST_ASSERT_LESS_OR_EQUAL_UINT8(0x6F, chk);
    }
}

// --- ctiBuildFrame tests ---

void test_build_frame_single_cmd(void) {
    char buf[32];
    int len = ctiBuildFrame("J", buf, sizeof(buf));
    TEST_ASSERT_GREATER_THAN(0, len);
    // Frame: $J<chk>\r
    TEST_ASSERT_EQUAL_CHAR('$', buf[0]);
    TEST_ASSERT_EQUAL_CHAR('J', buf[1]);
    TEST_ASSERT_EQUAL_CHAR('\r', buf[len - 1]);
    TEST_ASSERT_EQUAL_INT(4, len);  // $ + J + chk + \r
}

void test_build_frame_multi_char_cmd(void) {
    char buf[32];
    int len = ctiBuildFrame("A1", buf, sizeof(buf));
    TEST_ASSERT_GREATER_THAN(0, len);
    TEST_ASSERT_EQUAL_CHAR('$', buf[0]);
    TEST_ASSERT_EQUAL_CHAR('A', buf[1]);
    TEST_ASSERT_EQUAL_CHAR('1', buf[2]);
    TEST_ASSERT_EQUAL_CHAR('\r', buf[len - 1]);
    TEST_ASSERT_EQUAL_INT(5, len);
}

void test_build_frame_buffer_too_small(void) {
    char buf[3];  // too small for $J<chk>\r
    int len = ctiBuildFrame("J", buf, sizeof(buf));
    TEST_ASSERT_EQUAL_INT(-1, len);
}

void test_build_frame_null(void) {
    char buf[32];
    TEST_ASSERT_EQUAL_INT(-1, ctiBuildFrame(nullptr, buf, sizeof(buf)));
    TEST_ASSERT_EQUAL_INT(-1, ctiBuildFrame("J", nullptr, 32));
}

void test_build_frame_checksum_matches(void) {
    // Build a frame and verify the checksum byte is correct
    char buf[32];
    int len = ctiBuildFrame("J", buf, sizeof(buf));
    TEST_ASSERT_GREATER_THAN(0, len);

    uint8_t expectedChk = ctiChecksum("J", 1);
    TEST_ASSERT_EQUAL_UINT8(expectedChk, (uint8_t)buf[2]);
}

// --- ctiParseFrame tests ---

void test_parse_success_response(void) {
    // Build a valid response: $A15.3<chk>\r
    const char* content = "A15.3";
    uint8_t chk = ctiChecksum(content, strlen(content));
    char frame[32];
    snprintf(frame, sizeof(frame), "$%s%c\r", content, (char)chk);

    CtiResponse resp;
    bool ok = ctiParseFrame(frame, strlen(frame), resp);
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_EQUAL(CtiResponseCode::SUCCESS, resp.code);
    TEST_ASSERT_EQUAL_STRING("15.3", resp.data);
    TEST_ASSERT_EQUAL(4, resp.dataLen);
    TEST_ASSERT_TRUE(resp.checksumValid);
}

void test_parse_error_response(void) {
    const char* content = "E";
    uint8_t chk = ctiChecksum(content, strlen(content));
    char frame[32];
    snprintf(frame, sizeof(frame), "$%s%c\r", content, (char)chk);

    CtiResponse resp;
    bool ok = ctiParseFrame(frame, strlen(frame), resp);
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_EQUAL(CtiResponseCode::INVALID_COMMAND, resp.code);
    TEST_ASSERT_EQUAL(0, resp.dataLen);
    TEST_ASSERT_TRUE(resp.checksumValid);
}

void test_parse_power_fail_response(void) {
    const char* content = "B22.7";
    uint8_t chk = ctiChecksum(content, strlen(content));
    char frame[32];
    snprintf(frame, sizeof(frame), "$%s%c\r", content, (char)chk);

    CtiResponse resp;
    bool ok = ctiParseFrame(frame, strlen(frame), resp);
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_EQUAL(CtiResponseCode::SUCCESS_POWER_FAIL, resp.code);
    TEST_ASSERT_EQUAL_STRING("22.7", resp.data);
    TEST_ASSERT_TRUE(resp.checksumValid);
}

void test_parse_bad_checksum(void) {
    // Valid structure but wrong checksum
    char frame[] = "$A15.3X\r";
    CtiResponse resp;
    bool ok = ctiParseFrame(frame, strlen(frame), resp);
    TEST_ASSERT_TRUE(ok);  // structure is valid
    TEST_ASSERT_FALSE(resp.checksumValid);  // checksum doesn't match
}

void test_parse_too_short(void) {
    CtiResponse resp;
    TEST_ASSERT_FALSE(ctiParseFrame("$A\r", 3, resp));  // needs at least 4 chars
}

void test_parse_no_dollar(void) {
    CtiResponse resp;
    TEST_ASSERT_FALSE(ctiParseFrame("A15.3@\r", 7, resp));
}

void test_parse_no_cr(void) {
    CtiResponse resp;
    TEST_ASSERT_FALSE(ctiParseFrame("$A15.3@", 7, resp));
}

void test_parse_null(void) {
    CtiResponse resp;
    TEST_ASSERT_FALSE(ctiParseFrame(nullptr, 0, resp));
}

// --- ctiIsDataValid / ctiIsSuccess tests ---

void test_data_valid_success(void) {
    TEST_ASSERT_TRUE(ctiIsDataValid(CtiResponseCode::SUCCESS));
    TEST_ASSERT_TRUE(ctiIsDataValid(CtiResponseCode::SUCCESS_POWER_FAIL));
}

void test_data_invalid_errors(void) {
    TEST_ASSERT_FALSE(ctiIsDataValid(CtiResponseCode::INVALID_COMMAND));
    TEST_ASSERT_FALSE(ctiIsDataValid(CtiResponseCode::INVALID_POWER_FAIL));
    TEST_ASSERT_FALSE(ctiIsDataValid(CtiResponseCode::INTERLOCKS_ACTIVE));
    TEST_ASSERT_FALSE(ctiIsDataValid(CtiResponseCode::INTERLOCKS_POWER));
    TEST_ASSERT_FALSE(ctiIsDataValid(CtiResponseCode::UNKNOWN));
}

// --- ctiParseStatusByte tests ---

void test_parse_status_hex_39(void) {
    uint8_t status;
    bool ok = ctiParseStatusByte("39", status);
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_EQUAL_UINT8(0x39, status);  // 57 decimal, NOT 39
}

void test_parse_status_hex_FF(void) {
    uint8_t status;
    bool ok = ctiParseStatusByte("FF", status);
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_EQUAL_UINT8(0xFF, status);
}

void test_parse_status_hex_00(void) {
    uint8_t status;
    bool ok = ctiParseStatusByte("00", status);
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_EQUAL_UINT8(0, status);
}

void test_parse_status_hex_lowercase(void) {
    uint8_t status;
    bool ok = ctiParseStatusByte("ab", status);
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_EQUAL_UINT8(0xAB, status);
}

void test_parse_status_null(void) {
    uint8_t status;
    TEST_ASSERT_FALSE(ctiParseStatusByte(nullptr, status));
}

void test_parse_status_empty(void) {
    uint8_t status;
    TEST_ASSERT_FALSE(ctiParseStatusByte("", status));
}

// --- Round-trip test ---

void test_roundtrip_build_parse(void) {
    // Build a command frame
    char txFrame[32];
    int txLen = ctiBuildFrame("S1", txFrame, sizeof(txFrame));
    TEST_ASSERT_GREATER_THAN(0, txLen);

    // Simulate a response: success with hex status "39"
    const char* respContent = "A39";
    uint8_t chk = ctiChecksum(respContent, strlen(respContent));
    char rxFrame[32];
    snprintf(rxFrame, sizeof(rxFrame), "$%s%c\r", respContent, (char)chk);

    CtiResponse resp;
    bool ok = ctiParseFrame(rxFrame, strlen(rxFrame), resp);
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_TRUE(resp.checksumValid);
    TEST_ASSERT_TRUE(ctiIsSuccess(resp.code));

    // Parse the status byte (hex)
    uint8_t status;
    ok = ctiParseStatusByte(resp.data, status);
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_EQUAL_UINT8(0x39, status);
}

int main(int argc, char **argv) {
    UNITY_BEGIN();
    // Checksum
    RUN_TEST(test_checksum_single_char_J);
    RUN_TEST(test_checksum_empty);
    RUN_TEST(test_checksum_multi_char_A1);
    RUN_TEST(test_checksum_in_printable_range);
    // Build frame
    RUN_TEST(test_build_frame_single_cmd);
    RUN_TEST(test_build_frame_multi_char_cmd);
    RUN_TEST(test_build_frame_buffer_too_small);
    RUN_TEST(test_build_frame_null);
    RUN_TEST(test_build_frame_checksum_matches);
    // Parse frame
    RUN_TEST(test_parse_success_response);
    RUN_TEST(test_parse_error_response);
    RUN_TEST(test_parse_power_fail_response);
    RUN_TEST(test_parse_bad_checksum);
    RUN_TEST(test_parse_too_short);
    RUN_TEST(test_parse_no_dollar);
    RUN_TEST(test_parse_no_cr);
    RUN_TEST(test_parse_null);
    // Data valid / success
    RUN_TEST(test_data_valid_success);
    RUN_TEST(test_data_invalid_errors);
    // Status byte parsing
    RUN_TEST(test_parse_status_hex_39);
    RUN_TEST(test_parse_status_hex_FF);
    RUN_TEST(test_parse_status_hex_00);
    RUN_TEST(test_parse_status_hex_lowercase);
    RUN_TEST(test_parse_status_null);
    RUN_TEST(test_parse_status_empty);
    // Round-trip
    RUN_TEST(test_roundtrip_build_parse);
    UNITY_END();
    return 0;
}
