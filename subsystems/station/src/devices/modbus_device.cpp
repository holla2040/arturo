#include "modbus_device.h"
#include <cstring>

#ifdef ARDUINO
#include "../debug_log.h"
#endif

namespace arturo {

const ModbusDeviceConfig MODBUS_DEFAULT_CONFIG = {
    1,      // slaveAddr
    9600,   // baudRate
    1000,   // responseTimeoutMs
    5       // turnaroundDelayMs
};

bool validateModbusConfig(const ModbusDeviceConfig& config) {
    if (config.slaveAddr == 0 || config.slaveAddr > 247) return false;  // 1-247 valid
    if (config.baudRate == 0) return false;
    if (config.responseTimeoutMs == 0) return false;
    return true;
}

unsigned long modbusCharTimeoutUs(uint32_t baudRate) {
    // 1 character = 11 bits (start + 8 data + parity + stop)
    // 1.5 char times for inter-character timeout
    // At 9600 baud: 11/9600 * 1.5 * 1e6 = 1718.75 us
    if (baudRate == 0) return 0;
    // For baud rates <= 19200, use calculated value
    // For higher baud rates, Modbus spec says fixed 750us
    if (baudRate > 19200) return 750;
    return (unsigned long)((11UL * 1500000UL) / baudRate);
}

unsigned long modbusFrameSilenceUs(uint32_t baudRate) {
    // 3.5 char times for inter-frame silence
    // At 9600 baud: 11/9600 * 3.5 * 1e6 = 4010.4 us
    if (baudRate == 0) return 0;
    if (baudRate > 19200) return 1750;
    return (unsigned long)((11UL * 3500000UL) / baudRate);
}

#ifdef ARDUINO
ModbusDevice::ModbusDevice()
    : _serial(nullptr), _config{0, 0, 0, 0}, _transactions(0),
      _errors(0), _initialized(false) {
    memset(&_lastResp, 0, sizeof(_lastResp));
}

bool ModbusDevice::init(SerialDevice& serial, const ModbusDeviceConfig& config) {
    if (!validateModbusConfig(config)) {
        LOG_ERROR("MODBUS", "Invalid config: slave=%d baud=%lu",
                  config.slaveAddr, (unsigned long)config.baudRate);
        return false;
    }

    _serial = &serial;
    _config = config;
    _initialized = true;

    LOG_INFO("MODBUS", "Initialized: slave=%d, baud=%lu, timeout=%lums",
             config.slaveAddr, (unsigned long)config.baudRate,
             config.responseTimeoutMs);
    return true;
}

bool ModbusDevice::sendAndReceive(const uint8_t* txBuf, size_t txLen,
                                  uint8_t* rxBuf, size_t expectedRxLen) {
    if (!_initialized || _serial == nullptr) return false;

    // Drain any stale data
    _serial->drain();

    // Send request
    int sent = _serial->send(txBuf, txLen);
    if (sent < 0 || (size_t)sent != txLen) {
        LOG_ERROR("MODBUS", "TX failed: sent %d/%zu bytes", sent, txLen);
        _errors++;
        return false;
    }
    _serial->flush();

    LOG_TRACE("MODBUS", "TX %zu bytes to slave %d", txLen, _config.slaveAddr);

    // Turnaround delay
    delay(_config.turnaroundDelayMs);

    // Receive response
    int rxLen = _serial->receiveExact(rxBuf, expectedRxLen, _config.responseTimeoutMs);
    if (rxLen < 4) {  // minimum Modbus frame is 4 bytes
        LOG_ERROR("MODBUS", "RX timeout or short frame: got %d bytes, expected %zu",
                  rxLen, expectedRxLen);
        _errors++;
        return false;
    }

    _transactions++;
    LOG_TRACE("MODBUS", "RX %d bytes", rxLen);
    return modbusParseResponse(rxBuf, (size_t)rxLen, _lastResp);
}

int ModbusDevice::readHolding(uint16_t startReg, uint16_t regCount,
                              uint16_t* values, int maxValues) {
    uint8_t txBuf[8];
    int txLen = modbusBuildReadHolding(_config.slaveAddr, startReg, regCount,
                                       txBuf, sizeof(txBuf));
    if (txLen < 0) {
        LOG_ERROR("MODBUS", "Failed to build read holding request");
        return -1;
    }

    size_t expectedLen = modbusExpectedResponseLen(MODBUS_FC_READ_HOLDING, regCount);
    uint8_t rxBuf[256];
    if (!sendAndReceive(txBuf, txLen, rxBuf, expectedLen)) {
        return -1;
    }

    if (!_lastResp.crcValid) {
        LOG_ERROR("MODBUS", "CRC mismatch on read holding response");
        _errors++;
        return -1;
    }

    if (_lastResp.isException) {
        LOG_ERROR("MODBUS", "Exception %d reading registers 0x%04X",
                  _lastResp.exceptionCode, startReg);
        _errors++;
        return -1;
    }

    return modbusExtractRegisters(_lastResp, values, maxValues);
}

bool ModbusDevice::writeSingle(uint16_t reg, uint16_t value) {
    uint8_t txBuf[8];
    int txLen = modbusBuildWriteSingle(_config.slaveAddr, reg, value,
                                       txBuf, sizeof(txBuf));
    if (txLen < 0) {
        LOG_ERROR("MODBUS", "Failed to build write single request");
        return false;
    }

    size_t expectedLen = modbusExpectedResponseLen(MODBUS_FC_WRITE_SINGLE, 0);
    uint8_t rxBuf[16];
    if (!sendAndReceive(txBuf, txLen, rxBuf, expectedLen)) {
        return false;
    }

    if (!_lastResp.crcValid) {
        LOG_ERROR("MODBUS", "CRC mismatch on write single response");
        _errors++;
        return false;
    }

    if (_lastResp.isException) {
        LOG_ERROR("MODBUS", "Exception %d writing register 0x%04X",
                  _lastResp.exceptionCode, reg);
        _errors++;
        return false;
    }

    LOG_DEBUG("MODBUS", "Wrote 0x%04X to register 0x%04X", value, reg);
    return true;
}

bool ModbusDevice::writeMultiple(uint16_t startReg, uint16_t regCount,
                                 const uint16_t* values) {
    uint8_t txBuf[256];
    int txLen = modbusBuildWriteMultiple(_config.slaveAddr, startReg, regCount,
                                         values, txBuf, sizeof(txBuf));
    if (txLen < 0) {
        LOG_ERROR("MODBUS", "Failed to build write multiple request");
        return false;
    }

    size_t expectedLen = modbusExpectedResponseLen(MODBUS_FC_WRITE_MULTIPLE, 0);
    uint8_t rxBuf[16];
    if (!sendAndReceive(txBuf, txLen, rxBuf, expectedLen)) {
        return false;
    }

    if (!_lastResp.crcValid) {
        LOG_ERROR("MODBUS", "CRC mismatch on write multiple response");
        _errors++;
        return false;
    }

    if (_lastResp.isException) {
        LOG_ERROR("MODBUS", "Exception %d writing %d registers at 0x%04X",
                  _lastResp.exceptionCode, regCount, startReg);
        _errors++;
        return false;
    }

    LOG_DEBUG("MODBUS", "Wrote %d registers starting at 0x%04X", regCount, startReg);
    return true;
}
#endif

} // namespace arturo
