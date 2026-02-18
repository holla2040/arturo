#pragma once
#include <cstdint>

namespace arturo {

// E-stop button states
enum class EStopState : uint8_t {
    CLEAR   = 0,  // Normal operation
    TRIPPED = 1   // E-stop active, all relays off
};

// E-stop event types for logging/messaging
enum class EStopEvent : uint8_t {
    BUTTON_PRESSED,   // Physical button pressed
    REMOTE_RECEIVED,  // Received system.emergency_stop from Redis
    MANUAL_CLEAR      // Operator cleared the E-stop
};

// Debounce logic — testable without hardware
// Returns true if button state is stable (has been in same state for debounceMs)
bool estopDebounce(bool currentReading, bool lastReading,
                   unsigned long lastChangeMs, unsigned long nowMs,
                   unsigned long debounceMs = 50);

// E-stop configuration
struct EStopConfig {
    int buttonPin;       // GPIO pin for E-stop button
    bool activeLow;      // true = button grounds pin (pull-up), false = button pulls high
    int ledPin;          // GPIO pin for E-stop indicator LED (-1 = none)
    unsigned long debounceMs;
};

// Default E-stop config
extern const EStopConfig ESTOP_DEFAULT_CONFIG;

#ifdef ARDUINO

class EStopHandler {
public:
    EStopHandler();

    // Initialize GPIO and set initial state
    bool init(const EStopConfig& config);

    // Poll button state — call from main loop or watchdog task
    // Returns true if E-stop state changed
    bool poll();

    // Trip E-stop from software (e.g., remote command)
    void trip();

    // Clear E-stop (operator action)
    void clear();

    // Current state
    EStopState state() const { return _state; }
    bool isTripped() const { return _state == EStopState::TRIPPED; }

    // Event tracking
    int tripCount() const { return _tripCount; }
    unsigned long lastTripMs() const { return _lastTripMs; }

private:
    void activateEstop();
    void deactivateEstop();

    EStopConfig _config;
    EStopState _state;
    bool _lastReading;
    unsigned long _lastChangeMs;
    int _tripCount;
    unsigned long _lastTripMs;
    bool _initialized;
};

#endif

} // namespace arturo
