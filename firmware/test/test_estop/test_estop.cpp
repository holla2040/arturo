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

// --- E-stop edge cases: debounce boundary conditions ---

void test_debounce_rapid_toggle(void) {
    // Rapid toggling within debounce window — should reject all transitions
    // Simulates contact bounce: rapid true/false within 50ms
    TEST_ASSERT_FALSE(estopDebounce(true, false, 100, 105, 50));   // 5ms after change
    TEST_ASSERT_FALSE(estopDebounce(false, true, 105, 110, 50));   // 5ms after change
    TEST_ASSERT_FALSE(estopDebounce(true, false, 110, 115, 50));   // 5ms after change
}

void test_debounce_large_time_gap(void) {
    // Very large time gap — should always be stable
    TEST_ASSERT_TRUE(estopDebounce(true, false, 0, 1000000, 50));
    TEST_ASSERT_TRUE(estopDebounce(false, true, 0, 1000000, 50));
}

void test_debounce_max_unsigned_long_wrap(void) {
    // Test near unsigned long overflow (millis() wraps every ~49 days)
    unsigned long nearMax = 0xFFFFFFFF - 10;
    unsigned long wrapped = 40;  // After overflow
    // elapsed = wrapped - nearMax = 40 - (2^32 - 11) = wraps to ~51 (due to unsigned arithmetic)
    TEST_ASSERT_TRUE(estopDebounce(true, false, nearMax, wrapped, 50));
}

void test_debounce_same_timestamp(void) {
    // Reading changed at exactly the same millisecond — elapsed is 0, needs 50ms
    TEST_ASSERT_FALSE(estopDebounce(true, false, 100, 100, 50));
}

void test_debounce_one_ms_debounce(void) {
    // Very short debounce time (1ms)
    TEST_ASSERT_FALSE(estopDebounce(true, false, 100, 100, 1)); // 0ms elapsed, need 1ms
    TEST_ASSERT_TRUE(estopDebounce(true, false, 100, 101, 1));  // 1ms elapsed, need 1ms
}

// --- EStopConfig custom values ---

void test_custom_config_active_high(void) {
    // Verify config struct can hold active-high configuration
    EStopConfig config = { 15, false, 2, 100 };
    TEST_ASSERT_EQUAL_INT(15, config.buttonPin);
    TEST_ASSERT_FALSE(config.activeLow);
    TEST_ASSERT_EQUAL_INT(2, config.ledPin);
    TEST_ASSERT_EQUAL_UINT32(100, config.debounceMs);
}

void test_custom_config_no_led(void) {
    // Config without LED pin
    EStopConfig config = { 4, true, -1, 50 };
    TEST_ASSERT_EQUAL_INT(-1, config.ledPin);
}

void test_custom_config_zero_pin(void) {
    // GPIO 0 is valid on ESP32
    EStopConfig config = { 0, true, -1, 50 };
    TEST_ASSERT_EQUAL_INT(0, config.buttonPin);
}

// --- State transition verification ---

void test_state_enum_distinctness(void) {
    // CLEAR and TRIPPED must be distinct values
    TEST_ASSERT_NOT_EQUAL((uint8_t)EStopState::CLEAR, (uint8_t)EStopState::TRIPPED);
}

void test_event_enum_completeness(void) {
    // All three event types should have sequential values
    TEST_ASSERT_EQUAL_UINT8(0, (uint8_t)EStopEvent::BUTTON_PRESSED);
    TEST_ASSERT_EQUAL_UINT8(1, (uint8_t)EStopEvent::REMOTE_RECEIVED);
    TEST_ASSERT_EQUAL_UINT8(2, (uint8_t)EStopEvent::MANUAL_CLEAR);
    // Verify they fit in uint8_t
    TEST_ASSERT_TRUE((uint8_t)EStopEvent::MANUAL_CLEAR < 255);
}

// --- Debounce sequence simulation ---

void test_debounce_full_press_sequence(void) {
    // Simulate a complete button press with bouncing:
    // t=0:    button idle (false/false) - stable
    // t=10:   first contact (true/false) - changed, too soon
    // t=15:   bounce back (false/true) - changed, too soon
    // t=20:   contact again (true/false) - changed, too soon
    // t=75:   settled (true/true) - stable (same reading)

    // Idle — stable
    TEST_ASSERT_TRUE(estopDebounce(false, false, 0, 0, 50));

    // First contact at t=10, last change at t=0: 10ms < 50ms
    TEST_ASSERT_FALSE(estopDebounce(true, false, 0, 10, 50));

    // Bounce back at t=15, last change at t=10: 5ms < 50ms
    TEST_ASSERT_FALSE(estopDebounce(false, true, 10, 15, 50));

    // Contact again at t=20, last change at t=15: 5ms < 50ms
    TEST_ASSERT_FALSE(estopDebounce(true, false, 15, 20, 50));

    // Settled at t=75, last change at t=20: same reading = stable
    TEST_ASSERT_TRUE(estopDebounce(true, true, 20, 75, 50));

    // Or if reading still counts as changed at t=70: 50ms = 50ms (threshold met)
    TEST_ASSERT_TRUE(estopDebounce(true, false, 20, 70, 50));
}

void test_debounce_full_release_sequence(void) {
    // Simulate button release with bouncing:
    // Button was pressed (true), now releasing

    // Held — stable
    TEST_ASSERT_TRUE(estopDebounce(true, true, 0, 100, 50));

    // First release contact at t=100, last change at t=100: 0ms < 50ms
    TEST_ASSERT_FALSE(estopDebounce(false, true, 100, 100, 50));

    // Stable release at t=155, last change at t=100: 55ms >= 50ms
    TEST_ASSERT_TRUE(estopDebounce(false, true, 100, 155, 50));
}

int main(int argc, char **argv) {
    UNITY_BEGIN();
    // Debounce — basic
    RUN_TEST(test_debounce_stable_reading);
    RUN_TEST(test_debounce_changed_after_delay);
    RUN_TEST(test_debounce_changed_too_soon);
    RUN_TEST(test_debounce_exact_threshold);
    RUN_TEST(test_debounce_just_under_threshold);
    RUN_TEST(test_debounce_zero_debounce_time);
    // Debounce — edge cases
    RUN_TEST(test_debounce_rapid_toggle);
    RUN_TEST(test_debounce_large_time_gap);
    RUN_TEST(test_debounce_max_unsigned_long_wrap);
    RUN_TEST(test_debounce_same_timestamp);
    RUN_TEST(test_debounce_one_ms_debounce);
    // Debounce — full sequences
    RUN_TEST(test_debounce_full_press_sequence);
    RUN_TEST(test_debounce_full_release_sequence);
    // State/event enums
    RUN_TEST(test_estop_state_clear);
    RUN_TEST(test_estop_state_tripped);
    RUN_TEST(test_estop_event_values);
    RUN_TEST(test_state_enum_distinctness);
    RUN_TEST(test_event_enum_completeness);
    // Config
    RUN_TEST(test_default_config);
    RUN_TEST(test_custom_config_active_high);
    RUN_TEST(test_custom_config_no_led);
    RUN_TEST(test_custom_config_zero_pin);
    UNITY_END();
    return 0;
}
