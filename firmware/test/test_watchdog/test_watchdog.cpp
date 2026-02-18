#include <unity.h>
#include "safety/watchdog.h"

using namespace arturo;

void setUp(void) {}
void tearDown(void) {}

// --- watchdogFeedDue tests ---

void test_feed_due_after_interval(void) {
    TEST_ASSERT_TRUE(watchdogFeedDue(0, 1000, 1000));
}

void test_feed_not_due_before_interval(void) {
    TEST_ASSERT_FALSE(watchdogFeedDue(0, 500, 1000));
}

void test_feed_due_exact_interval(void) {
    TEST_ASSERT_TRUE(watchdogFeedDue(100, 1100, 1000));
}

void test_feed_due_past_interval(void) {
    TEST_ASSERT_TRUE(watchdogFeedDue(100, 5000, 1000));
}

void test_feed_due_large_elapsed(void) {
    // Large elapsed time — always due
    TEST_ASSERT_TRUE(watchdogFeedDue(0, 100000, 1000));
}

void test_feed_not_due_just_under(void) {
    TEST_ASSERT_FALSE(watchdogFeedDue(0, 999, 1000));
}

void test_feed_due_zero_interval(void) {
    // Zero interval: always due
    TEST_ASSERT_TRUE(watchdogFeedDue(0, 0, 0));
}

// --- Constants ---

void test_watchdog_timeout_constants(void) {
    TEST_ASSERT_EQUAL_UINT32(8, WATCHDOG_TIMEOUT_S);
    TEST_ASSERT_EQUAL_UINT32(8000, WATCHDOG_TIMEOUT_MS);
}

int main(int argc, char **argv) {
    UNITY_BEGIN();
    RUN_TEST(test_feed_due_after_interval);
    RUN_TEST(test_feed_not_due_before_interval);
    RUN_TEST(test_feed_due_exact_interval);
    RUN_TEST(test_feed_due_past_interval);
    RUN_TEST(test_feed_due_large_elapsed);
    RUN_TEST(test_feed_not_due_just_under);
    RUN_TEST(test_feed_due_zero_interval);
    RUN_TEST(test_watchdog_timeout_constants);
    UNITY_END();
    return 0;
}
