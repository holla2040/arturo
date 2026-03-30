#include "relay_controller.h"

#ifdef ARDUINO
#include <Arduino.h>
#include "../debug_log.h"
#endif

namespace arturo {

bool isValidChannel(int channel, int numChannels) {
    return channel >= 0 && channel < numChannels && numChannels <= RELAY_MAX_CHANNELS;
}

int relayStateToGpioLevel(RelayState state, bool activeHigh) {
    if (activeHigh) {
        return (state == RelayState::ON) ? 1 : 0;
    } else {
        // Active-low: ON = LOW, OFF = HIGH
        return (state == RelayState::ON) ? 0 : 1;
    }
}

#ifdef ARDUINO
bool RelayController::init(const RelayChannel* channels, int numChannels) {
    if (channels == nullptr || numChannels <= 0 || numChannels > RELAY_MAX_CHANNELS) {
        LOG_ERROR("RELAY", "Invalid channel config: count=%d", numChannels);
        return false;
    }

    _numChannels = numChannels;
    _initialized = true;

    for (int i = 0; i < numChannels; i++) {
        _channels[i] = channels[i];
        _states[i] = RelayState::OFF;

        // Configure GPIO as output and set safe state (OFF)
        pinMode(_channels[i].gpioPin, OUTPUT);
        int level = relayStateToGpioLevel(RelayState::OFF, _channels[i].activeHigh);
        digitalWrite(_channels[i].gpioPin, level);

        LOG_INFO("RELAY", "Channel %d: GPIO %d, activeHigh=%d -> OFF",
                 i, _channels[i].gpioPin, _channels[i].activeHigh);
    }

    LOG_INFO("RELAY", "Initialized %d channels (all OFF)", numChannels);
    return true;
}

bool RelayController::setChannel(int channel, RelayState state) {
    if (!_initialized || !isValidChannel(channel, _numChannels)) {
        LOG_ERROR("RELAY", "Invalid channel: %d", channel);
        return false;
    }

    _states[channel] = state;
    int level = relayStateToGpioLevel(state, _channels[channel].activeHigh);
    digitalWrite(_channels[channel].gpioPin, level);

    LOG_DEBUG("RELAY", "Channel %d (GPIO %d) -> %s",
              channel, _channels[channel].gpioPin,
              state == RelayState::ON ? "ON" : "OFF");
    return true;
}

RelayState RelayController::getChannel(int channel) const {
    if (!_initialized || !isValidChannel(channel, _numChannels)) {
        return RelayState::OFF;
    }
    return _states[channel];
}

void RelayController::setAll(RelayState state) {
    for (int i = 0; i < _numChannels; i++) {
        setChannel(i, state);
    }
}

void RelayController::allOff() {
    LOG_INFO("RELAY", "ALL OFF (emergency/safe state)");
    for (int i = 0; i < _numChannels; i++) {
        _states[i] = RelayState::OFF;
        int level = relayStateToGpioLevel(RelayState::OFF, _channels[i].activeHigh);
        digitalWrite(_channels[i].gpioPin, level);
    }
}
#endif

} // namespace arturo
