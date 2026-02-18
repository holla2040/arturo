#include "device_registry.h"
#include <cstring>

namespace arturo {

// Phase 2: Hardcoded device registry
// In future phases, this will read from YAML profiles
static const DeviceInfo DEVICES[] = {
    {"DMM-01", "192.168.1.100", 5025, "scpi"},  // Fluke 8846A
};

static const int NUM_DEVICES = sizeof(DEVICES) / sizeof(DEVICES[0]);

const DeviceInfo* getDevice(const char* deviceId) {
    if (deviceId == nullptr) return nullptr;

    for (int i = 0; i < NUM_DEVICES; i++) {
        if (strcmp(DEVICES[i].deviceId, deviceId) == 0) {
            return &DEVICES[i];
        }
    }
    return nullptr;
}

const DeviceInfo* getDevices(int& count) {
    count = NUM_DEVICES;
    return DEVICES;
}

} // namespace arturo
