#include <unity.h>
#include "devices/relay_controller.h"

using namespace arturo;

void setUp(void) {}
void tearDown(void) {}

// --- isValidChannel tests ---

void test_valid_channel_zero(void) {
    TEST_ASSERT_TRUE(isValidChannel(0, 4));
}

void test_valid_channel_last(void) {
    TEST_ASSERT_TRUE(isValidChannel(3, 4));
}

void test_invalid_channel_negative(void) {
    TEST_ASSERT_FALSE(isValidChannel(-1, 4));
}

void test_invalid_channel_equal_to_count(void) {
    TEST_ASSERT_FALSE(isValidChannel(4, 4));
}

void test_invalid_channel_exceeds_count(void) {
    TEST_ASSERT_FALSE(isValidChannel(5, 4));
}

void test_invalid_channel_zero_count(void) {
    TEST_ASSERT_FALSE(isValidChannel(0, 0));
}

void test_invalid_channel_exceeds_max(void) {
    TEST_ASSERT_FALSE(isValidChannel(0, RELAY_MAX_CHANNELS + 1));
}

// --- relayStateToGpioLevel tests ---

void test_gpio_level_on_active_high(void) {
    TEST_ASSERT_EQUAL_INT(1, relayStateToGpioLevel(RelayState::ON, true));
}

void test_gpio_level_off_active_high(void) {
    TEST_ASSERT_EQUAL_INT(0, relayStateToGpioLevel(RelayState::OFF, true));
}

void test_gpio_level_on_active_low(void) {
    // Active-low: ON means pull LOW
    TEST_ASSERT_EQUAL_INT(0, relayStateToGpioLevel(RelayState::ON, false));
}

void test_gpio_level_off_active_low(void) {
    // Active-low: OFF means pull HIGH
    TEST_ASSERT_EQUAL_INT(1, relayStateToGpioLevel(RelayState::OFF, false));
}

// --- RelayState enum values ---

void test_relay_state_off_is_zero(void) {
    TEST_ASSERT_EQUAL_UINT8(0, (uint8_t)RelayState::OFF);
}

void test_relay_state_on_is_one(void) {
    TEST_ASSERT_EQUAL_UINT8(1, (uint8_t)RelayState::ON);
}

// --- Max channels constant ---

void test_max_channels(void) {
    TEST_ASSERT_EQUAL_INT(8, RELAY_MAX_CHANNELS);
}

int main(int argc, char **argv) {
    UNITY_BEGIN();
    RUN_TEST(test_valid_channel_zero);
    RUN_TEST(test_valid_channel_last);
    RUN_TEST(test_invalid_channel_negative);
    RUN_TEST(test_invalid_channel_equal_to_count);
    RUN_TEST(test_invalid_channel_exceeds_count);
    RUN_TEST(test_invalid_channel_zero_count);
    RUN_TEST(test_invalid_channel_exceeds_max);
    RUN_TEST(test_gpio_level_on_active_high);
    RUN_TEST(test_gpio_level_off_active_high);
    RUN_TEST(test_gpio_level_on_active_low);
    RUN_TEST(test_gpio_level_off_active_low);
    RUN_TEST(test_relay_state_off_is_zero);
    RUN_TEST(test_relay_state_on_is_one);
    RUN_TEST(test_max_channels);
    UNITY_END();
    return 0;
}
