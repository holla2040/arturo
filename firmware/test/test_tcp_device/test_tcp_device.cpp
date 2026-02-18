#include <unity.h>
#include "devices/tcp_device.h"

using namespace arturo;

void setUp(void) {}
void tearDown(void) {}

// --- reconnectBackoffMs tests ---

void test_backoff_attempt_zero(void) {
    unsigned long ms = reconnectBackoffMs(0);
    TEST_ASSERT_EQUAL_UINT32(1000, ms);
}

void test_backoff_attempt_one(void) {
    unsigned long ms = reconnectBackoffMs(1);
    TEST_ASSERT_EQUAL_UINT32(2000, ms);
}

void test_backoff_attempt_two(void) {
    unsigned long ms = reconnectBackoffMs(2);
    TEST_ASSERT_EQUAL_UINT32(4000, ms);
}

void test_backoff_attempt_three(void) {
    unsigned long ms = reconnectBackoffMs(3);
    TEST_ASSERT_EQUAL_UINT32(8000, ms);
}

void test_backoff_attempt_four(void) {
    unsigned long ms = reconnectBackoffMs(4);
    TEST_ASSERT_EQUAL_UINT32(16000, ms);
}

void test_backoff_caps_at_max(void) {
    // Default max is 30000
    unsigned long ms = reconnectBackoffMs(5);
    TEST_ASSERT_EQUAL_UINT32(30000, ms);

    ms = reconnectBackoffMs(10);
    TEST_ASSERT_EQUAL_UINT32(30000, ms);

    ms = reconnectBackoffMs(100);
    TEST_ASSERT_EQUAL_UINT32(30000, ms);
}

void test_backoff_custom_max(void) {
    unsigned long ms = reconnectBackoffMs(0, 500);
    TEST_ASSERT_EQUAL_UINT32(500, ms);

    ms = reconnectBackoffMs(3, 5000);
    TEST_ASSERT_EQUAL_UINT32(5000, ms);
}

void test_backoff_negative_attempt(void) {
    unsigned long ms = reconnectBackoffMs(-1);
    TEST_ASSERT_EQUAL_UINT32(0, ms);
}

int main(int argc, char **argv) {
    UNITY_BEGIN();
    RUN_TEST(test_backoff_attempt_zero);
    RUN_TEST(test_backoff_attempt_one);
    RUN_TEST(test_backoff_attempt_two);
    RUN_TEST(test_backoff_attempt_three);
    RUN_TEST(test_backoff_attempt_four);
    RUN_TEST(test_backoff_caps_at_max);
    RUN_TEST(test_backoff_custom_max);
    RUN_TEST(test_backoff_negative_attempt);
    UNITY_END();
    return 0;
}
