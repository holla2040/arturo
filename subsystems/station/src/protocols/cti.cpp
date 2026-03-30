#include "cti.h"
#include <cstring>
#include <cstdlib>

#ifdef ARDUINO
#include "../debug_log.h"
#endif

namespace arturo {

uint8_t ctiChecksum(const char* data, size_t len) {
    uint8_t sum = 0;
    for (size_t i = 0; i < len; i++) {
        sum += (uint8_t)data[i];
    }

    uint8_t d7d6 = sum >> 6;
    uint8_t d1d0 = sum & 0x03;
    uint8_t xor_val = d7d6 ^ d1d0;

    return (((sum & 0xFC) + xor_val) & 0x3F) + 0x30;
}

int ctiBuildFrame(const char* command, char* output, size_t outputLen) {
    if (command == nullptr || output == nullptr) return -1;

    size_t cmdLen = strlen(command);
    // Frame: '$' + command + checksum(1) + '\r' + null
    size_t frameLen = 1 + cmdLen + 1 + 1;

    if (frameLen >= outputLen) return -1;

    output[0] = '$';
    memcpy(output + 1, command, cmdLen);

    // Checksum covers the command characters only
    uint8_t chk = ctiChecksum(command, cmdLen);
    output[1 + cmdLen] = (char)chk;
    output[1 + cmdLen + 1] = '\r';
    output[1 + cmdLen + 2] = '\0';

    return (int)frameLen;
}

bool ctiParseFrame(const char* frame, size_t frameLen, CtiResponse& response) {
    // Minimum frame: $<code><checksum>\r = 4 chars
    if (frame == nullptr || frameLen < 4) return false;

    // Must start with '$'
    if (frame[0] != '$') return false;

    // Must end with '\r'
    if (frame[frameLen - 1] != '\r') return false;

    // Checksum is the character just before '\r'
    char receivedChk = frame[frameLen - 2];

    // Response code is the first char after '$'
    char codeChar = frame[1];
    switch (codeChar) {
        case 'A': response.code = CtiResponseCode::SUCCESS; break;
        case 'B': response.code = CtiResponseCode::SUCCESS_POWER_FAIL; break;
        case 'E': response.code = CtiResponseCode::INVALID_COMMAND; break;
        case 'F': response.code = CtiResponseCode::INVALID_POWER_FAIL; break;
        case 'G': response.code = CtiResponseCode::INTERLOCKS_ACTIVE; break;
        case 'H': response.code = CtiResponseCode::INTERLOCKS_POWER; break;
        default:  response.code = CtiResponseCode::UNKNOWN; break;
    }

    // Data is between code and checksum: frame[2] .. frame[frameLen-3]
    // Content to checksum: code + data = frame[1] .. frame[frameLen-3]
    size_t contentLen = frameLen - 3;  // skip '$', checksum, '\r'
    size_t dataLen = (contentLen > 1) ? contentLen - 1 : 0;  // skip code char

    if (dataLen >= sizeof(response.data)) {
        dataLen = sizeof(response.data) - 1;
    }

    if (dataLen > 0) {
        memcpy(response.data, frame + 2, dataLen);
    }
    response.data[dataLen] = '\0';
    response.dataLen = dataLen;

    // Validate checksum: computed over code + data (everything between '$' and checksum)
    uint8_t expectedChk = ctiChecksum(frame + 1, contentLen);
    response.checksumValid = ((char)expectedChk == receivedChk);

    return true;
}

bool ctiIsDataValid(CtiResponseCode code) {
    return code == CtiResponseCode::SUCCESS ||
           code == CtiResponseCode::SUCCESS_POWER_FAIL;
}

bool ctiIsSuccess(CtiResponseCode code) {
    return code == CtiResponseCode::SUCCESS ||
           code == CtiResponseCode::SUCCESS_POWER_FAIL;
}

bool ctiParseStatusByte(const char* hexStr, uint8_t& status) {
    if (hexStr == nullptr || strlen(hexStr) == 0) return false;

    char* endPtr = nullptr;
    long val = strtol(hexStr, &endPtr, 16);

    // Must consume at least some chars and value must fit in uint8_t
    if (endPtr == hexStr) return false;
    if (val < 0 || val > 255) return false;

    status = (uint8_t)val;
    return true;
}

} // namespace arturo
