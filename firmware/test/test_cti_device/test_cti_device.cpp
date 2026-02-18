#include <unity.h>
#include <cstring>
#include "devices/cti_device.h"

using namespace arturo;

void setUp(void) {}
void tearDown(void) {}

// --- ctiLookupCommand tests ---

void test_lookup_pump_status(void) {
    const char* cmd = ctiLookupCommand("pump_status");
    TEST_ASSERT_NOT_NULL(cmd);
    TEST_ASSERT_EQUAL_STRING("A?", cmd);
}

void test_lookup_pump_on(void) {
    const char* cmd = ctiLookupCommand("pump_on");
    TEST_ASSERT_NOT_NULL(cmd);
    TEST_ASSERT_EQUAL_STRING("A1", cmd);
}

void test_lookup_pump_off(void) {
    const char* cmd = ctiLookupCommand("pump_off");
    TEST_ASSERT_NOT_NULL(cmd);
    TEST_ASSERT_EQUAL_STRING("A0", cmd);
}

void test_lookup_temp_1st_stage(void) {
    const char* cmd = ctiLookupCommand("get_temp_1st_stage");
    TEST_ASSERT_NOT_NULL(cmd);
    TEST_ASSERT_EQUAL_STRING("J", cmd);
}

void test_lookup_temp_2nd_stage(void) {
    const char* cmd = ctiLookupCommand("get_temp_2nd_stage");
    TEST_ASSERT_NOT_NULL(cmd);
    TEST_ASSERT_EQUAL_STRING("K", cmd);
}

void test_lookup_pump_tc_pressure(void) {
    const char* cmd = ctiLookupCommand("get_pump_tc_pressure");
    TEST_ASSERT_NOT_NULL(cmd);
    TEST_ASSERT_EQUAL_STRING("L", cmd);
}

void test_lookup_aux_tc_pressure(void) {
    const char* cmd = ctiLookupCommand("get_aux_tc_pressure");
    TEST_ASSERT_NOT_NULL(cmd);
    TEST_ASSERT_EQUAL_STRING("M", cmd);
}

void test_lookup_status_1(void) {
    const char* cmd = ctiLookupCommand("get_status_1");
    TEST_ASSERT_NOT_NULL(cmd);
    TEST_ASSERT_EQUAL_STRING("S1", cmd);
}

void test_lookup_status_2(void) {
    const char* cmd = ctiLookupCommand("get_status_2");
    TEST_ASSERT_NOT_NULL(cmd);
    TEST_ASSERT_EQUAL_STRING("S2", cmd);
}

void test_lookup_status_3(void) {
    const char* cmd = ctiLookupCommand("get_status_3");
    TEST_ASSERT_NOT_NULL(cmd);
    TEST_ASSERT_EQUAL_STRING("S3", cmd);
}

void test_lookup_unknown_command(void) {
    TEST_ASSERT_NULL(ctiLookupCommand("nonexistent"));
}

void test_lookup_null_command(void) {
    TEST_ASSERT_NULL(ctiLookupCommand(nullptr));
}

void test_lookup_empty_command(void) {
    TEST_ASSERT_NULL(ctiLookupCommand(""));
}

int main(int argc, char **argv) {
    UNITY_BEGIN();
    RUN_TEST(test_lookup_pump_status);
    RUN_TEST(test_lookup_pump_on);
    RUN_TEST(test_lookup_pump_off);
    RUN_TEST(test_lookup_temp_1st_stage);
    RUN_TEST(test_lookup_temp_2nd_stage);
    RUN_TEST(test_lookup_pump_tc_pressure);
    RUN_TEST(test_lookup_aux_tc_pressure);
    RUN_TEST(test_lookup_status_1);
    RUN_TEST(test_lookup_status_2);
    RUN_TEST(test_lookup_status_3);
    RUN_TEST(test_lookup_unknown_command);
    RUN_TEST(test_lookup_null_command);
    RUN_TEST(test_lookup_empty_command);
    UNITY_END();
    return 0;
}
