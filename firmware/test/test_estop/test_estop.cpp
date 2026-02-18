#include <unity.h>
#include "safety/estop.h"

using namespace arturo;

void setUp(void) {}
void tearDown(void) {}

// --- estopDebounce tests ---

void test_debounce_stable_reading(void) {
    // Same reading as last time — always stable
    TEST_ASSERT_TRUE(estopDebounce(true, true, 0, 100, 50));
    TEST_ASSERT_TRUE(estopDebounce(false, false, 0, 100, 50));
}

void test_debounce_changed_after_delay(void) {
    // Reading changed and enough time has passed
    TEST_ASSERT_TRUE(estopDebounce(true, false, 0, 50, 50));
    TEST_ASSERT_TRUE(estopDebounce(true, false, 0, 100, 50));
}

void test_debounce_changed_too_soon(void) {
    // Reading changed but not enough time since last change
    TEST_ASSERT_FALSE(estopDebounce(true, false, 0, 30, 50));
}

void test_debounce_exact_threshold(void) {
    // Exactly at the debounce threshold — should be stable
    TEST_ASSERT_TRUE(estopDebounce(true, false, 100, 150, 50));
}

void test_debounce_just_under_threshold(void) {
    TEST_ASSERT_FALSE(estopDebounce(true, false, 100, 149, 50));
}

void test_debounce_zero_debounce_time(void) {
    // Zero debounce = always stable
    TEST_ASSERT_TRUE(estopDebounce(true, false, 0, 0, 0));
}

// --- EStopState enum values ---

void test_estop_state_clear(void) {
    TEST_ASSERT_EQUAL_UINT8(0, (uint8_t)EStopState::CLEAR);
}

void test_estop_state_tripped(void) {
    TEST_ASSERT_EQUAL_UINT8(1, (uint8_t)EStopState::TRIPPED);
}

// --- EStopEvent enum values ---

void test_estop_event_values(void) {
    TEST_ASSERT_EQUAL_UINT8(0, (uint8_t)EStopEvent::BUTTON_PRESSED);
    TEST_ASSERT_EQUAL_UINT8(1, (uint8_t)EStopEvent::REMOTE_RECEIVED);
    TEST_ASSERT_EQUAL_UINT8(2, (uint8_t)EStopEvent::MANUAL_CLEAR);
}

// --- Default config ---

void test_default_config(void) {
    TEST_ASSERT_EQUAL_INT(-1, ESTOP_DEFAULT_CONFIG.buttonPin);
    TEST_ASSERT_TRUE(ESTOP_DEFAULT_CONFIG.activeLow);
    TEST_ASSERT_EQUAL_INT(-1, ESTOP_DEFAULT_CONFIG.ledPin);
    TEST_ASSERT_EQUAL_UINT32(50, ESTOP_DEFAULT_CONFIG.debounceMs);
}

int main(int argc, char **argv) {
    UNITY_BEGIN();
    // Debounce
    RUN_TEST(test_debounce_stable_reading);
    RUN_TEST(test_debounce_changed_after_delay);
    RUN_TEST(test_debounce_changed_too_soon);
    RUN_TEST(test_debounce_exact_threshold);
    RUN_TEST(test_debounce_just_under_threshold);
    RUN_TEST(test_debounce_zero_debounce_time);
    // State/event enums
    RUN_TEST(test_estop_state_clear);
    RUN_TEST(test_estop_state_tripped);
    RUN_TEST(test_estop_event_values);
    // Default config
    RUN_TEST(test_default_config);
    UNITY_END();
    return 0;
}
