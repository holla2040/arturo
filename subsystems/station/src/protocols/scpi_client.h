#pragma once
#include <cstddef>

#ifdef ARDUINO
#include <WiFi.h>
#endif

namespace arturo {

// SCPI command formatting — testable without hardware
// Formats a SCPI command with line ending into output buffer
// Returns length of formatted command, or -1 if buffer too small
int formatScpiCommand(const char* cmd, char* output, size_t outputLen, const char* lineEnding = "\n");

// Parse a SCPI response — strips line ending, detects errors
// Returns true if response was parsed successfully
// Sets isError to true if response starts with error indicators
bool parseScpiResponse(const char* raw, size_t rawLen, char* output, size_t outputLen, bool& isError);

// Parse SCPI error response format: code,"message"
// e.g., "-100,\"Command error\""
bool parseScpiError(const char* response, int& errorCode, char* errorMsg, size_t errorMsgLen);

#ifdef ARDUINO
class ScpiClient {
public:
    ScpiClient();

    bool connect(const char* host, uint16_t port);
    bool isConnected();
    void disconnect();

    // Send a SCPI command and read response
    // Returns true on success, response stored in responseBuf
    // timeoutMs: max time to wait for response
    bool sendCommand(const char* cmd, char* responseBuf, size_t responseBufLen,
                     unsigned long timeoutMs = 5000);

private:
    WiFiClient _socket;
    const char* _host;
    uint16_t _port;
    static const unsigned long DEFAULT_TIMEOUT_MS = 5000;
};
#endif

} // namespace arturo
