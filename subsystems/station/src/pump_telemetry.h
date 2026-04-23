#pragma once

#include <cstddef>
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
    char regenChar = 'A';        // CTI 'O' response: A=off, B=warmup, H=purge, etc.
    uint16_t operatingHours = 0;
    uint8_t status1 = 0;         // S1 bitmask
    uint8_t status2 = 0;         // S2 bitmask
    uint8_t status3 = 0;         // S3 bitmask
    uint8_t staleCount = 10;     // 0=fresh, >=CACHE_STALE_THRESHOLD=stale
    uint32_t lastUpdateMs = 0;
};

// Consecutive failed polls at which the cache is considered untrustworthy.
// Cache-served commands return "pump_cache_stale" at or above this count.
// See docs/architecture/ARCHITECTURE.md §4.6.
static constexpr uint8_t CACHE_STALE_THRESHOLD = 3;

// True if commandName is served from the firmware's PumpTelemetry cache
// instead of the CTI UART. Set must match docs/SCRIPTING_HAL.md.
bool isPumpCacheServedCommand(const char* commandName);

// Serialize a snapshot to a stringified JSON object per docs/SCRIPTING_HAL.md
// "Telemetry Snapshot". Returns false on serialization error or truncation.
bool serializePumpTelemetryJson(const PumpTelemetry& snapshot, char* buf, size_t bufLen);

// Format a single cache-served command's response into buf, matching the
// on-wire format of the equivalent CTI command so controller-side parsers
// don't change. Returns false if commandName isn't cache-served.
bool formatCachedPumpCommand(const char* commandName,
                             const PumpTelemetry& snapshot,
                             char* buf, size_t bufLen);

} // namespace arturo
