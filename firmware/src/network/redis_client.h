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
    bool expectOK();
    int64_t readInteger();
};

} // namespace arturo
