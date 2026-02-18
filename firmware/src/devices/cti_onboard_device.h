#pragma once
#include <cstddef>
#include "../protocols/cti.h"

#ifdef ARDUINO
#include "serial_device.h"
#endif

namespace arturo {

// CTI OnBoard command name → CTI protocol command mapping
struct CtiOnBoardCommandMapping {
    const char* commandName;   // from Redis request (e.g., "pump_status")
    const char* ctiCommand;    // CTI protocol command (e.g., "A?")
};

// Look up a CTI command string by command name
// Returns the CTI command string, or nullptr if not found
const char* ctiOnBoardLookupCommand(const char* commandName);

#ifdef ARDUINO
class CtiOnBoardDevice {
public:
    CtiOnBoardDevice();

    // Initialize with a serial device (must already be begin()'d)
    bool init(SerialDevice& serial);

    bool isInitialized() const { return _initialized; }

    // Execute a CTI command and store response data in responseBuf.
    // ctiCmd is the raw CTI command string (e.g., "A?").
    // Returns true if a valid response was received.
    bool executeCommand(const char* ctiCmd, char* responseBuf, size_t responseBufLen);

    // Diagnostics
    int transactionCount() const { return _transactions; }
    int errorCount() const { return _errors; }
    const CtiResponse& lastResponse() const { return _lastResp; }

private:
    SerialDevice* _serial;
    CtiResponse _lastResp;
    int _transactions;
    int _errors;
    bool _initialized;
};
#endif

} // namespace arturo
