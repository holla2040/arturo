#pragma once
#include <cstddef>
#include <cstdint>

namespace arturo {

// Serial port configuration — testable without hardware
struct SerialConfig {
    uint32_t baudRate;
    uint8_t dataBits;   // 5, 6, 7, 8
    char parity;        // 'N' = none, 'E' = even, 'O' = odd
    uint8_t stopBits;   // 1, 2
};

// Parse a serial config shorthand like "2400-7E1" or "9600-8N1"
// Returns true if parsed successfully
bool parseSerialConfig(const char* configStr, SerialConfig& config);

// Build the Arduino serial mode constant from config
// Returns the SERIAL_* mode value (e.g., SERIAL_8N1 = 0x800001c)
// On non-Arduino, returns a packed representation for testing
uint32_t serialConfigToMode(const SerialConfig& config);

// Default configs for known protocols
extern const SerialConfig SERIAL_CONFIG_CTI;      // 2400-7E1
extern const SerialConfig SERIAL_CONFIG_MODBUS;   // 9600-8N1
extern const SerialConfig SERIAL_CONFIG_ASCII;    // 115200-8N1

#ifdef ARDUINO
#include <HardwareSerial.h>

class SerialDevice {
public:
    // uartNum: 0=USB, 1=UART1, 2=UART2
    SerialDevice(int uartNum = 1);

    // Begin serial with config, optionally specifying pins
    bool begin(const SerialConfig& config, int rxPin = -1, int txPin = -1);
    void end();

    bool isReady() const { return _ready; }

    // Send raw bytes. Returns number of bytes written.
    int send(const uint8_t* data, size_t len);

    // Send null-terminated string.
    int sendString(const char* str);

    // Receive bytes with timeout. Returns bytes read, or -1 on timeout with no data.
    int receive(uint8_t* buf, size_t bufLen, unsigned long timeoutMs = 1000);

    // Receive until terminator character. Strips terminator.
    // Returns line length, or -1 on timeout.
    int receiveLine(char* buf, size_t bufLen, char terminator = '\n',
                    unsigned long timeoutMs = 1000);

    // Receive exact number of bytes with timeout.
    // Returns bytes read (may be < expected on timeout), or -1 on error.
    int receiveExact(uint8_t* buf, size_t expected, unsigned long timeoutMs = 1000);

    // Flush TX and drain RX
    void flush();
    void drain();

    // Accessors
    const SerialConfig& config() const { return _config; }

private:
    HardwareSerial _serial;
    SerialConfig _config;
    bool _ready;
};
#endif

} // namespace arturo
