#pragma once

#include <cstdint>

namespace arturo {

static const int LOCAL_CMD_QUEUE_SIZE = 4;

struct LocalCommand {
    char commandName[32];   // Abstract name: "pump_on", "open_rough_valve", etc.
    char deviceId[16];      // Device ID or empty string for default device
};

} // namespace arturo
