#include <unity.h>
#include "safety/ota_update.h"
#include <cstring>

using namespace arturo;

void setUp(void) {}
void tearDown(void) {}

// --- Semver comparison tests ---

void test_semver_equal(void) {
    TEST_ASSERT_EQUAL_INT(0, compareSemver("1.0.0", "1.0.0"));
}

void test_semver_major_less(void) {
    TEST_ASSERT_TRUE(compareSemver("1.0.0", "2.0.0") < 0);
}

void test_semver_major_greater(void) {
    TEST_ASSERT_TRUE(compareSemver("2.0.0", "1.0.0") > 0);
}

void test_semver_minor_less(void) {
    TEST_ASSERT_TRUE(compareSemver("1.0.0", "1.1.0") < 0);
}

void test_semver_minor_greater(void) {
    TEST_ASSERT_TRUE(compareSemver("1.2.0", "1.1.0") > 0);
}

void test_semver_patch_less(void) {
    TEST_ASSERT_TRUE(compareSemver("1.0.0", "1.0.1") < 0);
}

void test_semver_patch_greater(void) {
    TEST_ASSERT_TRUE(compareSemver("1.0.2", "1.0.1") > 0);
}

void test_semver_complex(void) {
    TEST_ASSERT_TRUE(compareSemver("1.9.9", "2.0.0") < 0);
    TEST_ASSERT_TRUE(compareSemver("10.0.0", "9.99.99") > 0);
}

void test_semver_null(void) {
    TEST_ASSERT_EQUAL_INT(0, compareSemver(nullptr, nullptr));
    TEST_ASSERT_EQUAL_INT(0, compareSemver("1.0.0", nullptr));
    TEST_ASSERT_EQUAL_INT(0, compareSemver(nullptr, "1.0.0"));
}

// --- Semver validation tests ---

void test_valid_semver(void) {
    TEST_ASSERT_TRUE(isValidSemver("1.0.0"));
    TEST_ASSERT_TRUE(isValidSemver("0.0.1"));
    TEST_ASSERT_TRUE(isValidSemver("10.20.30"));
    TEST_ASSERT_TRUE(isValidSemver("99.99.99"));
}

void test_invalid_semver(void) {
    TEST_ASSERT_FALSE(isValidSemver(nullptr));
    TEST_ASSERT_FALSE(isValidSemver(""));
    TEST_ASSERT_FALSE(isValidSemver("1.0"));
    TEST_ASSERT_FALSE(isValidSemver("1"));
    TEST_ASSERT_FALSE(isValidSemver("1.0.0.0"));
    TEST_ASSERT_FALSE(isValidSemver("abc"));
    TEST_ASSERT_FALSE(isValidSemver("1.0.0-beta"));
    TEST_ASSERT_FALSE(isValidSemver("v1.0.0"));
}

// --- SHA256 validation tests ---

void test_valid_sha256(void) {
    TEST_ASSERT_TRUE(isValidSHA256Hex("a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1"));
    TEST_ASSERT_TRUE(isValidSHA256Hex("0000000000000000000000000000000000000000000000000000000000000000"));
    TEST_ASSERT_TRUE(isValidSHA256Hex("abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"));
}

void test_invalid_sha256(void) {
    TEST_ASSERT_FALSE(isValidSHA256Hex(nullptr));
    TEST_ASSERT_FALSE(isValidSHA256Hex(""));
    // Too short
    TEST_ASSERT_FALSE(isValidSHA256Hex("abcdef"));
    // Too long
    TEST_ASSERT_FALSE(isValidSHA256Hex("a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1a"));
    // Uppercase not allowed
    TEST_ASSERT_FALSE(isValidSHA256Hex("A3F2B8C9D4E5F6A7B8C9D0E1F2A3B4C5D6E7F8A9B0C1D2E3F4A5B6C7D8E9F0A1"));
    // Invalid characters
    TEST_ASSERT_FALSE(isValidSHA256Hex("g3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1"));
}

// --- URL validation tests ---

void test_valid_url(void) {
    TEST_ASSERT_TRUE(isValidFirmwareURL("http://192.168.1.10:8080/firmware/v1.0.0.bin"));
    TEST_ASSERT_TRUE(isValidFirmwareURL("https://example.com/firmware.bin"));
    TEST_ASSERT_TRUE(isValidFirmwareURL("http://x"));
}

