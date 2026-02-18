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

    // Redis Streams
    bool xadd(const char* stream, const char* field, const char* value,
              char* entryId, size_t entryIdLen);
    int xreadBlock(const char* stream, const char* lastId, unsigned long blockMs,
                   char* field, size_t fieldLen, char* value, size_t valueLen);

    // After a successful xreadBlock, returns the entry ID of the last read message.
    // Caller should use this to update lastId for subsequent xreadBlock calls.
    const char* lastEntryId() const { return _lastEntryId; }

    int reconnectCount();

private:
    const char* _host;
    uint16_t _port;
    WiFiClient _socket;
    int _reconnects = 0;
    bool _hasConnected = false;
    char _buf[256];
    char _lastEntryId[32];

    bool sendCommand(const char** argv, int argc);
    bool readLine();
    bool expectOK();
    int64_t readInteger();
    int readBulkString(char* out, size_t outLen);
    int readArrayLen();
    bool skipBulkString();
};

} // namespace arturo
