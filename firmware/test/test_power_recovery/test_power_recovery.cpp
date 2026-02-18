#include <unity.h>
#include "safety/power_recovery.h"
#include <cstring>

using namespace arturo;

void setUp(void) {}
void tearDown(void) {}

// --- isAbnormalBoot tests ---

void test_power_on_is_normal(void) {
    TEST_ASSERT_FALSE(isAbnormalBoot(BootReason::POWER_ON));
}

void test_software_is_normal(void) {
    TEST_ASSERT_FALSE(isAbnormalBoot(BootReason::SOFTWARE));
}

void test_deep_sleep_is_normal(void) {
    TEST_ASSERT_FALSE(isAbnormalBoot(BootReason::DEEP_SLEEP));
}

void test_unknown_is_normal(void) {
    TEST_ASSERT_FALSE(isAbnormalBoot(BootReason::UNKNOWN));
}

void test_watchdog_is_abnormal(void) {
    TEST_ASSERT_TRUE(isAbnormalBoot(BootReason::WATCHDOG));
}

void test_brownout_is_abnormal(void) {
    TEST_ASSERT_TRUE(isAbnormalBoot(BootReason::BROWNOUT));
}

void test_panic_is_abnormal(void) {
    TEST_ASSERT_TRUE(isAbnormalBoot(BootReason::PANIC));
}

// --- isPowerRelatedBoot tests ---

void test_brownout_is_power_related(void) {
    TEST_ASSERT_TRUE(isPowerRelatedBoot(BootReason::BROWNOUT));
}

void test_watchdog_not_power_related(void) {
    TEST_ASSERT_FALSE(isPowerRelatedBoot(BootReason::WATCHDOG));
}

void test_power_on_not_power_related(void) {
    TEST_ASSERT_FALSE(isPowerRelatedBoot(BootReason::POWER_ON));
}

// --- bootReasonToString tests ---

void test_reason_string_power_on(void) {
    TEST_ASSERT_EQUAL_STRING("POWER_ON", bootReasonToString(BootReason::POWER_ON));
}

void test_reason_string_software(void) {
    TEST_ASSERT_EQUAL_STRING("SOFTWARE", bootReasonToString(BootReason::SOFTWARE));
}

void test_reason_string_watchdog(void) {
    TEST_ASSERT_EQUAL_STRING("WATCHDOG", bootReasonToString(BootReason::WATCHDOG));
}

void test_reason_string_brownout(void) {
    TEST_ASSERT_EQUAL_STRING("BROWNOUT", bootReasonToString(BootReason::BROWNOUT));
}

void test_reason_string_panic(void) {
    TEST_ASSERT_EQUAL_STRING("PANIC", bootReasonToString(BootReason::PANIC));
}

void test_reason_string_deep_sleep(void) {
    TEST_ASSERT_EQUAL_STRING("DEEP_SLEEP", bootReasonToString(BootReason::DEEP_SLEEP));
}

void test_reason_string_unknown(void) {
    TEST_ASSERT_EQUAL_STRING("UNKNOWN", bootReasonToString(BootReason::UNKNOWN));
}

// --- SafeStateFlags tests ---

void test_all_safe_states_all_true(void) {
    SafeStateFlags flags = {true, true, true, true};
    TEST_ASSERT_TRUE(allSafeStatesVerified(flags));
}

void test_all_safe_states_one_false(void) {
    SafeStateFlags flags = {true, true, false, true};
    TEST_ASSERT_FALSE(allSafeStatesVerified(flags));
}

void test_all_safe_states_all_false(void) {
    SafeStateFlags flags = {false, false, false, false};
    TEST_ASSERT_FALSE(allSafeStatesVerified(flags));
}

void test_safe_state_count_all(void) {
    SafeStateFlags flags = {true, true, true, true};
    TEST_ASSERT_EQUAL_INT(4, safeStateCount(flags));
}

void test_safe_state_count_some(void) {
    SafeStateFlags flags = {true, false, true, false};
    TEST_ASSERT_EQUAL_INT(2, safeStateCount(flags));
}

