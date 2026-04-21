#pragma once

#ifdef ARDUINO
#include <freertos/FreeRTOS.h>
#include <freertos/queue.h>
#endif

#include <cstddef>
#include <cstdint>

namespace arturo {

// Error codes posted back on CtiRequest.error by CtiWorker.
enum CtiError : int8_t {
    CTI_ERR_NONE       = 0,
    CTI_ERR_TIMEOUT    = 1,
    CTI_ERR_OVERRUN    = 2,
    CTI_ERR_CHECKSUM   = 3,
    CTI_ERR_DEVICE     = 4,   // response code != 'A' and != 'B'
    CTI_ERR_QUEUE_FULL = 5,
};

// Max CTI command string length, e.g. "S1", "D?", "D1". Keep small; longest
// legitimate CTI command is 2-3 chars.
static const int CTI_REQUEST_CMD_LEN   = 16;

// Max CTI reply payload (data between code and checksum).
static const int CTI_REQUEST_REPLY_LEN = 80;

// Worker-side timeout for a single request. Mirrors CTI_TIMEOUT_MS but
// kept separate so the worker's timing is self-contained.
static const unsigned long CTI_REQUEST_TIMEOUT_MS = 600;

#ifdef ARDUINO
// Request/response carrier. Caller fills request[], replyQueue, and
// (optionally) flushBefore+requestIndex. Worker fills reply[], code, error.
struct CtiRequest {
    QueueHandle_t replyQueue;                      // worker posts this struct back here
    char          request[CTI_REQUEST_CMD_LEN];
    char          reply[CTI_REQUEST_REPLY_LEN];
    int8_t        error;                           // CtiError
    char          code;                            // raw response code char
    bool          flushBefore;                     // do flush() before sending
    uint8_t       requestIndex;                    // opaque caller tag
};
#endif

} // namespace arturo
