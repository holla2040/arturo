#include "redis_client.h"
#include "../debug_log.h"
#include <cstring>
#include <cstdlib>

namespace arturo {

static const unsigned long RESP_TIMEOUT_MS = 2000;

RedisClient::RedisClient(const char* host, uint16_t port)
    : _host(host), _port(port) {
    _lastEntryId[0] = '\0';
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

int RedisClient::readArrayLen() {
    if (!readLine()) {
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
    if (!readLine()) {
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
        while (pos < toRead && millis() - start < RESP_TIMEOUT_MS) {
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
        while (trailing < 2 && millis() - crStart < RESP_TIMEOUT_MS) {
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

bool RedisClient::xadd(const char* stream, const char* field, const char* value,
                        char* entryId, size_t entryIdLen) {
    const char* argv[] = { "XADD", stream, "*", field, value };
    if (!sendCommand(argv, 5)) {
        return false;
    }
    int len = readBulkString(entryId, entryIdLen);
    if (len < 0) {
        LOG_ERROR("REDIS", "XADD failed: no entry ID returned");
        return false;
    }
    LOG_DEBUG("REDIS", "XADD to %s -> %s", stream, entryId);
    return true;
}

int RedisClient::xreadBlock(const char* stream, const char* lastId,
                             unsigned long blockMs,
                             char* field, size_t fieldLen,
                             char* value, size_t valueLen) {
    char blockStr[12];
    snprintf(blockStr, sizeof(blockStr), "%lu", blockMs);

    const char* argv[] = { "XREAD", "COUNT", "1", "BLOCK", blockStr, "STREAMS", stream, lastId };
    if (!sendCommand(argv, 8)) {
        return -1;
    }

    // Response: *1 -> [*2 -> [streamName, *1 -> [*2 -> [entryID, *N -> [f,v,...]]]]]
    // Or nil (*-1 or $-1) on timeout
    // Poll for data availability before calling readLine, since BLOCK may take longer
    // than the normal RESP_TIMEOUT_MS.
    unsigned long waitStart = millis();
    unsigned long totalWait = blockMs + 2000; // block time + 2s buffer
    while (!_socket.available() && millis() - waitStart < totalWait) {
        delay(10);
    }
    if (!_socket.available()) {
        LOG_DEBUG("REDIS", "XREAD timeout waiting for response");
        return 0;
    }

    int streamsCount = readArrayLen();
    if (streamsCount < 0) {
        // nil = timeout, no messages
        return 0;
    }
    if (streamsCount == 0) {
        return 0;
    }

    // *2 [streamName, entries]
    int streamTuple = readArrayLen();
    if (streamTuple < 2) {
        LOG_ERROR("REDIS", "XREAD: expected stream tuple of 2, got %d", streamTuple);
        return -1;
    }

    // Read and discard stream name (bulk string)
    char streamName[64];
    if (readBulkString(streamName, sizeof(streamName)) < 0) {
        return -1;
    }

    // entries array: *1 (one entry since COUNT 1)
    int entriesCount = readArrayLen();
    if (entriesCount < 1) {
        return 0;
    }

    // Each entry: *2 [entryID, fieldValues]
    int entryTuple = readArrayLen();
    if (entryTuple < 2) {
        LOG_ERROR("REDIS", "XREAD: expected entry tuple of 2, got %d", entryTuple);
        return -1;
    }

    // Read entry ID (stored in _lastEntryId after parsing completes)
    char entryIdBuf[32];
    if (readBulkString(entryIdBuf, sizeof(entryIdBuf)) < 0) {
        return -1;
    }

    // field-value array: *N (pairs)
    int fvCount = readArrayLen();
    if (fvCount < 2) {
        LOG_ERROR("REDIS", "XREAD: expected at least 2 field-values, got %d", fvCount);
        return -1;
    }

    // Read first field-value pair
    if (readBulkString(field, fieldLen) < 0) {
        return -1;
    }
    if (readBulkString(value, valueLen) < 0) {
        return -1;
    }

    // Skip remaining field-value pairs
    for (int i = 2; i < fvCount; i++) {
        char skip[1];
        readBulkString(skip, 0);
    }

    // Skip remaining entries (shouldn't be any with COUNT 1)
    for (int i = 1; i < entriesCount; i++) {
        // skip each entry: entryID + field-values
        int t = readArrayLen();
        for (int j = 0; j < t; j++) {
            char skip[1];
            readBulkString(skip, 0);
        }
    }

    // Store entry ID so caller can update lastId via lastEntryId()
    strncpy(_lastEntryId, entryIdBuf, sizeof(_lastEntryId) - 1);
    _lastEntryId[sizeof(_lastEntryId) - 1] = '\0';

    LOG_DEBUG("REDIS", "XREAD from %s entry=%s field=%s", stream, entryIdBuf, field);
    return 1;
}

} // namespace arturo