void test_invalid_url(void) {
    TEST_ASSERT_FALSE(isValidFirmwareURL(nullptr));
    TEST_ASSERT_FALSE(isValidFirmwareURL(""));
    TEST_ASSERT_FALSE(isValidFirmwareURL("ftp://example.com/firmware.bin"));
    TEST_ASSERT_FALSE(isValidFirmwareURL("not-a-url"));
    TEST_ASSERT_FALSE(isValidFirmwareURL("http:/"));
}

// --- OTA state/error string tests ---

void test_ota_state_strings(void) {
    TEST_ASSERT_EQUAL_STRING("idle", otaStateToString(OTAState::IDLE));
    TEST_ASSERT_EQUAL_STRING("checking", otaStateToString(OTAState::CHECKING));
    TEST_ASSERT_EQUAL_STRING("downloading", otaStateToString(OTAState::DOWNLOADING));
    TEST_ASSERT_EQUAL_STRING("verifying", otaStateToString(OTAState::VERIFYING));
    TEST_ASSERT_EQUAL_STRING("applying", otaStateToString(OTAState::APPLYING));
    TEST_ASSERT_EQUAL_STRING("rebooting", otaStateToString(OTAState::REBOOTING));
    TEST_ASSERT_EQUAL_STRING("failed", otaStateToString(OTAState::FAILED));
}

void test_ota_error_strings(void) {
    TEST_ASSERT_EQUAL_STRING("none", otaErrorToString(OTAError::NONE));
    TEST_ASSERT_EQUAL_STRING("invalid_url", otaErrorToString(OTAError::INVALID_URL));
    TEST_ASSERT_EQUAL_STRING("invalid_version", otaErrorToString(OTAError::INVALID_VERSION));
    TEST_ASSERT_EQUAL_STRING("invalid_sha256", otaErrorToString(OTAError::INVALID_SHA256));
    TEST_ASSERT_EQUAL_STRING("same_version", otaErrorToString(OTAError::SAME_VERSION));
    TEST_ASSERT_EQUAL_STRING("download_failed", otaErrorToString(OTAError::DOWNLOAD_FAILED));
    TEST_ASSERT_EQUAL_STRING("checksum_mismatch", otaErrorToString(OTAError::CHECKSUM_MISMATCH));
    TEST_ASSERT_EQUAL_STRING("flash_write_failed", otaErrorToString(OTAError::FLASH_WRITE_FAILED));
    TEST_ASSERT_EQUAL_STRING("rollback_active", otaErrorToString(OTAError::ROLLBACK_ACTIVE));
    TEST_ASSERT_EQUAL_STRING("insufficient_space", otaErrorToString(OTAError::INSUFFICIENT_SPACE));
    TEST_ASSERT_EQUAL_STRING("busy", otaErrorToString(OTAError::BUSY));
}

// --- parseOTAPayload tests ---

void test_parse_ota_payload_valid(void) {
    OTARequest req;
    bool ok = parseOTAPayload(
        "http://192.168.1.10:8080/firmware/v1.1.0.bin",
        "1.1.0",
        "a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1",
        false,
        req
    );
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_EQUAL_STRING("http://192.168.1.10:8080/firmware/v1.1.0.bin", req.firmwareUrl);
    TEST_ASSERT_EQUAL_STRING("1.1.0", req.version);
    TEST_ASSERT_EQUAL_STRING("a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1", req.sha256);
    TEST_ASSERT_FALSE(req.force);
}

void test_parse_ota_payload_force(void) {
    OTARequest req;
    bool ok = parseOTAPayload(
        "http://example.com/fw.bin",
        "2.0.0",
        "0000000000000000000000000000000000000000000000000000000000000000",
        true,
        req
    );
    TEST_ASSERT_TRUE(ok);
    TEST_ASSERT_TRUE(req.force);
}

void test_parse_ota_payload_null_url(void) {
    OTARequest req;
    TEST_ASSERT_FALSE(parseOTAPayload(nullptr, "1.0.0",
        "0000000000000000000000000000000000000000000000000000000000000000", false, req));
}

void test_parse_ota_payload_null_version(void) {
    OTARequest req;
    TEST_ASSERT_FALSE(parseOTAPayload("http://x", nullptr,
        "0000000000000000000000000000000000000000000000000000000000000000", false, req));
}

void test_parse_ota_payload_null_sha256(void) {
    OTARequest req;
    TEST_ASSERT_FALSE(parseOTAPayload("http://x", "1.0.0", nullptr, false, req));
}

void test_parse_ota_payload_empty_url(void) {
    OTARequest req;
    TEST_ASSERT_FALSE(parseOTAPayload("", "1.0.0",
        "0000000000000000000000000000000000000000000000000000000000000000", false, req));
}

