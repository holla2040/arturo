#include "estop.h"

#ifdef ARDUINO
#include <Arduino.h>
#include "../debug_log.h"
#endif

namespace arturo {

const EStopConfig ESTOP_DEFAULT_CONFIG = {
    -1,     // buttonPin (must be configured per station)
    true,   // activeLow (typical: button grounds the pin, internal pull-up)
    -1,     // ledPin (optional)
    50      // debounceMs
};

bool estopDebounce(bool currentReading, bool lastReading,
                   unsigned long lastChangeMs, unsigned long nowMs,
                   unsigned long debounceMs) {
    // If reading hasn't changed, no transition to debounce
    if (currentReading == lastReading) return true;

    // Reading changed — check if enough time has passed
    unsigned long elapsed = nowMs - lastChangeMs;
    return elapsed >= debounceMs;
}

#ifdef ARDUINO
EStopHandler::EStopHandler()
    : _config{-1, true, -1, 50}, _state(EStopState::CLEAR),
      _lastReading(false), _lastChangeMs(0),
      _tripCount(0), _lastTripMs(0), _initialized(false) {}

bool EStopHandler::init(const EStopConfig& config) {
    if (config.buttonPin < 0) {
        LOG_ERROR("ESTOP", "No button pin configured");
        return false;
    }

    _config = config;

    // Configure button pin with pull-up if active-low
    if (_config.activeLow) {
        pinMode(_config.buttonPin, INPUT_PULLUP);
    } else {
        pinMode(_config.buttonPin, INPUT);
    }

    // Configure LED pin if present
    if (_config.ledPin >= 0) {
        pinMode(_config.ledPin, OUTPUT);
        digitalWrite(_config.ledPin, LOW);  // LED off initially
    }

    // Read initial state
    bool reading = digitalRead(_config.buttonPin);
    _lastReading = reading;
    _lastChangeMs = millis();

    // Check if button is already pressed on boot
    bool pressed = _config.activeLow ? !reading : reading;
    if (pressed) {
        activateEstop();
        LOG_ERROR("ESTOP", "Button pressed on boot — starting in TRIPPED state");
    }

    _initialized = true;
    LOG_INFO("ESTOP", "Initialized: pin=%d, activeLow=%d, ledPin=%d",
             _config.buttonPin, _config.activeLow, _config.ledPin);
    return true;
}

bool EStopHandler::poll() {
    if (!_initialized) return false;

    bool reading = digitalRead(_config.buttonPin);
    unsigned long now = millis();

    // Track state changes for debounce
    if (reading != _lastReading) {
        _lastChangeMs = now;
    }
    _lastReading = reading;

    // Check debounce
    if (!estopDebounce(reading, _lastReading, _lastChangeMs, now, _config.debounceMs)) {
        return false;
    }

    // Determine if button is pressed
    bool pressed = _config.activeLow ? !reading : reading;

    // Check for state transitions
    if (pressed && _state == EStopState::CLEAR) {
        activateEstop();
        return true;
    }

    return false;
}

void EStopHandler::trip() {
    if (_state == EStopState::TRIPPED) return;
    LOG_ERROR("ESTOP", "Remote E-stop received");
    activateEstop();
}

void EStopHandler::clear() {
    if (_state == EStopState::CLEAR) return;
    LOG_INFO("ESTOP", "E-stop cleared by operator");
    deactivateEstop();
}

void EStopHandler::activateEstop() {
    _state = EStopState::TRIPPED;
    _tripCount++;
    _lastTripMs = millis();

    // LED on
    if (_config.ledPin >= 0) {
        digitalWrite(_config.ledPin, HIGH);
    }

    LOG_ERROR("ESTOP", "TRIPPED (count=%d)", _tripCount);
}

void EStopHandler::deactivateEstop() {
    _state = EStopState::CLEAR;

    // LED off
    if (_config.ledPin >= 0) {
        digitalWrite(_config.ledPin, LOW);
    }

    LOG_INFO("ESTOP", "CLEAR");
}
#endif

} // namespace arturo
