#include "redis_client.h"
#include "../debug_log.h"
#include <cstring>
#include <cstdlib>

namespace arturo {

static const unsigned long RESP_TIMEOUT_MS = 2000;

RedisClient::RedisClient(const char* host, uint16_t port)
    : _host(host), _port(port) {}

bool RedisClient::connect(const char* username, const char* password) {
    LOG_INFO("REDIS", "Connecting to %s:%u", _host, _port);

    if (!_socket.connect(_host, _port)) {
        LOG_ERROR("REDIS", "TCP connection failed");
        return false;
    }

    LOG_INFO("REDIS", "TCP connected");

    // AUTH if credentials provided
    if (username != nullptr && username[0] != '\0') {
        LOG_DEBUG("REDIS", "Authenticating as %s", username);
        const char* argv[] = { "AUTH", username, password };
        if (!sendCommand(argv, 3) || !expectOK()) {
            LOG_ERROR("REDIS", "AUTH failed");
            _socket.stop();
            return false;
        }
        LOG_INFO("REDIS", "Authenticated");
    }

    if (_hasConnected) {
        _reconnects++;
        LOG_INFO("REDIS", "Reconnected (count: %d)", _reconnects);
    }
    _hasConnected = true;

    return true;
}

bool RedisClient::isConnected() {
    return _socket.connected();
}

void RedisClient::disconnect() {
    _socket.stop();
    LOG_INFO("REDIS", "Disconnected");
}

bool RedisClient::set(const char* key, const char* value, int exSeconds) {
    char exStr[12];
    snprintf(exStr, sizeof(exStr), "%d", exSeconds);

    const char* argv[] = { "SET", key, value, "EX", exStr };
    if (!sendCommand(argv, 5)) {
        return false;
    }
    return expectOK();
}

bool RedisClient::publish(const char* channel, const char* message) {
    const char* argv[] = { "PUBLISH", channel, message };
    if (!sendCommand(argv, 3)) {
        return false;
    }
    int64_t subscribers = readInteger();
    if (subscribers < 0) {
        return false;
    }
    LOG_DEBUG("REDIS", "PUBLISH to %s, %lld subscribers", channel, (long long)subscribers);
    return true;
}

int RedisClient::reconnectCount() {
    return _reconnects;
}

bool RedisClient::sendCommand(const char** argv, int argc) {
    if (!_socket.connected()) {
        LOG_ERROR("REDIS", "Not connected");
        return false;
    }

    // Write RESP array header: *N\r\n
    char header[16];
    snprintf(header, sizeof(header), "*%d\r\n", argc);
    _socket.print(header);

    // Write each bulk string: $len\r\n<data>\r\n
    for (int i = 0; i < argc; i++) {
        size_t len = strlen(argv[i]);
        char bulkHeader[16];
        snprintf(bulkHeader, sizeof(bulkHeader), "$%u\r\n", (unsigned)len);
        _socket.print(bulkHeader);
        _socket.write((const uint8_t*)argv[i], len);
        _socket.print("\r\n");
    }

    _socket.flush();
    return true;
}

bool RedisClient::readLine() {
    unsigned long start = millis();
    size_t pos = 0;

    while (millis() - start < RESP_TIMEOUT_MS) {
        if (_socket.available()) {
            char c = _socket.read();
            if (c == '\r') {
                // Expect \n next
                unsigned long crStart = millis();
                while (!_socket.available()) {
                    if (millis() - crStart >= RESP_TIMEOUT_MS) {
                        LOG_ERROR("REDIS", "Timeout waiting for \\n after \\r");
                        _buf[pos] = '\0';
                        return false;
                    }
                }
                _socket.read(); // consume \n
                _buf[pos] = '\0';
                return true;
            }
            if (pos < sizeof(_buf) - 1) {
                _buf[pos++] = c;
            }
        } else {
            delay(1);
        }
    }

    _buf[pos] = '\0';
    LOG_ERROR("REDIS", "readLine timeout");
    return false;
}

bool RedisClient::expectOK() {
    if (!readLine()) {
        return false;
    }
    if (_buf[0] == '+') {
        return true;
    }
    if (_buf[0] == '-') {
        LOG_ERROR("REDIS", "Error response: %s", _buf + 1);
    }
    return false;
}

int64_t RedisClient::readInteger() {
    if (!readLine()) {
        return -1;
    }
    if (_buf[0] == ':') {
        return strtoll(_buf + 1, nullptr, 10);
    }
    if (_buf[0] == '-') {
        LOG_ERROR("REDIS", "Error response: %s", _buf + 1);
    }
    return -1;
}

} // namespace arturo
