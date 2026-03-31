#pragma once

#include <cstdint>

namespace arturo {

enum class OperationalMode : uint8_t {
    IDLE    = 0,    // Not testing — full manual controls available
    TESTING = 1     // Test in progress — controls locked, only abort/pause/continue
};

enum class TestAction : uint8_t {
    ABORT    = 0,
    PAUSE    = 1,
    CONTINUE = 2
};

struct TestState {
    OperationalMode mode = OperationalMode::IDLE;
    char testId[32] = {};
    char testName[64] = {};
    bool paused = false;
    uint32_t elapsedSecs = 0;
    uint32_t lastUpdateMs = 0;
};

} // namespace arturo
