#pragma once
#include <cstdint>
#include <cstddef>
#include "../protocols/modbus.h"

namespace arturo {

// Modbus device configuration — testable without hardware
struct ModbusDeviceConfig {
    uint8_t slaveAddr;
    uint32_t baudRate;
    unsigned long responseTimeoutMs;  // inter-frame timeout
    unsigned long turnaroundDelayMs;  // delay between TX and RX
};

// Validate a Modbus device config
bool validateModbusConfig(const ModbusDeviceConfig& config);

// Default Modbus RTU config (9600 baud, slave 1)
extern const ModbusDeviceConfig MODBUS_DEFAULT_CONFIG;

// Calculate inter-character timeout for a given baud rate (Modbus spec: 1.5 char times)
// Returns timeout in microseconds
unsigned long modbusCharTimeoutUs(uint32_t baudRate);

// Calculate inter-frame silence for a given baud rate (Modbus spec: 3.5 char times)
// Returns silence period in microseconds
unsigned long modbusFrameSilenceUs(uint32_t baudRate);

#ifdef ARDUINO
#include "serial_device.h"

class ModbusDevice {
public:
    ModbusDevice();

    // Initialize with serial device and config
    bool init(SerialDevice& serial, const ModbusDeviceConfig& config);

    // Read holding registers (FC 0x03)
    // Returns number of registers read, or -1 on error
    int readHolding(uint16_t startReg, uint16_t regCount,
                    uint16_t* values, int maxValues);

    // Write a single register (FC 0x06)
    bool writeSingle(uint16_t reg, uint16_t value);

    // Write multiple registers (FC 0x10)
    bool writeMultiple(uint16_t startReg, uint16_t regCount, const uint16_t* values);

    // Last response details (for diagnostics)
    const ModbusResponse& lastResponse() const { return _lastResp; }
    int transactionCount() const { return _transactions; }
    int errorCount() const { return _errors; }

private:
    bool sendAndReceive(const uint8_t* txBuf, size_t txLen,
                        uint8_t* rxBuf, size_t expectedRxLen);

    SerialDevice* _serial;
    ModbusDeviceConfig _config;
    ModbusResponse _lastResp;
    int _transactions;
    int _errors;
    bool _initialized;
};
#endif

} // namespace arturo
