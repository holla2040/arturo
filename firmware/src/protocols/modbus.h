#pragma once
#include <cstdint>
#include <cstddef>

namespace arturo {

// Modbus RTU function codes
static const uint8_t MODBUS_FC_READ_HOLDING    = 0x03;
static const uint8_t MODBUS_FC_WRITE_SINGLE    = 0x06;
static const uint8_t MODBUS_FC_WRITE_MULTIPLE  = 0x10;

// Modbus exception codes
static const uint8_t MODBUS_EX_ILLEGAL_FUNCTION = 0x01;
static const uint8_t MODBUS_EX_ILLEGAL_ADDRESS  = 0x02;
static const uint8_t MODBUS_EX_ILLEGAL_VALUE    = 0x03;
static const uint8_t MODBUS_EX_DEVICE_FAILURE   = 0x04;

// Max registers per request (Modbus spec limit is 125 for FC03)
static const int MODBUS_MAX_REGISTERS = 125;

// --- Testable pure functions ---

// Compute Modbus CRC16 over a byte buffer
uint16_t modbusCrc16(const uint8_t* data, size_t len);

// Build a Read Holding Registers (FC 0x03) request frame
// Returns frame length, or -1 on error
// Frame: [slave][0x03][startHi][startLo][countHi][countLo][crcLo][crcHi]
int modbusBuildReadHolding(uint8_t slaveAddr, uint16_t startReg, uint16_t regCount,
                           uint8_t* output, size_t outputLen);

// Build a Write Single Register (FC 0x06) request frame
// Returns frame length, or -1 on error
// Frame: [slave][0x06][regHi][regLo][valHi][valLo][crcLo][crcHi]
int modbusBuildWriteSingle(uint8_t slaveAddr, uint16_t reg, uint16_t value,
                           uint8_t* output, size_t outputLen);

// Build a Write Multiple Registers (FC 0x10) request frame
// Returns frame length, or -1 on error
int modbusBuildWriteMultiple(uint8_t slaveAddr, uint16_t startReg, uint16_t regCount,
                             const uint16_t* values,
                             uint8_t* output, size_t outputLen);

// Parsed Modbus response
struct ModbusResponse {
    uint8_t slaveAddr;
    uint8_t functionCode;
    bool isException;        // true if function code has high bit set
    uint8_t exceptionCode;   // valid only if isException
    uint8_t data[256];       // register data bytes
    size_t dataLen;
    bool crcValid;
};

// Parse a Modbus RTU response frame
// Returns true if frame structure is valid (CRC check reported in response.crcValid)
bool modbusParseResponse(const uint8_t* frame, size_t frameLen, ModbusResponse& response);

// Extract register values from a FC03 response
// Returns number of registers extracted, or -1 on error
int modbusExtractRegisters(const ModbusResponse& response, uint16_t* values, int maxRegisters);

// Calculate expected response length for a given request function code and register count
size_t modbusExpectedResponseLen(uint8_t functionCode, uint16_t regCount);

} // namespace arturo