void test_safe_state_count_none(void) {
    SafeStateFlags flags = {false, false, false, false};
    TEST_ASSERT_EQUAL_INT(0, safeStateCount(flags));
}

void test_safe_state_total(void) {
    TEST_ASSERT_EQUAL_INT(4, SAFE_STATE_TOTAL);
}

// --- RecoveryContext tests ---

void test_recovery_magic_constant(void) {
    TEST_ASSERT_EQUAL_UINT32(0x41525430, RECOVERY_MAGIC);
}

void test_valid_recovery_context(void) {
    RecoveryContext ctx;
    memset(&ctx, 0, sizeof(ctx));
    ctx.magic = RECOVERY_MAGIC;
    TEST_ASSERT_TRUE(isValidRecoveryContext(ctx));
}

void test_invalid_recovery_context_zero(void) {
    RecoveryContext ctx;
    memset(&ctx, 0, sizeof(ctx));
    TEST_ASSERT_FALSE(isValidRecoveryContext(ctx));
}

void test_invalid_recovery_context_bad_magic(void) {
    RecoveryContext ctx;
    memset(&ctx, 0, sizeof(ctx));
    ctx.magic = 0xDEADBEEF;
    TEST_ASSERT_FALSE(isValidRecoveryContext(ctx));
}

void test_init_recovery_context(void) {
    RecoveryContext ctx;
    memset(&ctx, 0xFF, sizeof(ctx));  // Fill with garbage first
    initRecoveryContext(ctx);

    TEST_ASSERT_EQUAL_UINT32(RECOVERY_MAGIC, ctx.magic);
    TEST_ASSERT_EQUAL_UINT32(0, ctx.bootCount);
    TEST_ASSERT_EQUAL_UINT32(0, ctx.abnormalBootCount);
    TEST_ASSERT_EQUAL_UINT8((uint8_t)BootReason::POWER_ON, (uint8_t)ctx.lastBootReason);
    TEST_ASSERT_EQUAL_UINT32(0, ctx.lastUptimeSeconds);
    TEST_ASSERT_FALSE(ctx.testWasRunning);
    TEST_ASSERT_EQUAL_CHAR('\0', ctx.lastTestId[0]);
}

// --- updateRecoveryContextOnBoot tests ---

void test_update_context_normal_boot(void) {
    RecoveryContext ctx;
    initRecoveryContext(ctx);

    updateRecoveryContextOnBoot(ctx, BootReason::POWER_ON);

    TEST_ASSERT_EQUAL_UINT32(1, ctx.bootCount);
    TEST_ASSERT_EQUAL_UINT32(0, ctx.abnormalBootCount);
    TEST_ASSERT_EQUAL_UINT8((uint8_t)BootReason::POWER_ON, (uint8_t)ctx.lastBootReason);
}

void test_update_context_watchdog_boot(void) {
    RecoveryContext ctx;
    initRecoveryContext(ctx);

    updateRecoveryContextOnBoot(ctx, BootReason::WATCHDOG);

    TEST_ASSERT_EQUAL_UINT32(1, ctx.bootCount);
    TEST_ASSERT_EQUAL_UINT32(1, ctx.abnormalBootCount);
    TEST_ASSERT_EQUAL_UINT8((uint8_t)BootReason::WATCHDOG, (uint8_t)ctx.lastBootReason);
}

void test_update_context_multiple_boots(void) {
    RecoveryContext ctx;
    initRecoveryContext(ctx);

    updateRecoveryContextOnBoot(ctx, BootReason::POWER_ON);
    updateRecoveryContextOnBoot(ctx, BootReason::BROWNOUT);
    updateRecoveryContextOnBoot(ctx, BootReason::SOFTWARE);
    updateRecoveryContextOnBoot(ctx, BootReason::PANIC);

    TEST_ASSERT_EQUAL_UINT32(4, ctx.bootCount);
    TEST_ASSERT_EQUAL_UINT32(2, ctx.abnormalBootCount);  // BROWNOUT + PANIC
    TEST_ASSERT_EQUAL_UINT8((uint8_t)BootReason::PANIC, (uint8_t)ctx.lastBootReason);
}

