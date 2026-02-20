#include "cti_onboard_device.h"
#include <cstring>

#ifdef ARDUINO
#include "../debug_log.h"
#endif

namespace arturo {

// Command name → CTI command mapping table
// Extracted from profiles/pumps/cti_onboard.yaml
static const CtiOnBoardCommandMapping CTI_ONBOARD_COMMANDS[] = {
    {"pump_status",          "A?"},
    {"pump_on",              "A1"},
    {"pump_off",             "A0"},
    {"get_temp_1st_stage",   "J"},
    {"get_temp_2nd_stage",   "K"},
    {"get_pump_tc_pressure", "L"},
    {"get_aux_tc_pressure",  "M"},
    {"get_status_1",         "S1"},
    {"get_status_2",         "S2"},
    {"get_status_3",         "S3"},
    {"get_rough_valve",      "D?"},
    {"open_rough_valve",     "D1"},
    {"close_rough_valve",    "D0"},
    {"get_purge_valve",      "E?"},
    {"open_purge_valve",     "E1"},
    {"close_purge_valve",    "E0"},
    {"start_regen",          "N1"},
    {"start_fast_regen",     "N2"},
    {"abort_regen",          "N0"},
    {"get_regen_step",       "O"},
    {"get_regen_status",     "O"},
};

static const int CTI_ONBOARD_COMMAND_COUNT = sizeof(CTI_ONBOARD_COMMANDS) / sizeof(CTI_ONBOARD_COMMANDS[0]);

const char* ctiOnBoardLookupCommand(const char* commandName) {
    if (commandName == nullptr) return nullptr;

    for (int i = 0; i < CTI_ONBOARD_COMMAND_COUNT; i++) {
        if (strcmp(CTI_ONBOARD_COMMANDS[i].commandName, commandName) == 0) {
            return CTI_ONBOARD_COMMANDS[i].ctiCommand;
        }
    }
    return nullptr;
}

#ifdef ARDUINO
CtiOnBoardDevice::CtiOnBoardDevice()
    : _serial(nullptr), _transactions(0), _errors(0), _initialized(false) {
    memset(&_lastResp, 0, sizeof(_lastResp));
}

bool CtiOnBoardDevice::init(SerialDevice& serial) {
    if (!serial.isReady()) {
        LOG_ERROR("CTI", "Serial device not ready");
        return false;
    }

    _serial = &serial;
    _initialized = true;

    LOG_INFO("CTI", "CtiOnBoardDevice initialized");
    return true;
}

bool CtiOnBoardDevice::executeCommand(const char* ctiCmd, char* responseBuf, size_t responseBufLen) {
    if (!_initialized || _serial == nullptr) {
        LOG_ERROR("CTI", "Not initialized");
        return false;
    }
    if (ctiCmd == nullptr || responseBuf == nullptr || responseBufLen == 0) {
        return false;
    }

    // Build the CTI frame: $<cmd><checksum>\r
    char frame[64];
    int frameLen = ctiBuildFrame(ctiCmd, frame, sizeof(frame));
    if (frameLen < 0) {
        LOG_ERROR("CTI", "Failed to build frame for '%s'", ctiCmd);
        _errors++;
        return false;
    }

    // Drain stale data
    _serial->drain();

    // Send frame
    int sent = _serial->sendString(frame);
    if (sent < 0 || sent != frameLen) {
        LOG_ERROR("CTI", "TX failed: sent %d/%d bytes", sent, frameLen);
        _errors++;
        return false;
    }
    _serial->flush();

    LOG_INFO("CTI", "TX: %s (%d bytes)", ctiCmd, frameLen);

    // Receive response line (terminated by \r)
    char rxBuf[128];
    int rxLen = _serial->receiveLine(rxBuf, sizeof(rxBuf), '\r', CTI_TIMEOUT_MS);
    if (rxLen < 0) {
        LOG_ERROR("CTI", "RX timeout for '%s'", ctiCmd);
        _errors++;
        return false;
    }

    // receiveLine strips \r, but ctiParseFrame expects it — re-append
    if ((size_t)rxLen < sizeof(rxBuf) - 1) {
        rxBuf[rxLen] = '\r';
        rxLen++;
        rxBuf[rxLen] = '\0';
    }

    LOG_DEBUG("CTI", "RX: %d bytes", rxLen);

    // Parse the CTI response frame
    if (!ctiParseFrame(rxBuf, (size_t)rxLen, _lastResp)) {
        LOG_ERROR("CTI", "Failed to parse response frame");
        _errors++;
        return false;
    }

    _transactions++;

    if (!_lastResp.checksumValid) {
        LOG_ERROR("CTI", "Checksum mismatch in response");
        _errors++;
        return false;
    }

    if (!ctiIsSuccess(_lastResp.code)) {
        LOG_ERROR("CTI", "Device error: %s code=%c", ctiCmd, (char)_lastResp.code);
        // Still copy the response code info for the caller
        snprintf(responseBuf, responseBufLen, "ERR:%c", (char)_lastResp.code);
        return false;
    }

    // Copy response data
    size_t copyLen = _lastResp.dataLen;
    if (copyLen >= responseBufLen) {
        copyLen = responseBufLen - 1;
    }
    memcpy(responseBuf, _lastResp.data, copyLen);
    responseBuf[copyLen] = '\0';

    LOG_INFO("CTI", "OK: %s -> '%s'", ctiCmd, responseBuf);
    return true;
}
#endif

} // namespace arturo
