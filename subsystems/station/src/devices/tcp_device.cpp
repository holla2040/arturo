#include "tcp_device.h"
#include <cstring>

#ifdef ARDUINO
#include "../debug_log.h"
#endif

namespace arturo {

unsigned long reconnectBackoffMs(int attempt, unsigned long maxDelayMs) {
    if (attempt < 0) return 0;

    // 1s * 2^attempt, capped at maxDelayMs
    unsigned long delayMs = 1000;
    for (int i = 0; i < attempt && delayMs < maxDelayMs; i++) {
        delayMs *= 2;
    }
    if (delayMs > maxDelayMs) delayMs = maxDelayMs;
    return delayMs;
}

#ifdef ARDUINO
TcpDevice::TcpDevice()
    : _host(nullptr), _port(0), _reconnects(0),
      _reconnectAttempt(0), _lastReconnectMs(0) {}

bool TcpDevice::connect(const char* host, uint16_t port, unsigned long timeoutMs) {
    _host = host;
    _port = port;
    _reconnectAttempt = 0;

    LOG_INFO("TCP", "Connecting to %s:%u", host, port);

    _socket.setTimeout(timeoutMs);
    if (!_socket.connect(host, port)) {
        LOG_ERROR("TCP", "Connection failed to %s:%u", host, port);
        return false;
    }

    LOG_INFO("TCP", "Connected to %s:%u", host, port);
    return true;
}

bool TcpDevice::isConnected() {
    return _socket.connected();
}

void TcpDevice::disconnect() {
    _socket.stop();
    LOG_INFO("TCP", "Disconnected from %s:%u", _host ? _host : "?", _port);
}

bool TcpDevice::reconnect() {
    if (_host == nullptr) {
        LOG_ERROR("TCP", "Cannot reconnect: no previous connection");
        return false;
    }

    unsigned long backoff = reconnectBackoffMs(_reconnectAttempt);
    unsigned long now = millis();

    // Enforce backoff delay between attempts
    if (_lastReconnectMs > 0 && (now - _lastReconnectMs) < backoff) {
        return false;
    }

    _lastReconnectMs = now;
    LOG_INFO("TCP", "Reconnecting to %s:%u (attempt %d, backoff %lums)",
             _host, _port, _reconnectAttempt + 1, backoff);

    _socket.stop();
    _socket.setTimeout(5000);
    if (!_socket.connect(_host, _port)) {
        _reconnectAttempt++;
        LOG_ERROR("TCP", "Reconnect failed to %s:%u", _host, _port);
        return false;
    }

    _reconnects++;
    _reconnectAttempt = 0;
    LOG_INFO("TCP", "Reconnected to %s:%u (total reconnects: %d)",
             _host, _port, _reconnects);
    return true;
}

int TcpDevice::send(const uint8_t* data, size_t len) {
    if (!_socket.connected()) {
        LOG_ERROR("TCP", "Send failed: not connected");
        return -1;
    }

    size_t written = _socket.write(data, len);
    LOG_TRACE("TCP", "TX %zu bytes", written);
    return (int)written;
}

int TcpDevice::sendString(const char* str) {
    return send((const uint8_t*)str, strlen(str));
}

int TcpDevice::receive(uint8_t* buf, size_t bufLen, unsigned long timeoutMs) {
    if (!_socket.connected()) {
        LOG_ERROR("TCP", "Receive failed: not connected");
        return -1;
    }

    unsigned long start = millis();
    size_t pos = 0;

    while (millis() - start < timeoutMs && pos < bufLen) {
        if (_socket.available()) {
            int n = _socket.read(buf + pos, bufLen - pos);
            if (n > 0) {
                pos += n;
                // Got some data — return what we have
                break;
            }
        } else {
            delay(1);
        }
    }

    if (pos == 0) {
        LOG_DEBUG("TCP", "Receive timeout (%lums)", timeoutMs);
        return -1;
    }

    LOG_TRACE("TCP", "RX %zu bytes", pos);
    return (int)pos;
}

int TcpDevice::receiveLine(char* buf, size_t bufLen, char terminator,
                           unsigned long timeoutMs) {
    if (!_socket.connected()) {
        LOG_ERROR("TCP", "ReceiveLine failed: not connected");
        return -1;
    }

    unsigned long start = millis();
    size_t pos = 0;

    while (millis() - start < timeoutMs) {
        if (_socket.available()) {
            char c = _socket.read();
            if (c == terminator) {
                buf[pos] = '\0';
                // Strip trailing \r if present
                if (pos > 0 && buf[pos - 1] == '\r') {
                    buf[pos - 1] = '\0';
                    pos--;
                }
                LOG_TRACE("TCP", "RX line: %s", buf);
                return (int)pos;
            }
            if (pos < bufLen - 1) {
                buf[pos++] = c;
            }
        } else {
            delay(1);
        }
    }

    buf[pos] = '\0';
    LOG_DEBUG("TCP", "ReceiveLine timeout (%lums), partial: %zu bytes", timeoutMs, pos);
    return -1;
}

void TcpDevice::flush() {
    _socket.flush();
}
#endif

} // namespace arturo
