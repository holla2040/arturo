#pragma once

#include <cstdint>
#include <time.h>

#ifdef ARDUINO
#include <Arduino.h>
#endif

namespace arturo {

// Minimum plausible Unix epoch (Nov 2023).
// If time(nullptr) returns less than this, NTP hasn't synced yet.
static constexpr int64_t MIN_VALID_EPOCH = 1700000000;

// Returns wall-clock UTC epoch seconds if NTP has synced,
// otherwise returns millis()/1000 as a fallback.
inline int64_t getTimestamp() {
    time_t now = time(nullptr);
    if (now > MIN_VALID_EPOCH) return (int64_t)now;
#ifdef ARDUINO
    return (int64_t)(millis() / 1000);
#else
    return (int64_t)now;
#endif
}

// Returns true if NTP has synced (wall clock is valid).
inline bool hasValidTime() {
    return time(nullptr) > MIN_VALID_EPOCH;
}

} // namespace arturo
