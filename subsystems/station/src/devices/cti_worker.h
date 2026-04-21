#pragma once

#ifdef ARDUINO

#include "cti_request.h"
#include "serial_device.h"
#include <freertos/FreeRTOS.h>
#include <freertos/task.h>
#include <freertos/queue.h>

namespace arturo {

// CtiWorker owns the CTI UART exclusively. All callers post CtiRequests to
// its request queue; the worker's task executes them one at a time and posts
// results back to each request's per-caller replyQueue.
//
// State machine: IDLE -> WAITING -> IN_PACKET -> IDLE. A new request is only
// dequeued in IDLE, guaranteeing exactly one request in flight. On any
// completion (success, timeout, overrun, bad checksum) the worker calls
// flush() -- send '\r', delay 100ms, drain UART -- before returning to IDLE.
// That flush absorbs any late/in-flight response from the previous request,
// so a stale reply can never be paired with the next command.
//
// Ported from pendant firmware (pump.cpp) which proved this pattern.
class CtiWorker {
public:
    CtiWorker();

    // Spawn the worker task and create the request queue. Returns false if
    // queue creation or task creation fails.
    bool begin(SerialDevice& serial);

    // Synchronous fire-and-block convenience used by CtiOnBoardDevice.
    // Creates a per-call reply queue, enqueues a CtiRequest, blocks for
    // timeoutMs, copies reply into responseBuf, deletes the reply queue.
    // Returns true iff the worker posted CTI_ERR_NONE.
    bool executeCommand(const char* ctiCmd, char* responseBuf, size_t responseBufLen,
                        unsigned long timeoutMs = 800);

    // Non-blocking enqueue for callers that own a persistent reply queue.
    bool enqueue(const CtiRequest& req, TickType_t waitTicks = pdMS_TO_TICKS(50));

    bool isInitialized() const { return _initialized; }
    int  transactionCount() const { return _transactions; }
    int  errorCount()       const { return _errors; }

private:
    enum State : uint8_t { PARSER_IDLE, PARSER_WAITING, PARSER_IN_PACKET };

    static void taskEntry(void* param);
    void taskLoop();

    void requestSend(const char* req);        // build frame, TX, -> WAITING
    void parserReset();                       // -> IDLE, replyIndex = 0
    void parserCharHandler(char c);           // consume one RX byte
    bool replyValid();                        // checksum over _replyBuf[0 .. _replyIndex-2]
    void postResult(int8_t err);              // send _current to _current.replyQueue
    void flush();                             // '\r', 100ms, drain

    SerialDevice*  _serial;
    QueueHandle_t  _requestQueue;
    TaskHandle_t   _taskHandle;
    bool           _initialized;

    // State machine
    State          _state;
    unsigned long  _requestStartMs;
    uint8_t        _replyIndex;
    char           _replyBuf[CTI_REQUEST_REPLY_LEN];
    CtiRequest     _current;

    // Stats
    volatile int   _transactions;
    volatile int   _errors;
};

} // namespace arturo

#endif // ARDUINO
