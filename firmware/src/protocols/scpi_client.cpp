#include "scpi_client.h"
#include <cstring>
#include <cstdlib>

#ifdef ARDUINO
#include "../debug_log.h"
#endif

namespace arturo {

int formatScpiCommand(const char* cmd, char* output, size_t outputLen, const char* lineEnding) {
    size_t cmdLen = strlen(cmd);
    size_t endLen = strlen(lineEnding);
    size_t totalLen = cmdLen + endLen;

    if (totalLen >= outputLen) return -1;

    memcpy(output, cmd, cmdLen);
    memcpy(output + cmdLen, lineEnding, endLen);
    output[totalLen] = '\0';
    return (int)totalLen;
}

bool parseScpiResponse(const char* raw, size_t rawLen, char* output, size_t outputLen, bool& isError) {
    if (raw == nullptr || rawLen == 0) return false;

    // Strip trailing \r\n or \n
    size_t len = rawLen;
    while (len > 0 && (raw[len - 1] == '\n' || raw[len - 1] == '\r')) {
        len--;
    }

    if (len == 0) return false;
    if (len >= outputLen) return false;

    memcpy(output, raw, len);
    output[len] = '\0';

    // Check for SCPI error patterns
    // Negative number followed by comma is typically an error code
    // e.g., "-100,\"Command error\""
    isError = false;
    if (raw[0] == '-' && len > 1) {
        for (size_t i = 1; i < len; i++) {
            if (raw[i] == ',') {
                isError = true;
                break;
            }
            if (raw[i] < '0' || raw[i] > '9') break;
        }
    }

    return true;
}

bool parseScpiError(const char* response, int& errorCode, char* errorMsg, size_t errorMsgLen) {
    if (response == nullptr) return false;

    // Format: -NNN,"message" or +NNN,"message"
    const char* comma = strchr(response, ',');
    if (comma == nullptr) return false;

    // Parse error code
    char codeStr[16];
    size_t codeLen = comma - response;
    if (codeLen >= sizeof(codeStr)) return false;
    memcpy(codeStr, response, codeLen);
    codeStr[codeLen] = '\0';
    errorCode = atoi(codeStr);

    // Parse message — skip comma and optional quote
    const char* msg = comma + 1;
    while (*msg == ' ' || *msg == '"') msg++;

    size_t msgLen = strlen(msg);
    // Strip trailing quote
    while (msgLen > 0 && msg[msgLen - 1] == '"') msgLen--;

    if (msgLen >= errorMsgLen) msgLen = errorMsgLen - 1;
    memcpy(errorMsg, msg, msgLen);
    errorMsg[msgLen] = '\0';

    return true;
}

#ifdef ARDUINO
ScpiClient::ScpiClient() : _host(nullptr), _port(0) {}

bool ScpiClient::connect(const char* host, uint16_t port) {
    _host = host;
    _port = port;
    LOG_INFO("SCPI", "Connecting to %s:%u", host, port);

    if (!_socket.connect(host, port)) {
        LOG_ERROR("SCPI", "TCP connection failed to %s:%u", host, port);
        return false;
    }

    LOG_INFO("SCPI", "Connected to %s:%u", host, port);
    return true;
}

bool ScpiClient::isConnected() {
    return _socket.connected();
}

void ScpiClient::disconnect() {
    _socket.stop();
    LOG_INFO("SCPI", "Disconnected");
}

bool ScpiClient::sendCommand(const char* cmd, char* responseBuf, size_t responseBufLen,
                              unsigned long timeoutMs) {
    if (!_socket.connected()) {
        LOG_ERROR("SCPI", "Not connected");
        return false;
    }

    // Format command with line ending
    char formatted[512];
    int len = formatScpiCommand(cmd, formatted, sizeof(formatted));
    if (len < 0) {
        LOG_ERROR("SCPI", "Command too long: %s", cmd);
        return false;
    }

    LOG_DEBUG("SCPI", "Sending: %s", cmd);
    _socket.write((const uint8_t*)formatted, len);
    _socket.flush();

    // Read response with timeout
    unsigned long start = millis();
    size_t pos = 0;

    while (millis() - start < timeoutMs) {
        if (_socket.available()) {
            char c = _socket.read();
            if (c == '\n') {
                responseBuf[pos] = '\0';
                // Strip trailing \r if present
                if (pos > 0 && responseBuf[pos - 1] == '\r') {
                    responseBuf[pos - 1] = '\0';
                }
                LOG_DEBUG("SCPI", "Response: %s", responseBuf);
                return true;
            }
            if (pos < responseBufLen - 1) {
                responseBuf[pos++] = c;
            }
        } else {
            delay(1);
        }
    }

    responseBuf[pos] = '\0';
    LOG_ERROR("SCPI", "Timeout waiting for response to: %s", cmd);
    return false;
}
#endif

} // namespace arturo
