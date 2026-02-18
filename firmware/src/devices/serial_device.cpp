#include "serial_device.h"
#include <cstring>
#include <cstdlib>

#ifdef ARDUINO
#include "../debug_log.h"
#endif

namespace arturo {

// Default configs for common protocols
const SerialConfig SERIAL_CONFIG_CTI    = {2400,  7, 'E', 1};  // CTI/Brooks: 2400 baud, 7E1
const SerialConfig SERIAL_CONFIG_MODBUS = {9600,  8, 'N', 1};  // Modbus RTU: 9600 baud, 8N1
const SerialConfig SERIAL_CONFIG_ASCII  = {115200, 8, 'N', 1}; // ASCII: 115200 baud, 8N1

bool parseSerialConfig(const char* configStr, SerialConfig& config) {
    if (configStr == nullptr) return false;

    // Format: "BAUD-DNP" where D=databits, N=parity, P=stopbits
    // Examples: "2400-7E1", "9600-8N1", "115200-8N1"
    const char* dash = strchr(configStr, '-');
    if (dash == nullptr) return false;

    // Parse baud rate
    char baudStr[16];
    size_t baudLen = dash - configStr;
    if (baudLen == 0 || baudLen >= sizeof(baudStr)) return false;
    memcpy(baudStr, configStr, baudLen);
    baudStr[baudLen] = '\0';
    config.baudRate = (uint32_t)atol(baudStr);
    if (config.baudRate == 0) return false;

    // Parse data bits, parity, stop bits after dash
    const char* mode = dash + 1;
    if (strlen(mode) != 3) return false;

    // Data bits: 5-8
    config.dataBits = mode[0] - '0';
    if (config.dataBits < 5 || config.dataBits > 8) return false;

    // Parity: N, E, O
    config.parity = mode[1];
    if (config.parity != 'N' && config.parity != 'E' && config.parity != 'O') return false;

    // Stop bits: 1-2
    config.stopBits = mode[2] - '0';
    if (config.stopBits < 1 || config.stopBits > 2) return false;

    return true;
}

uint32_t serialConfigToMode(const SerialConfig& config) {
    // Pack into a uint32_t that matches Arduino SERIAL_* constants on ESP32
    // ESP32 Arduino defines: SERIAL_8N1 = 0x800001c, etc.
    // We build the mode value from components.
    //
    // For native testing, we pack as: (dataBits << 16) | (parity << 8) | stopBits
    // On Arduino, we map to the actual constants.

#ifdef ARDUINO
    // Map to ESP32 Arduino serial mode constants
    uint32_t mode = 0;

    // The ESP32 Arduino uses uart_word_length_t, uart_parity_t, uart_stop_bits_t
    // packed into a single constant. We use the SERIAL_* defines.
    switch (config.dataBits) {
        case 5:
            switch (config.parity) {
                case 'N': mode = (config.stopBits == 1) ? SERIAL_5N1 : SERIAL_5N2; break;
                case 'E': mode = (config.stopBits == 1) ? SERIAL_5E1 : SERIAL_5E2; break;
                case 'O': mode = (config.stopBits == 1) ? SERIAL_5O1 : SERIAL_5O2; break;
            }
            break;
        case 6:
            switch (config.parity) {
                case 'N': mode = (config.stopBits == 1) ? SERIAL_6N1 : SERIAL_6N2; break;
                case 'E': mode = (config.stopBits == 1) ? SERIAL_6E1 : SERIAL_6E2; break;
                case 'O': mode = (config.stopBits == 1) ? SERIAL_6O1 : SERIAL_6O2; break;
            }
            break;
        case 7:
            switch (config.parity) {
                case 'N': mode = (config.stopBits == 1) ? SERIAL_7N1 : SERIAL_7N2; break;
                case 'E': mode = (config.stopBits == 1) ? SERIAL_7E1 : SERIAL_7E2; break;
                case 'O': mode = (config.stopBits == 1) ? SERIAL_7O1 : SERIAL_7O2; break;
            }
            break;
        case 8:
        default:
            switch (config.parity) {
                case 'N': mode = (config.stopBits == 1) ? SERIAL_8N1 : SERIAL_8N2; break;
                case 'E': mode = (config.stopBits == 1) ? SERIAL_8E1 : SERIAL_8E2; break;
                case 'O': mode = (config.stopBits == 1) ? SERIAL_8O1 : SERIAL_8O2; break;
            }
            break;
    }
    return mode;
#else
    // Native: pack for testing
    return ((uint32_t)config.dataBits << 16) | ((uint32_t)config.parity << 8) | config.stopBits;
#endif
}

#ifdef ARDUINO
SerialDevice::SerialDevice(int uartNum)
    : _serial(uartNum), _config{0, 0, 0, 0}, _ready(false) {}

bool SerialDevice::begin(const SerialConfig& config, int rxPin, int txPin) {
    _config = config;
    uint32_t mode = serialConfigToMode(config);

    LOG_INFO("SERIAL", "Opening UART: %lu baud, %d%c%d",
             (unsigned long)config.baudRate, config.dataBits,
             config.parity, config.stopBits);

    if (rxPin >= 0 && txPin >= 0) {
        _serial.begin(config.baudRate, mode, rxPin, txPin);
    } else {
        _serial.begin(config.baudRate, mode);
    }

    _serial.setTimeout(100);
    _ready = true;

    LOG_INFO("SERIAL", "UART ready");
    return true;
}

void SerialDevice::end() {
    _serial.end();
    _ready = false;
    LOG_INFO("SERIAL", "UART closed");
}

int SerialDevice::send(const uint8_t* data, size_t len) {
    if (!_ready) return -1;
    size_t written = _serial.write(data, len);
    LOG_TRACE("SERIAL", "TX %zu bytes", written);
    return (int)written;
}

int SerialDevice::sendString(const char* str) {
    return send((const uint8_t*)str, strlen(str));
}

int SerialDevice::receive(uint8_t* buf, size_t bufLen, unsigned long timeoutMs) {
    if (!_ready) return -1;

    unsigned long start = millis();
    size_t pos = 0;

    while (millis() - start < timeoutMs && pos < bufLen) {
        if (_serial.available()) {
            int c = _serial.read();
            if (c >= 0) {
                buf[pos++] = (uint8_t)c;
            }
        } else {
            delay(1);
        }
    }

    if (pos == 0) {
        LOG_DEBUG("SERIAL", "Receive timeout (%lums)", timeoutMs);
        return -1;
    }

    LOG_TRACE("SERIAL", "RX %zu bytes", pos);
    return (int)pos;
}

int SerialDevice::receiveLine(char* buf, size_t bufLen, char terminator,
                              unsigned long timeoutMs) {
    if (!_ready) return -1;

    unsigned long start = millis();
    size_t pos = 0;

    while (millis() - start < timeoutMs) {
        if (_serial.available()) {
            char c = _serial.read();
            if (c == terminator) {
                buf[pos] = '\0';
                if (pos > 0 && buf[pos - 1] == '\r') {
                    buf[pos - 1] = '\0';
                    pos--;
                }
                LOG_TRACE("SERIAL", "RX line: %s", buf);
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
    LOG_DEBUG("SERIAL", "ReceiveLine timeout (%lums)", timeoutMs);
    return -1;
}

int SerialDevice::receiveExact(uint8_t* buf, size_t expected, unsigned long timeoutMs) {
    if (!_ready) return -1;

    unsigned long start = millis();
    size_t pos = 0;

    while (millis() - start < timeoutMs && pos < expected) {
        if (_serial.available()) {
            int c = _serial.read();
            if (c >= 0) {
                buf[pos++] = (uint8_t)c;
            }
        } else {
            delay(1);
        }
    }

    if (pos == 0) return -1;

    LOG_TRACE("SERIAL", "RX exact %zu/%zu bytes", pos, expected);
    return (int)pos;
}

void SerialDevice::flush() {
    if (_ready) _serial.flush();
}

void SerialDevice::drain() {
    if (!_ready) return;
    while (_serial.available()) {
        _serial.read();
    }
}
#endif

} // namespace arturo
