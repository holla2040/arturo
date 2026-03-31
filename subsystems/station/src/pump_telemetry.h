#pragma once

#include <cstdint>

#ifdef ARDUINO
#include <freertos/FreeRTOS.h>
#include <freertos/semphr.h>
#endif

namespace arturo {

struct PumpTelemetry {
    float stage1TempK = 0;
    float stage2TempK = 0;
    float pressureTorr = 0;
    bool pumpOn = false;
    bool roughValveOpen = false;
    bool purgeValveOpen = false;
    uint8_t regenStep = 0;       // 0=off, 1-5=phase
    uint16_t operatingHours = 0;
    uint8_t status1 = 0;         // S1 bitmask
    uint8_t status2 = 0;         // S2 bitmask
    uint8_t status3 = 0;         // S3 bitmask
    uint8_t staleCount = 10;     // 0=fresh, >2=offline
    uint32_t lastUpdateMs = 0;
};

} // namespace arturo
