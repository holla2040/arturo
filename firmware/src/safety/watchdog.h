#pragma once
#include <cstdint>

namespace arturo {

// Watchdog configuration — testable without hardware
static const unsigned long WATCHDOG_TIMEOUT_S  = 8;
static const unsigned long WATCHDOG_TIMEOUT_MS = WATCHDOG_TIMEOUT_S * 1000;

// Feed interval: half the timeout gives a safe margin
static const unsigned long WATCHDOG_FEED_INTERVAL_MS = WATCHDOG_TIMEOUT_MS / 2;

// Late feed warning threshold: 75% of timeout means we're cutting it close
static const unsigned long WATCHDOG_LATE_THRESHOLD_MS = (WATCHDOG_TIMEOUT_MS * 3) / 4;

// Check if watchdog feed is overdue (for testing feed interval logic)
// Returns true if the watchdog should be fed based on elapsed time
bool watchdogFeedDue(unsigned long lastFeedMs, unsigned long nowMs,
                     unsigned long intervalMs);

// Check if a feed is dangerously late (elapsed time exceeds the late threshold)
// Returns true if the elapsed time since last feed is >= lateThresholdMs
bool watchdogIsLateFeed(unsigned long lastFeedMs, unsigned long nowMs,
                        unsigned long lateThresholdMs);

// Calculate elapsed time handling millis() overflow correctly
unsigned long watchdogElapsed(unsigned long startMs, unsigned long nowMs);

#ifdef ARDUINO

class Watchdog {
public:
    Watchdog();

    // Initialize hardware watchdog with timeout
    bool init(unsigned long timeoutMs = WATCHDOG_TIMEOUT_MS);

    // Feed the watchdog — must be called before timeout expires
    void feed();

    // Get diagnostics
    unsigned long lastFeedMs() const { return _lastFeedMs; }
    int resetCount() const { return _resetCount; }
    bool isInitialized() const { return _initialized; }

private:
    unsigned long _lastFeedMs;
    int _resetCount;
    bool _initialized;
};

#endif

} // namespace arturo
