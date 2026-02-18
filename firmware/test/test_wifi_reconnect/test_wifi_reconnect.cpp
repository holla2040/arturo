#include <unity.h>
#include "safety/wifi_reconnect.h"

using namespace arturo;

void setUp(void) {}
void tearDown(void) {}

// --- backoffNext tests ---

void test_backoff_next_doubles(void) {
    TEST_ASSERT_EQUAL_INT(2000, backoffNext(1000, 2, 30000));
}

void test_backoff_next_clamps_at_max(void) {
    TEST_ASSERT_EQUAL_INT(30000, backoffNext(16000, 2, 30000));
}

void test_backoff_next_already_at_max(void) {
    TEST_ASSERT_EQUAL_INT(30000, backoffNext(30000, 2, 30000));
}

void test_backoff_next_small_values(void) {
    TEST_ASSERT_EQUAL_INT(200, backoffNext(100, 2, 30000));
}

void test_backoff_next_multiplier_three(void) {
    TEST_ASSERT_EQUAL_INT(3000, backoffNext(1000, 3, 30000));
}

void test_backoff_next_zero_current(void) {
    // Zero current should return 1 (minimum positive)
    TEST_ASSERT_EQUAL_INT(1, backoffNext(0, 2, 30000));
}

void test_backoff_next_multiplier_one(void) {
    // Multiplier of 1 = no growth
    TEST_ASSERT_EQUAL_INT(1000, backoffNext(1000, 1, 30000));
}

void test_backoff_next_exact_max(void) {
    // 15000 * 2 = 30000, exactly max
    TEST_ASSERT_EQUAL_INT(30000, backoffNext(15000, 2, 30000));
}

// --- backoffReady tests ---

void test_backoff_ready_after_interval(void) {
    TEST_ASSERT_TRUE(backoffReady(0, 1000, 1000));
}

void test_backoff_not_ready_before_interval(void) {
    TEST_ASSERT_FALSE(backoffReady(0, 500, 1000));
}

void test_backoff_ready_exact_interval(void) {
    TEST_ASSERT_TRUE(backoffReady(100, 1100, 1000));
}

void test_backoff_ready_well_past(void) {
    TEST_ASSERT_TRUE(backoffReady(0, 5000, 1000));
}

void test_backoff_not_ready_just_under(void) {
    TEST_ASSERT_FALSE(backoffReady(0, 999, 1000));
}

void test_backoff_ready_zero_interval(void) {
    TEST_ASSERT_TRUE(backoffReady(0, 0, 0));
}

// --- backoffStepsToMax tests ---

void test_steps_to_max_default(void) {
    // 1000 -> 2000 -> 4000 -> 8000 -> 16000 -> 30000 = 5 steps
    TEST_ASSERT_EQUAL_INT(5, backoffStepsToMax(1000, 2, 30000));
}

void test_steps_to_max_already_at_max(void) {
    TEST_ASSERT_EQUAL_INT(0, backoffStepsToMax(30000, 2, 30000));
}

void test_steps_to_max_one_step(void) {
    // 15000 -> 30000 = 1 step
    TEST_ASSERT_EQUAL_INT(1, backoffStepsToMax(15000, 2, 30000));
}

void test_steps_to_max_zero_initial(void) {
    TEST_ASSERT_EQUAL_INT(0, backoffStepsToMax(0, 2, 30000));
}

void test_steps_to_max_invalid_multiplier(void) {
    TEST_ASSERT_EQUAL_INT(0, backoffStepsToMax(1000, 1, 30000));
}

// --- queueHasSpace tests ---

void test_queue_has_space_empty(void) {
    // head=0, tail=0, capacity=16 -> count=0, has space
    TEST_ASSERT_TRUE(queueHasSpace(0, 0, 16));
}

void test_queue_has_space_partial(void) {
    // head=0, tail=5, capacity=16 -> count=5, has space
    TEST_ASSERT_TRUE(queueHasSpace(0, 5, 16));
}

void test_queue_no_space_full(void) {
    // head=0, tail=15, capacity=16 -> count=15, no space (capacity-1)
    TEST_ASSERT_FALSE(queueHasSpace(0, 15, 16));
}

void test_queue_has_space_wrapped(void) {
    // head=10, tail=5, capacity=16 -> count=11, has space (11 < 15)
    TEST_ASSERT_TRUE(queueHasSpace(10, 5, 16));
}

void test_queue_no_space_zero_capacity(void) {
    TEST_ASSERT_FALSE(queueHasSpace(0, 0, 0));
}

// --- queueCount tests ---

