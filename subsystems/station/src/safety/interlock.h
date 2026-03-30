#pragma once
#include <cstdint>
#include <cstddef>

namespace arturo {

static const int INTERLOCK_MAX_CHECKS = 8;

// Interlock check result
enum class InterlockStatus : uint8_t {
    OK      = 0,  // Within limits
    WARNING = 1,  // Approaching limit
    FAULT   = 2   // Limit exceeded, action required
};

// Type of interlock check
enum class InterlockType : uint8_t {
    TEMPERATURE,
    CURRENT,
    VOLTAGE,
    PRESSURE,
    CUSTOM
};

// Interlock threshold configuration — testable without hardware
struct InterlockThreshold {
    float warningLow;
    float warningHigh;
    float faultLow;
    float faultHigh;
};

// Evaluate a value against thresholds
// Returns OK if within warning range, WARNING if outside warning but inside fault,
// FAULT if outside fault range
InterlockStatus interlockEvaluate(float value, const InterlockThreshold& threshold);

// Check if a status is actionable (WARNING or FAULT)
bool interlockIsActionable(InterlockStatus status);

// Interlock check definition
struct InterlockCheck {
    const char* name;
    InterlockType type;
    InterlockThreshold threshold;
};

// Result of a single interlock evaluation
struct InterlockResult {
    const char* name;
    InterlockType type;
    InterlockStatus status;
    float value;
};

#ifdef ARDUINO

class InterlockManager {
public:
    InterlockManager();

    // Register an interlock check. Returns index, or -1 if full.
    int addCheck(const char* name, InterlockType type,
                 const InterlockThreshold& threshold);

    // Evaluate a single check by index with a new reading
    InterlockResult evaluate(int index, float value);

    // Check if any interlock is in FAULT state
    bool hasFault() const;

    // Check if any interlock is in WARNING or FAULT state
    bool hasWarning() const;

    // Get the worst status across all checks
    InterlockStatus worstStatus() const;

    // Get results for all checks
    int getResults(InterlockResult* results, int maxResults) const;

    int numChecks() const { return _numChecks; }

private:
    InterlockCheck _checks[INTERLOCK_MAX_CHECKS];
    InterlockResult _results[INTERLOCK_MAX_CHECKS];
    int _numChecks;
};

#endif

} // namespace arturo
