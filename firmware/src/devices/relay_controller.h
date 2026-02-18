#pragma once
#include <cstdint>
#include <cstddef>

namespace arturo {

static const int RELAY_MAX_CHANNELS = 8;

// Relay channel state
enum class RelayState : uint8_t {
    OFF = 0,
    ON  = 1
};

// Relay channel configuration — testable without hardware
struct RelayChannel {
    int gpioPin;
    bool activeHigh;  // true = HIGH=ON, false = LOW=ON (active-low relay)
};

// Validate channel number against max channels
bool isValidChannel(int channel, int numChannels);

// Map relay state + activeHigh to GPIO level
// Returns 0 (LOW) or 1 (HIGH)
int relayStateToGpioLevel(RelayState state, bool activeHigh);

#ifdef ARDUINO

class RelayController {
public:
    // Initialize with channel configuration
    // All relays are set to OFF (safe state) on init
    bool init(const RelayChannel* channels, int numChannels);

    // Set a single channel. Returns false if channel invalid.
    bool setChannel(int channel, RelayState state);

    // Get current state of a channel. Returns OFF if channel invalid.
    RelayState getChannel(int channel) const;

    // Convenience
    bool turnOn(int channel)  { return setChannel(channel, RelayState::ON); }
    bool turnOff(int channel) { return setChannel(channel, RelayState::OFF); }

    // Set all channels to the same state
    void setAll(RelayState state);

    // Emergency: all relays OFF immediately
    void allOff();

    // Number of configured channels
    int numChannels() const { return _numChannels; }

private:
    RelayChannel _channels[RELAY_MAX_CHANNELS];
    RelayState _states[RELAY_MAX_CHANNELS];
    int _numChannels;
    bool _initialized;
};

#endif

} // namespace arturo
