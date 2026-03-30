#include <unity.h>
#include "safety/watchdog.h"
#include <climits>

using namespace arturo;

void setUp(void) {}
void tearDown(void) {}

// --- Constants ---

void test_watchdog_timeout_constants(void) {
    TEST_ASSERT_EQUAL_UINT32(8, WATCHDOG_TIMEOUT_S);
    TEST_ASSERT_EQUAL_UINT32(8000, WATCHDOG_TIMEOUT_MS);
}

void test_watchdog_feed_interval_constant(void) {
    // Feed interval should be half the timeout
    TEST_ASSERT_EQUAL_UINT32(4000, WATCHDOG_FEED_INTERVAL_MS);
    TEST_ASSERT_EQUAL_UINT32(WATCHDOG_TIMEOUT_MS / 2, WATCHDOG_FEED_INTERVAL_MS);
}

void test_watchdog_late_threshold_constant(void) {
    // Late threshold should be 75% of timeout
    TEST_ASSERT_EQUAL_UINT32(6000, WATCHDOG_LATE_THRESHOLD_MS);
}

// --- watchdogElapsed tests ---

void test_elapsed_normal(void) {
    TEST_ASSERT_EQUAL_UINT32(1000, watchdogElapsed(0, 1000));
}

void test_elapsed_same_time(void) {
    TEST_ASSERT_EQUAL_UINT32(0, watchdogElapsed(500, 500));
}

void test_elapsed_millis_overflow(void) {
    // Simulates millis() wrapping around from near ULONG_MAX to near 0
    // ULONG_MAX = 0xFFFFFFFF on 32-bit
    unsigned long start = ULONG_MAX - 10;  // 0xFFFFFFF5
    unsigned long now = 20;                 // Wrapped around
    // Expected: 31 (11 to overflow + 20 after)
    TEST_ASSERT_EQUAL_UINT32(31, watchdogElapsed(start, now));
}

void test_elapsed_millis_overflow_exact_wrap(void) {
    unsigned long start = ULONG_MAX;
    unsigned long now = 0;
    TEST_ASSERT_EQUAL_UINT32(1, watchdogElapsed(start, now));
}

void test_elapsed_millis_overflow_large_gap(void) {
    unsigned long start = ULONG_MAX - 3999;  // 4000ms before overflow
    unsigned long now = 4000;                  // 4000ms after overflow
    TEST_ASSERT_EQUAL_UINT32(8000, watchdogElapsed(start, now));
}

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
    TEST_ASSERT_TRUE(watchdogFeedDue(0, 100000, 1000));
}

void test_feed_not_due_just_under(void) {
    TEST_ASSERT_FALSE(watchdogFeedDue(0, 999, 1000));
}

void test_feed_due_zero_interval(void) {
    // Zero interval: always due
    TEST_ASSERT_TRUE(watchdogFeedDue(0, 0, 0));
}

void test_feed_due_with_millis_overflow(void) {
    // Feed was at near-max millis, now is past overflow
    unsigned long lastFeed = ULONG_MAX - 2000;
    unsigned long now = 2000;
    // Elapsed = 4001, interval = 4000 -> due
    TEST_ASSERT_TRUE(watchdogFeedDue(lastFeed, now, 4000));
}

void test_feed_not_due_with_millis_overflow(void) {
    // Feed was at near-max millis, now just past overflow
    unsigned long lastFeed = ULONG_MAX - 2000;
    unsigned long now = 1000;
    // Elapsed = 3001, interval = 4000 -> not due
    TEST_ASSERT_FALSE(watchdogFeedDue(lastFeed, now, 4000));
}

void test_feed_due_default_interval(void) {
    // Using the default feed interval constant (4000ms)
    TEST_ASSERT_TRUE(watchdogFeedDue(0, WATCHDOG_FEED_INTERVAL_MS, WATCHDOG_FEED_INTERVAL_MS));
    TEST_ASSERT_FALSE(watchdogFeedDue(0, WATCHDOG_FEED_INTERVAL_MS - 1, WATCHDOG_FEED_INTERVAL_MS));
}

// --- watchdogIsLateFeed tests ---

void test_late_feed_not_late(void) {
    // 3000ms elapsed, threshold at 6000ms -> not late
    TEST_ASSERT_FALSE(watchdogIsLateFeed(0, 3000, WATCHDOG_LATE_THRESHOLD_MS));
}

void test_late_feed_at_threshold(void) {
    // Exactly at the late threshold
    TEST_ASSERT_TRUE(watchdogIsLateFeed(0, 6000, WATCHDOG_LATE_THRESHOLD_MS));
}

void test_late_feed_past_threshold(void) {
    // Well past the late threshold
    TEST_ASSERT_TRUE(watchdogIsLateFeed(0, 7500, WATCHDOG_LATE_THRESHOLD_MS));
}

void test_late_feed_just_under_threshold(void) {
    TEST_ASSERT_FALSE(watchdogIsLateFeed(0, 5999, WATCHDOG_LATE_THRESHOLD_MS));
}

void test_late_feed_with_millis_overflow(void) {
    // Feed at near-max millis, now is past overflow, total elapsed > threshold
    unsigned long lastFeed = ULONG_MAX - 3000;
    unsigned long now = 3001;
    // Elapsed = 6002, threshold = 6000 -> late
    TEST_ASSERT_TRUE(watchdogIsLateFeed(lastFeed, now, WATCHDOG_LATE_THRESHOLD_MS));
}

