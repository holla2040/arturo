#pragma once
#include <WiFi.h>

namespace arturo {

class RedisClient {
public:
    RedisClient(const char* host, uint16_t port);

    bool connect(const char* username = nullptr, const char* password = nullptr);
    bool isConnected();
    void disconnect();

    bool set(const char* key, const char* value, int exSeconds);
    bool publish(const char* channel, const char* message);

    // Pub/Sub subscribe
    bool subscribe(const char* channel);

    // Read next Pub/Sub message. Returns payload length, 0 on timeout, -1 on error.
    int readMessage(char* buf, size_t bufLen, unsigned long timeoutMs);

    int reconnectCount();

private:
    const char* _host;
    uint16_t _port;
    WiFiClient _socket;
    int _reconnects = 0;
    bool _hasConnected = false;
    char _buf[256];

    bool sendCommand(const char** argv, int argc);
    bool readLine();
    bool readLineWithTimeout(unsigned long timeoutMs);
    bool expectOK();
    int64_t readInteger();
    int readBulkString(char* out, size_t outLen);
    int readBulkStringWithTimeout(char* out, size_t outLen, unsigned long timeoutMs);
    int readArrayLen();
    int readArrayLenWithTimeout(unsigned long timeoutMs);
    bool skipBulkString();
};

} // namespace arturo
