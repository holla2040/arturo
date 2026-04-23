#include "pump_telemetry.h"

#include <ArduinoJson.h>
#include <cstdio>
#include <cstring>

namespace arturo {

namespace {

// Must match docs/SCRIPTING_HAL.md Cache-served commands for cti_onboard.
// Keep alphabetized for grep-ability.
const char* const kCachedCommands[] = {
    "get_purge_valve",
    "get_pump_tc_pressure",
    "get_regen_status",
    "get_regen_step",
    "get_rough_valve",
    "get_status_1",
    "get_telemetry",
    "get_temp_1st_stage",
    "get_temp_2nd_stage",
};

} // namespace

bool isPumpCacheServedCommand(const char* commandName) {
    if (commandName == nullptr) return false;
    for (const char* c : kCachedCommands) {
        if (strcmp(commandName, c) == 0) return true;
    }
    return false;
}

bool serializePumpTelemetryJson(const PumpTelemetry& s, char* buf, size_t bufLen) {
    if (buf == nullptr || bufLen == 0) return false;

    JsonDocument doc;
    doc["stage1_temp_k"] = s.stage1TempK;
    doc["stage2_temp_k"] = s.stage2TempK;
    doc["pressure_torr"] = s.pressureTorr;
    doc["pump_on"] = s.pumpOn;
    doc["rough_valve_open"] = s.roughValveOpen;
    doc["purge_valve_open"] = s.purgeValveOpen;

    char regenStr[2] = { s.regenChar, '\0' };
    doc["regen_char"] = regenStr;

    doc["operating_hours"] = s.operatingHours;
    doc["status_1"] = s.status1;
    doc["stale_count"] = s.staleCount;
    doc["last_update_ms"] = s.lastUpdateMs;

    size_t written = serializeJson(doc, buf, bufLen);
    return written > 0 && written < bufLen;
}

bool formatCachedPumpCommand(const char* commandName,
                             const PumpTelemetry& s,
                             char* buf, size_t bufLen) {
    if (commandName == nullptr || buf == nullptr || bufLen == 0) return false;

    if (strcmp(commandName, "get_telemetry") == 0) {
        return serializePumpTelemetryJson(s, buf, bufLen);
    }

    if (strcmp(commandName, "get_temp_1st_stage") == 0) {
        int n = snprintf(buf, bufLen, "%.1f", s.stage1TempK);
        return n > 0 && (size_t)n < bufLen;
    }
    if (strcmp(commandName, "get_temp_2nd_stage") == 0) {
        int n = snprintf(buf, bufLen, "%.1f", s.stage2TempK);
        return n > 0 && (size_t)n < bufLen;
    }
    if (strcmp(commandName, "get_pump_tc_pressure") == 0) {
        // CTI L returns pressure in scientific notation; match that here.
        int n = snprintf(buf, bufLen, "%.3e", s.pressureTorr);
        return n > 0 && (size_t)n < bufLen;
    }
    if (strcmp(commandName, "get_status_1") == 0) {
        // CTI S1 returns one raw byte; the poller at poller.go:94-105 reads
        // (*s1)[0] as an int. Preserve that shape exactly.
        if (bufLen < 2) return false;
        buf[0] = (char)s.status1;
        buf[1] = '\0';
        return true;
    }
    if (strcmp(commandName, "get_rough_valve") == 0) {
        if (bufLen < 2) return false;
        buf[0] = s.roughValveOpen ? '1' : '0';
        buf[1] = '\0';
        return true;
    }
    if (strcmp(commandName, "get_purge_valve") == 0) {
        if (bufLen < 2) return false;
        buf[0] = s.purgeValveOpen ? '1' : '0';
        buf[1] = '\0';
        return true;
    }
    if (strcmp(commandName, "get_regen_status") == 0 ||
        strcmp(commandName, "get_regen_step") == 0) {
        if (bufLen < 2) return false;
        buf[0] = s.regenChar;
        buf[1] = '\0';
        return true;
    }

    return false;
}

} // namespace arturo
