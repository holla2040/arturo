#include "redis_client.h"
#include "../debug_log.h"
#include <cstring>
#include <cstdlib>

namespace arturo {

static const unsigned long RESP_TIMEOUT_MS = 2000;

RedisClient::RedisClient(const char* host, uint16_t port)
    : _host(host), _port(port) {
}

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

bool RedisClient::subscribe(const char* channel) {
    const char* argv[] = { "SUBSCRIBE", channel };
    if (!sendCommand(argv, 2)) {
        return false;
    }

    // Read confirmation: *3\r\n $9\r\n subscribe\r\n $<len>\r\n <channel>\r\n :1\r\n
    int arrLen = readArrayLen();
    if (arrLen != 3) {
        LOG_ERROR("REDIS", "SUBSCRIBE: expected 3-element array, got %d", arrLen);
        return false;
    }

    // Read "subscribe" string
    char type[16];
    if (readBulkString(type, sizeof(type)) < 0) {
        LOG_ERROR("REDIS", "SUBSCRIBE: failed to read type");
        return false;
    }

    // Read channel name
    char ch[64];
    if (readBulkString(ch, sizeof(ch)) < 0) {
        LOG_ERROR("REDIS", "SUBSCRIBE: failed to read channel");
        return false;
    }

    // Read subscription count (integer)
    int64_t count = readInteger();
    if (count < 0) {
        LOG_ERROR("REDIS", "SUBSCRIBE: failed to read count");
        return false;
    }

    LOG_INFO("REDIS", "SUBSCRIBE %s (subscriptions: %lld)", ch, (long long)count);
    return true;
}

int RedisClient::readMessage(char* buf, size_t bufLen, unsigned long timeoutMs) {
    // Wait for data on socket with caller-specified timeout
    unsigned long start = millis();
    while (!_socket.available()) {
        if (millis() - start >= timeoutMs) {
            return 0; // timeout, no message
        }
        if (!_socket.connected()) {
            return -1; // disconnected
        }
        delay(1);
    }

    // Remaining time for parsing the RESP message
    unsigned long remaining = timeoutMs - (millis() - start);
    if (remaining < RESP_TIMEOUT_MS) remaining = RESP_TIMEOUT_MS;

    // Parse *3\r\n $7\r\n message\r\n $<len>\r\n <channel>\r\n $<len>\r\n <payload>\r\n
    int arrLen = readArrayLenWithTimeout(remaining);
    if (arrLen != 3) {
        LOG_ERROR("REDIS", "readMessage: expected 3-element array, got %d", arrLen);
        return -1;
    }

    // Read "message" type string
    char type[16];
    if (readBulkStringWithTimeout(type, sizeof(type), remaining) < 0) {
        LOG_ERROR("REDIS", "readMessage: failed to read type");
        return -1;
    }

    // Read channel name (discard)
    char channel[64];
    if (readBulkStringWithTimeout(channel, sizeof(channel), remaining) < 0) {
        LOG_ERROR("REDIS", "readMessage: failed to read channel");
        return -1;
    }

    // Read payload
    int payloadLen = readBulkStringWithTimeout(buf, bufLen, remaining);
    if (payloadLen < 0) {
        LOG_ERROR("REDIS", "readMessage: failed to read payload");
        return -1;
    }

    LOG_DEBUG("REDIS", "Message from %s (%d bytes)", channel, payloadLen);
    return payloadLen;
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
    return readLineWithTimeout(RESP_TIMEOUT_MS);
}

bool RedisClient::readLineWithTimeout(unsigned long timeoutMs) {
    unsigned long start = millis();
    size_t pos = 0;

    while (millis() - start < timeoutMs) {
        if (_socket.available()) {
            char c = _socket.read();
            if (c == '\r') {
                // Expect \n next
                unsigned long crStart = millis();
                while (!_socket.available()) {
                    if (millis() - crStart >= timeoutMs) {
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

int RedisClient::readArrayLen() {
    return readArrayLenWithTimeout(RESP_TIMEOUT_MS);
}

int RedisClient::readArrayLenWithTimeout(unsigned long timeoutMs) {
    if (!readLineWithTimeout(timeoutMs)) {
        return -1;
    }
    if (_buf[0] == '*') {
        int len = (int)strtol(_buf + 1, nullptr, 10);
        if (len < 0) return -1; // nil array
        return len;
    }
    if (_buf[0] == '$' && _buf[1] == '-') {
        return -1; // nil bulk string treated as nil
    }
    if (_buf[0] == '-') {
        LOG_ERROR("REDIS", "Error response: %s", _buf + 1);
    }
    return -1;
}

int RedisClient::readBulkString(char* out, size_t outLen) {
    return readBulkStringWithTimeout(out, outLen, RESP_TIMEOUT_MS);
}

int RedisClient::readBulkStringWithTimeout(char* out, size_t outLen, unsigned long timeoutMs) {
    if (!readLineWithTimeout(timeoutMs)) {
        return -1;
    }
    if (_buf[0] == '$') {
        int len = (int)strtol(_buf + 1, nullptr, 10);
        if (len < 0) {
            // nil bulk string
            if (out && outLen > 0) out[0] = '\0';
            return -1;
        }
        // Read exactly len bytes + \r\n
        size_t toRead = (size_t)len;
        size_t pos = 0;
        unsigned long start = millis();
        while (pos < toRead && millis() - start < timeoutMs) {
            if (_socket.available()) {
                char c = _socket.read();
                if (pos < outLen - 1) {
                    out[pos] = c;
                }
                pos++;
            } else {
                delay(1);
            }
        }
        if (pos < outLen) {
            out[pos] = '\0';
        } else if (outLen > 0) {
            out[outLen - 1] = '\0';
        }
        // Consume trailing \r\n
        unsigned long crStart = millis();
        int trailing = 0;
        while (trailing < 2 && millis() - crStart < timeoutMs) {
            if (_socket.available()) {
                _socket.read();
                trailing++;
            } else {
                delay(1);
            }
        }
        return len;
    }
    if (_buf[0] == '-') {
        LOG_ERROR("REDIS", "Error response: %s", _buf + 1);
    }
    return -1;
}

bool RedisClient::skipBulkString() {
    char tmp[1];
    return readBulkString(tmp, 0) >= 0;
}

} // namespace arturo