void test_parse_ota_payload_empty_version(void) {
    OTARequest req;
    TEST_ASSERT_FALSE(parseOTAPayload("http://x", "",
        "0000000000000000000000000000000000000000000000000000000000000000", false, req));
}

void test_parse_ota_payload_empty_sha256(void) {
    OTARequest req;
    TEST_ASSERT_FALSE(parseOTAPayload("http://x", "1.0.0", "", false, req));
}

// --- validateOTARequest tests ---

void test_validate_ota_request_valid(void) {
    OTARequest req;
    parseOTAPayload(
        "http://192.168.1.10/fw.bin",
        "1.1.0",
        "a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1",
        false, req
    );
    OTAError err;
    TEST_ASSERT_TRUE(validateOTARequest(req, err));
    TEST_ASSERT_EQUAL(OTAError::NONE, err);
}

void test_validate_ota_request_bad_url(void) {
    OTARequest req;
    parseOTAPayload(
        "ftp://bad",
        "1.0.0",
        "a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1",
        false, req
    );
    OTAError err;
    TEST_ASSERT_FALSE(validateOTARequest(req, err));
    TEST_ASSERT_EQUAL(OTAError::INVALID_URL, err);
}

void test_validate_ota_request_bad_version(void) {
    OTARequest req;
    parseOTAPayload(
        "http://ok",
        "bad",
        "a3f2b8c9d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1",
        false, req
    );
    OTAError err;
    TEST_ASSERT_FALSE(validateOTARequest(req, err));
    TEST_ASSERT_EQUAL(OTAError::INVALID_VERSION, err);
}

void test_validate_ota_request_bad_sha256(void) {
    OTARequest req;
    parseOTAPayload(
        "http://ok",
        "1.0.0",
        "tooshort",
        false, req
    );
    OTAError err;
    TEST_ASSERT_FALSE(validateOTARequest(req, err));
    TEST_ASSERT_EQUAL(OTAError::INVALID_SHA256, err);
}

// --- Semver edge cases ---

void test_semver_zero_versions(void) {
    TEST_ASSERT_EQUAL_INT(0, compareSemver("0.0.0", "0.0.0"));
    TEST_ASSERT_TRUE(compareSemver("0.0.0", "0.0.1") < 0);
}

void test_semver_large_numbers(void) {
    TEST_ASSERT_TRUE(compareSemver("100.200.300", "100.200.301") < 0);
    TEST_ASSERT_TRUE(compareSemver("100.201.0", "100.200.999") > 0);
}

int main(int argc, char **argv) {
    UNITY_BEGIN();

    // Semver comparison
    RUN_TEST(test_semver_equal);
    RUN_TEST(test_semver_major_less);
    RUN_TEST(test_semver_major_greater);
    RUN_TEST(test_semver_minor_less);
    RUN_TEST(test_semver_minor_greater);
    RUN_TEST(test_semver_patch_less);
    RUN_TEST(test_semver_patch_greater);
    RUN_TEST(test_semver_complex);
    RUN_TEST(test_semver_null);

    // Semver validation
    RUN_TEST(test_valid_semver);
    RUN_TEST(test_invalid_semver);

    // SHA256 validation
    RUN_TEST(test_valid_sha256);
    RUN_TEST(test_invalid_sha256);

    // URL validation
    RUN_TEST(test_valid_url);
    RUN_TEST(test_invalid_url);

    // State/error strings
    RUN_TEST(test_ota_state_strings);
    RUN_TEST(test_ota_error_strings);

    // parseOTAPayload
    RUN_TEST(test_parse_ota_payload_valid);
    RUN_TEST(test_parse_ota_payload_force);
    RUN_TEST(test_parse_ota_payload_null_url);
    RUN_TEST(test_parse_ota_payload_null_version);
    RUN_TEST(test_parse_ota_payload_null_sha256);
    RUN_TEST(test_parse_ota_payload_empty_url);
    RUN_TEST(test_parse_ota_payload_empty_version);
    RUN_TEST(test_parse_ota_payload_empty_sha256);

    // validateOTARequest
    RUN_TEST(test_validate_ota_request_valid);
    RUN_TEST(test_validate_ota_request_bad_url);
    RUN_TEST(test_validate_ota_request_bad_version);
    RUN_TEST(test_validate_ota_request_bad_sha256);

    // Edge cases
    RUN_TEST(test_semver_zero_versions);
    RUN_TEST(test_semver_large_numbers);

    UNITY_END();
    return 0;
}