void test_update_context_preserves_test_state(void) {
    RecoveryContext ctx;
    initRecoveryContext(ctx);

    // Simulate a test was running when power was lost
    ctx.testWasRunning = true;
    strncpy(ctx.lastTestId, "test-calibration-01", sizeof(ctx.lastTestId) - 1);

    // After reboot
    updateRecoveryContextOnBoot(ctx, BootReason::BROWNOUT);

    // Test state should be preserved for the caller to check
    TEST_ASSERT_TRUE(ctx.testWasRunning);
    TEST_ASSERT_EQUAL_STRING("test-calibration-01", ctx.lastTestId);
}

// --- BootReason enum values ---

void test_boot_reason_enum_values(void) {
    TEST_ASSERT_EQUAL_UINT8(0, (uint8_t)BootReason::POWER_ON);
    TEST_ASSERT_EQUAL_UINT8(1, (uint8_t)BootReason::SOFTWARE);
    TEST_ASSERT_EQUAL_UINT8(2, (uint8_t)BootReason::WATCHDOG);
    TEST_ASSERT_EQUAL_UINT8(3, (uint8_t)BootReason::BROWNOUT);
    TEST_ASSERT_EQUAL_UINT8(4, (uint8_t)BootReason::PANIC);
    TEST_ASSERT_EQUAL_UINT8(5, (uint8_t)BootReason::DEEP_SLEEP);
    TEST_ASSERT_EQUAL_UINT8(6, (uint8_t)BootReason::UNKNOWN);
}

int main(int argc, char **argv) {
    UNITY_BEGIN();

    // isAbnormalBoot
    RUN_TEST(test_power_on_is_normal);
    RUN_TEST(test_software_is_normal);
    RUN_TEST(test_deep_sleep_is_normal);
    RUN_TEST(test_unknown_is_normal);
    RUN_TEST(test_watchdog_is_abnormal);
    RUN_TEST(test_brownout_is_abnormal);
    RUN_TEST(test_panic_is_abnormal);

    // isPowerRelatedBoot
    RUN_TEST(test_brownout_is_power_related);
    RUN_TEST(test_watchdog_not_power_related);
    RUN_TEST(test_power_on_not_power_related);

    // bootReasonToString
    RUN_TEST(test_reason_string_power_on);
    RUN_TEST(test_reason_string_software);
    RUN_TEST(test_reason_string_watchdog);
    RUN_TEST(test_reason_string_brownout);
    RUN_TEST(test_reason_string_panic);
    RUN_TEST(test_reason_string_deep_sleep);
    RUN_TEST(test_reason_string_unknown);

    // SafeStateFlags
    RUN_TEST(test_all_safe_states_all_true);
    RUN_TEST(test_all_safe_states_one_false);
    RUN_TEST(test_all_safe_states_all_false);
    RUN_TEST(test_safe_state_count_all);
    RUN_TEST(test_safe_state_count_some);
    RUN_TEST(test_safe_state_count_none);
    RUN_TEST(test_safe_state_total);

    // RecoveryContext
    RUN_TEST(test_recovery_magic_constant);
    RUN_TEST(test_valid_recovery_context);
    RUN_TEST(test_invalid_recovery_context_zero);
    RUN_TEST(test_invalid_recovery_context_bad_magic);
    RUN_TEST(test_init_recovery_context);

    // updateRecoveryContextOnBoot
    RUN_TEST(test_update_context_normal_boot);
    RUN_TEST(test_update_context_watchdog_boot);
    RUN_TEST(test_update_context_multiple_boots);
    RUN_TEST(test_update_context_preserves_test_state);

    // Enum values
    RUN_TEST(test_boot_reason_enum_values);

    UNITY_END();
    return 0;
}
