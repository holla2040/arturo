#pragma once
#include <cstdint>
#include <cstddef>

namespace arturo {

// CTI response codes (first char after '$')
enum class CtiResponseCode : char {
    SUCCESS             = 'A',
    SUCCESS_POWER_FAIL  = 'B',
    INVALID_COMMAND     = 'E',
    INVALID_POWER_FAIL  = 'F',
    INTERLOCKS_ACTIVE   = 'G',
    INTERLOCKS_POWER    = 'H',
    UNKNOWN             = '?'
};

// Parsed CTI response
struct CtiResponse {
    CtiResponseCode code;
    char data[64];       // response data (between code and checksum)
    size_t dataLen;
    bool checksumValid;
};

// --- Testable pure functions ---

// Compute CTI checksum for a command/data string
// Returns single ASCII character in range 0x30-0x6F
uint8_t ctiChecksum(const char* data, size_t len);

// Build a CTI request frame: $<command><checksum>\r
// Returns frame length, or -1 if buffer too small
int ctiBuildFrame(const char* command, char* output, size_t outputLen);

// Parse a CTI response frame: $<code><data><checksum>\r
// Returns true if frame structure is valid (regardless of checksum match)
bool ctiParseFrame(const char* frame, size_t frameLen, CtiResponse& response);

// Check if a response code indicates valid data
bool ctiIsDataValid(CtiResponseCode code);

// Check if a response code indicates success (A or B)
bool ctiIsSuccess(CtiResponseCode code);

// Parse a CTI status byte (hex string to uint8_t)
// Status bytes from S1/S2/S3 commands are 2-char hex, NOT decimal
bool ctiParseStatusByte(const char* hexStr, uint8_t& status);

// CTI timing constants
static const unsigned long CTI_TIMEOUT_MS         = 600;
static const unsigned long CTI_POLL_INTERVAL_MS   = 150;
static const unsigned long CTI_BACKOFF_INTERVAL_MS = 5000;
static const int CTI_OFFLINE_THRESHOLD            = 2;
static const int CTI_BACKOFF_THRESHOLD            = 5;

} // namespace arturo
