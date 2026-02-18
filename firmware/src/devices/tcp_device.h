#pragma once
#include <cstddef>
#include <cstdint>

#ifdef ARDUINO
#include <WiFi.h>
#endif

namespace arturo {

// Reconnect backoff calculation — testable without hardware
// Returns delay in ms for the given attempt (0-indexed)
// Exponential backoff: 1s, 2s, 4s, 8s, 16s, capped at maxDelayMs
unsigned long reconnectBackoffMs(int attempt, unsigned long maxDelayMs = 30000);

#ifdef ARDUINO
class TcpDevice {
public:
    TcpDevice();

    // Connect to host:port with timeout
    bool connect(const char* host, uint16_t port, unsigned long timeoutMs = 5000);
    bool isConnected();
    void disconnect();

    // Reconnect using last host/port with exponential backoff
    // Returns true if reconnection succeeded
    bool reconnect();

    // Send raw bytes. Returns number of bytes written, or -1 on error.
    int send(const uint8_t* data, size_t len);

    // Send null-terminated string. Returns number of bytes written, or -1 on error.
    int sendString(const char* str);

    // Receive bytes until buffer full or timeout.
    // Returns number of bytes read, or -1 on error/timeout with no data.
    int receive(uint8_t* buf, size_t bufLen, unsigned long timeoutMs = 5000);

    // Receive a line (terminated by terminator char, default '\n').
    // Strips the terminator. Returns length of line, or -1 on timeout.
    int receiveLine(char* buf, size_t bufLen, char terminator = '\n',
                    unsigned long timeoutMs = 5000);

    // Flush any pending output
    void flush();

    // Accessors
    const char* host() const { return _host; }
    uint16_t port() const { return _port; }
    int reconnectCount() const { return _reconnects; }

private:
    WiFiClient _socket;
    const char* _host;
    uint16_t _port;
    int _reconnects;
    int _reconnectAttempt;
    unsigned long _lastReconnectMs;
};
#endif

} // namespace arturo