void test_queue_count_empty(void) {
    TEST_ASSERT_EQUAL_INT(0, queueCount(0, 0, 16));
}

void test_queue_count_some(void) {
    TEST_ASSERT_EQUAL_INT(5, queueCount(0, 5, 16));
}

void test_queue_count_wrapped(void) {
    // head=10, tail=3, capacity=16 -> (3-10+16)%16 = 9
    TEST_ASSERT_EQUAL_INT(9, queueCount(10, 3, 16));
}

void test_queue_count_zero_capacity(void) {
    TEST_ASSERT_EQUAL_INT(0, queueCount(0, 0, 0));
}

// --- queueAdvance tests ---

void test_queue_advance_normal(void) {
    TEST_ASSERT_EQUAL_INT(1, queueAdvance(0, 16));
}

void test_queue_advance_wraps(void) {
    TEST_ASSERT_EQUAL_INT(0, queueAdvance(15, 16));
}

void test_queue_advance_middle(void) {
    TEST_ASSERT_EQUAL_INT(8, queueAdvance(7, 16));
}

// --- outrageDuration tests ---

void test_outage_duration_normal(void) {
    TEST_ASSERT_EQUAL_UINT32(5000, outrageDuration(1000, 6000));
}

void test_outage_duration_immediate(void) {
    TEST_ASSERT_EQUAL_UINT32(0, outrageDuration(1000, 1000));
}

void test_outage_duration_long(void) {
    TEST_ASSERT_EQUAL_UINT32(30000, outrageDuration(0, 30000));
}

// --- Default backoff config ---

void test_default_backoff_config(void) {
    TEST_ASSERT_EQUAL_INT(1000, BACKOFF_DEFAULT.initialMs);
    TEST_ASSERT_EQUAL_INT(30000, BACKOFF_DEFAULT.maxMs);
    TEST_ASSERT_EQUAL_INT(2, BACKOFF_DEFAULT.multiplier);
}

// --- Constants ---

void test_command_queue_max(void) {
    TEST_ASSERT_EQUAL_INT(16, COMMAND_QUEUE_MAX);
}

void test_command_queue_entry_size(void) {
    TEST_ASSERT_EQUAL_INT(256, COMMAND_QUEUE_ENTRY_SIZE);
}

int main(int argc, char **argv) {
    UNITY_BEGIN();

    // backoffNext
    RUN_TEST(test_backoff_next_doubles);
    RUN_TEST(test_backoff_next_clamps_at_max);
    RUN_TEST(test_backoff_next_already_at_max);
    RUN_TEST(test_backoff_next_small_values);
    RUN_TEST(test_backoff_next_multiplier_three);
    RUN_TEST(test_backoff_next_zero_current);
    RUN_TEST(test_backoff_next_multiplier_one);
    RUN_TEST(test_backoff_next_exact_max);

    // backoffReady
    RUN_TEST(test_backoff_ready_after_interval);
    RUN_TEST(test_backoff_not_ready_before_interval);
    RUN_TEST(test_backoff_ready_exact_interval);
    RUN_TEST(test_backoff_ready_well_past);
    RUN_TEST(test_backoff_not_ready_just_under);
    RUN_TEST(test_backoff_ready_zero_interval);

    // backoffStepsToMax
    RUN_TEST(test_steps_to_max_default);
    RUN_TEST(test_steps_to_max_already_at_max);
    RUN_TEST(test_steps_to_max_one_step);
    RUN_TEST(test_steps_to_max_zero_initial);
    RUN_TEST(test_steps_to_max_invalid_multiplier);

    // queueHasSpace
    RUN_TEST(test_queue_has_space_empty);
    RUN_TEST(test_queue_has_space_partial);
    RUN_TEST(test_queue_no_space_full);
    RUN_TEST(test_queue_has_space_wrapped);
    RUN_TEST(test_queue_no_space_zero_capacity);

    // queueCount
    RUN_TEST(test_queue_count_empty);
    RUN_TEST(test_queue_count_some);
    RUN_TEST(test_queue_count_wrapped);
    RUN_TEST(test_queue_count_zero_capacity);

    // queueAdvance
    RUN_TEST(test_queue_advance_normal);
    RUN_TEST(test_queue_advance_wraps);
    RUN_TEST(test_queue_advance_middle);

    // outrageDuration
    RUN_TEST(test_outage_duration_normal);
    RUN_TEST(test_outage_duration_immediate);
    RUN_TEST(test_outage_duration_long);

    // Config/constants
    RUN_TEST(test_default_backoff_config);
    RUN_TEST(test_command_queue_max);
    RUN_TEST(test_command_queue_entry_size);

    UNITY_END();
    return 0;
}
