#pragma once
#include <cstddef>
#include "../protocols/cti.h"

#ifdef ARDUINO
#include "cti_worker.h"
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
// Thin wrapper around CtiWorker. Keeps a command-name lookup table for
// external callers that ask by abstract name; all I/O is delegated to the
// worker, which owns the UART. Synchronous executeCommand() blocks on the
// worker's reply queue.
class CtiOnBoardDevice {
public:
    CtiOnBoardDevice();

    // Attach to a running worker. begin() must have been called on the worker.
    bool init(CtiWorker& worker);

    bool isInitialized() const { return _initialized; }

    // Execute a CTI command via the worker. ctiCmd is the raw protocol
    // string (e.g., "A?"). Returns true on success.
    bool executeCommand(const char* ctiCmd, char* responseBuf, size_t responseBufLen);

    // Diagnostics (forwarded from worker).
    int transactionCount() const;
    int errorCount() const;

private:
    CtiWorker* _worker;
    bool       _initialized;
};
#endif

} // namespace arturo
