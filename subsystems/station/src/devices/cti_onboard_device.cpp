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
    : _worker(nullptr), _initialized(false) {}

bool CtiOnBoardDevice::init(CtiWorker& worker) {
    if (!worker.isInitialized()) {
        LOG_ERROR("CTI", "CtiWorker not initialized");
        return false;
    }
    _worker = &worker;
    _initialized = true;
    LOG_INFO("CTI", "CtiOnBoardDevice attached to worker");
    return true;
}

bool CtiOnBoardDevice::executeCommand(const char* ctiCmd, char* responseBuf, size_t responseBufLen) {
    if (!_initialized || _worker == nullptr) {
        LOG_ERROR("CTI", "Not initialized");
        return false;
    }
    if (ctiCmd == nullptr || responseBuf == nullptr || responseBufLen == 0) {
        return false;
    }
    return _worker->executeCommand(ctiCmd, responseBuf, responseBufLen);
}

int CtiOnBoardDevice::transactionCount() const {
    return _worker ? _worker->transactionCount() : 0;
}

int CtiOnBoardDevice::errorCount() const {
    return _worker ? _worker->errorCount() : 0;
}
#endif

} // namespace arturo