void test_late_feed_custom_threshold(void) {
    // Custom threshold of 2000ms
    TEST_ASSERT_FALSE(watchdogIsLateFeed(1000, 2500, 2000));
    TEST_ASSERT_TRUE(watchdogIsLateFeed(1000, 3000, 2000));
    TEST_ASSERT_TRUE(watchdogIsLateFeed(1000, 5000, 2000));
}

// --- Feed timing safety margin tests ---

void test_feed_interval_provides_safety_margin(void) {
    // The feed interval (4000ms) should be well under the timeout (8000ms)
    TEST_ASSERT_TRUE(WATCHDOG_FEED_INTERVAL_MS < WATCHDOG_TIMEOUT_MS);
    // The late threshold (6000ms) should be between feed interval and timeout
    TEST_ASSERT_TRUE(WATCHDOG_FEED_INTERVAL_MS < WATCHDOG_LATE_THRESHOLD_MS);
    TEST_ASSERT_TRUE(WATCHDOG_LATE_THRESHOLD_MS < WATCHDOG_TIMEOUT_MS);
}

void test_normal_feed_cycle(void) {
    // Simulate a normal feed cycle: feed every 4000ms
    unsigned long lastFeed = 0;
    unsigned long now = 4000;

    // Feed is due
    TEST_ASSERT_TRUE(watchdogFeedDue(lastFeed, now, WATCHDOG_FEED_INTERVAL_MS));
    // But not late
    TEST_ASSERT_FALSE(watchdogIsLateFeed(lastFeed, now, WATCHDOG_LATE_THRESHOLD_MS));

    // After feeding, next check at 8000ms from original start
    lastFeed = now;
    now = 8000;
    TEST_ASSERT_TRUE(watchdogFeedDue(lastFeed, now, WATCHDOG_FEED_INTERVAL_MS));
    TEST_ASSERT_FALSE(watchdogIsLateFeed(lastFeed, now, WATCHDOG_LATE_THRESHOLD_MS));
}

void test_delayed_feed_cycle(void) {
    // Simulate a delayed feed: feed was due at 4000ms but loop ran late at 6500ms
    unsigned long lastFeed = 0;
    unsigned long now = 6500;

    // Feed is overdue
    TEST_ASSERT_TRUE(watchdogFeedDue(lastFeed, now, WATCHDOG_FEED_INTERVAL_MS));
    // And it's a late feed (past 75% of timeout)
    TEST_ASSERT_TRUE(watchdogIsLateFeed(lastFeed, now, WATCHDOG_LATE_THRESHOLD_MS));
}

void test_critical_feed_near_timeout(void) {
    // 7900ms without feed — dangerously close to 8000ms timeout
    unsigned long lastFeed = 0;
    unsigned long now = 7900;

    TEST_ASSERT_TRUE(watchdogFeedDue(lastFeed, now, WATCHDOG_FEED_INTERVAL_MS));
    TEST_ASSERT_TRUE(watchdogIsLateFeed(lastFeed, now, WATCHDOG_LATE_THRESHOLD_MS));
    // Still under the absolute timeout
    TEST_ASSERT_FALSE(watchdogFeedDue(lastFeed, now, WATCHDOG_TIMEOUT_MS));
}

int main(int argc, char **argv) {
    UNITY_BEGIN();

    // Constants
    RUN_TEST(test_watchdog_timeout_constants);
    RUN_TEST(test_watchdog_feed_interval_constant);
    RUN_TEST(test_watchdog_late_threshold_constant);

    // Elapsed time calculation
    RUN_TEST(test_elapsed_normal);
    RUN_TEST(test_elapsed_same_time);
    RUN_TEST(test_elapsed_millis_overflow);
    RUN_TEST(test_elapsed_millis_overflow_exact_wrap);
    RUN_TEST(test_elapsed_millis_overflow_large_gap);

    // Feed due (original + overflow)
    RUN_TEST(test_feed_due_after_interval);
    RUN_TEST(test_feed_not_due_before_interval);
    RUN_TEST(test_feed_due_exact_interval);
    RUN_TEST(test_feed_due_past_interval);
    RUN_TEST(test_feed_due_large_elapsed);
    RUN_TEST(test_feed_not_due_just_under);
    RUN_TEST(test_feed_due_zero_interval);
    RUN_TEST(test_feed_due_with_millis_overflow);
    RUN_TEST(test_feed_not_due_with_millis_overflow);
    RUN_TEST(test_feed_due_default_interval);

    // Late feed detection
    RUN_TEST(test_late_feed_not_late);
    RUN_TEST(test_late_feed_at_threshold);
    RUN_TEST(test_late_feed_past_threshold);
    RUN_TEST(test_late_feed_just_under_threshold);
    RUN_TEST(test_late_feed_with_millis_overflow);
    RUN_TEST(test_late_feed_custom_threshold);

    // Safety margin and feed cycles
    RUN_TEST(test_feed_interval_provides_safety_margin);
    RUN_TEST(test_normal_feed_cycle);
    RUN_TEST(test_delayed_feed_cycle);
    RUN_TEST(test_critical_feed_near_timeout);

    UNITY_END();
    return 0;
}
