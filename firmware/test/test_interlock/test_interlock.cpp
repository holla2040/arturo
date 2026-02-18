#include <unity.h>
#include "safety/interlock.h"

using namespace arturo;

void setUp(void) {}
void tearDown(void) {}

// Temperature threshold: warn at 5-85C, fault at 0-100C
static const InterlockThreshold TEMP_THRESHOLD = {5.0f, 85.0f, 0.0f, 100.0f};

// Current threshold: warn at 0.1-4.5A, fault at 0.0-5.0A
static const InterlockThreshold CURRENT_THRESHOLD = {0.1f, 4.5f, 0.0f, 5.0f};

// --- interlockEvaluate tests ---

void test_evaluate_ok(void) {
    TEST_ASSERT_EQUAL(InterlockStatus::OK, interlockEvaluate(25.0f, TEMP_THRESHOLD));
    TEST_ASSERT_EQUAL(InterlockStatus::OK, interlockEvaluate(50.0f, TEMP_THRESHOLD));
}

void test_evaluate_warning_low(void) {
    // Between faultLow (0) and warningLow (5) — warning
    TEST_ASSERT_EQUAL(InterlockStatus::WARNING, interlockEvaluate(3.0f, TEMP_THRESHOLD));
}

void test_evaluate_warning_high(void) {
    // Between warningHigh (85) and faultHigh (100) — warning
    TEST_ASSERT_EQUAL(InterlockStatus::WARNING, interlockEvaluate(90.0f, TEMP_THRESHOLD));
}

void test_evaluate_fault_low(void) {
    // At or below faultLow (0)
    TEST_ASSERT_EQUAL(InterlockStatus::FAULT, interlockEvaluate(0.0f, TEMP_THRESHOLD));
    TEST_ASSERT_EQUAL(InterlockStatus::FAULT, interlockEvaluate(-5.0f, TEMP_THRESHOLD));
}

void test_evaluate_fault_high(void) {
    // At or above faultHigh (100)
    TEST_ASSERT_EQUAL(InterlockStatus::FAULT, interlockEvaluate(100.0f, TEMP_THRESHOLD));
    TEST_ASSERT_EQUAL(InterlockStatus::FAULT, interlockEvaluate(120.0f, TEMP_THRESHOLD));
}

void test_evaluate_at_warning_boundary(void) {
    // Exactly at warningLow — warning
    TEST_ASSERT_EQUAL(InterlockStatus::WARNING, interlockEvaluate(5.0f, TEMP_THRESHOLD));
    // Exactly at warningHigh — warning
    TEST_ASSERT_EQUAL(InterlockStatus::WARNING, interlockEvaluate(85.0f, TEMP_THRESHOLD));
}

void test_evaluate_just_inside_ok(void) {
    // Just above warningLow
    TEST_ASSERT_EQUAL(InterlockStatus::OK, interlockEvaluate(5.1f, TEMP_THRESHOLD));
    // Just below warningHigh
    TEST_ASSERT_EQUAL(InterlockStatus::OK, interlockEvaluate(84.9f, TEMP_THRESHOLD));
}

void test_evaluate_current_ok(void) {
    TEST_ASSERT_EQUAL(InterlockStatus::OK, interlockEvaluate(2.5f, CURRENT_THRESHOLD));
}

void test_evaluate_current_fault(void) {
    TEST_ASSERT_EQUAL(InterlockStatus::FAULT, interlockEvaluate(5.5f, CURRENT_THRESHOLD));
}

// --- interlockIsActionable tests ---

void test_actionable_ok(void) {
    TEST_ASSERT_FALSE(interlockIsActionable(InterlockStatus::OK));
}

void test_actionable_warning(void) {
    TEST_ASSERT_TRUE(interlockIsActionable(InterlockStatus::WARNING));
}

void test_actionable_fault(void) {
    TEST_ASSERT_TRUE(interlockIsActionable(InterlockStatus::FAULT));
}

// --- Enum values ---

void test_status_enum_ordering(void) {
    // OK < WARNING < FAULT
    TEST_ASSERT_LESS_THAN_UINT8((uint8_t)InterlockStatus::WARNING,
                                 (uint8_t)InterlockStatus::OK);
    TEST_ASSERT_LESS_THAN_UINT8((uint8_t)InterlockStatus::FAULT,
                                 (uint8_t)InterlockStatus::WARNING);
}

void test_max_checks_constant(void) {
    TEST_ASSERT_EQUAL_INT(8, INTERLOCK_MAX_CHECKS);
}

int main(int argc, char **argv) {
    UNITY_BEGIN();
    // Evaluate
    RUN_TEST(test_evaluate_ok);
    RUN_TEST(test_evaluate_warning_low);
    RUN_TEST(test_evaluate_warning_high);
    RUN_TEST(test_evaluate_fault_low);
    RUN_TEST(test_evaluate_fault_high);
    RUN_TEST(test_evaluate_at_warning_boundary);
    RUN_TEST(test_evaluate_just_inside_ok);
    RUN_TEST(test_evaluate_current_ok);
    RUN_TEST(test_evaluate_current_fault);
    // Actionable
    RUN_TEST(test_actionable_ok);
    RUN_TEST(test_actionable_warning);
    RUN_TEST(test_actionable_fault);
    // Enums
    RUN_TEST(test_status_enum_ordering);
    RUN_TEST(test_max_checks_constant);
    UNITY_END();
    return 0;
}
