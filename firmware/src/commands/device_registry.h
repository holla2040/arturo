#pragma once
#include <cstdint>

namespace arturo {

struct DeviceInfo {
    const char* deviceId;
    const char* host;
    uint16_t port;
    const char* protocolType;  // "scpi", "modbus", etc.
};

// Get device info by device ID
// Returns pointer to static DeviceInfo or nullptr if not found
const DeviceInfo* getDevice(const char* deviceId);

// Get all registered devices
// Returns pointer to static array, sets count
const DeviceInfo* getDevices(int& count);

} // namespace arturo
