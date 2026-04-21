#include "cti_worker.h"

#ifdef ARDUINO

#include "../protocols/cti.h"
#include "../debug_log.h"
#include <Arduino.h>
#include <cstring>

namespace arturo {

static const int  CTI_REQUEST_QUEUE_DEPTH = 4;
static const char CTI_PACKET_START        = '$';
static const char CTI_PACKET_END          = '\r';
static const TickType_t CTI_TASK_YIELD    = pdMS_TO_TICKS(2);
static const TickType_t CTI_FLUSH_DELAY   = pdMS_TO_TICKS(100);
static const TickType_t CTI_REPLY_POST_TIMEOUT = pdMS_TO_TICKS(50);

CtiWorker::CtiWorker()
    : _serial(nullptr),
      _requestQueue(nullptr),
      _taskHandle(nullptr),
      _initialized(false),
      _state(PARSER_IDLE),
      _requestStartMs(0),
      _replyIndex(0),
      _transactions(0),
      _errors(0) {
    memset(_replyBuf, 0, sizeof(_replyBuf));
    memset(&_current, 0, sizeof(_current));
}

bool CtiWorker::begin(SerialDevice& serial) {
    if (_initialized) return true;
    if (!serial.isReady()) {
        LOG_ERROR("CTI_WORKER", "Serial not ready");
        return false;
    }
    _serial = &serial;

    _requestQueue = xQueueCreate(CTI_REQUEST_QUEUE_DEPTH, sizeof(CtiRequest));
    if (_requestQueue == nullptr) {
        LOG_ERROR("CTI_WORKER", "Failed to create request queue");
        return false;
    }

    BaseType_t rc = xTaskCreatePinnedToCore(
        &CtiWorker::taskEntry, "tCtiWorker", 4096, this, 4, &_taskHandle, 1);
    if (rc != pdPASS) {
        LOG_ERROR("CTI_WORKER", "Failed to create task");
        vQueueDelete(_requestQueue);
        _requestQueue = nullptr;
        return false;
    }

    _initialized = true;
    LOG_INFO("CTI_WORKER", "Worker started (queue=%d, 4KB stack, core1 pri4)",
             CTI_REQUEST_QUEUE_DEPTH);
    return true;
}

bool CtiWorker::enqueue(const CtiRequest& req, TickType_t waitTicks) {
    if (!_initialized || _requestQueue == nullptr) return false;
    return xQueueSendToBack(_requestQueue, &req, waitTicks) == pdTRUE;
}

bool CtiWorker::executeCommand(const char* ctiCmd, char* responseBuf, size_t responseBufLen,
                                unsigned long timeoutMs) {
    if (!_initialized || ctiCmd == nullptr || responseBuf == nullptr || responseBufLen == 0) {
        return false;
    }

    QueueHandle_t rq = xQueueCreate(1, sizeof(CtiRequest));
    if (rq == nullptr) {
        LOG_ERROR("CTI_WORKER", "Failed to create per-call reply queue");
        return false;
    }

    CtiRequest req = {};
    req.replyQueue  = rq;
    strncpy(req.request, ctiCmd, CTI_REQUEST_CMD_LEN - 1);
    req.error       = CTI_ERR_NONE;
    req.flushBefore = false;

    if (!enqueue(req)) {
        LOG_ERROR("CTI_WORKER", "Request queue full, dropping '%s'", ctiCmd);
        vQueueDelete(rq);
        return false;
    }

    CtiRequest result;
    BaseType_t got = xQueueReceive(rq, &result, pdMS_TO_TICKS(timeoutMs));
    vQueueDelete(rq);

    if (got != pdTRUE) {
        // Worker didn't post in time -- should not happen (worker's own
        // timeout is shorter than our timeoutMs default) but guard anyway.
        LOG_ERROR("CTI_WORKER", "executeCommand wrapper timeout for '%s'", ctiCmd);
        return false;
    }

    if (result.error == CTI_ERR_NONE) {
        size_t n = strlen(result.reply);
        if (n >= responseBufLen) n = responseBufLen - 1;
        memcpy(responseBuf, result.reply, n);
        responseBuf[n] = '\0';
        return true;
    }

    if (result.error == CTI_ERR_DEVICE) {
        snprintf(responseBuf, responseBufLen, "ERR:%c", result.code);
        return false;
    }

    // Timeout / overrun / checksum
    responseBuf[0] = '\0';
    return false;
}

void CtiWorker::taskEntry(void* param) {
    static_cast<CtiWorker*>(param)->taskLoop();
}

void CtiWorker::taskLoop() {
    // Matches pendant TASKSTARTDELAY+4 -- give the pump a beat before we
    // start issuing commands, then flush a stray '\r' to clear it.
    vTaskDelay(pdMS_TO_TICKS(200));
    flush();
    LOG_INFO("CTI_WORKER", "Worker loop running");

    for (;;) {
        // Dequeue only while IDLE -- guarantees one-in-flight.
        if (_state == PARSER_IDLE) {
            if (xQueueReceive(_requestQueue, &_current, 0) == pdTRUE) {
                if (_current.flushBefore) flush();
                requestSend(_current.request);
                _requestStartMs = millis();
            }
        } else {
            // Timeout check
            if (millis() - _requestStartMs >= CTI_REQUEST_TIMEOUT_MS) {
                LOG_DEBUG("CTI_WORKER", "Timeout on '%s'", _current.request);
                _current.reply[0] = '\0';
                _current.code     = 0;
                postResult(CTI_ERR_TIMEOUT);
                flush();
                parserReset();
            }
        }

        // Drain any available RX bytes through the parser.
        int c;
        while ((c = _serial->readByte()) >= 0) {
            parserCharHandler((char)(c & 0x7F));   // strip parity bit, pendant convention
            // parserCharHandler may have reset state; keep draining either way.
        }

        vTaskDelay(CTI_TASK_YIELD);
    }
}

void CtiWorker::requestSend(const char* req) {
    char frame[CTI_REQUEST_CMD_LEN + 4];
    int frameLen = ctiBuildFrame(req, frame, sizeof(frame));
    if (frameLen < 0) {
        LOG_ERROR("CTI_WORKER", "Build frame failed for '%s'", req);
        _current.reply[0] = '\0';
        _current.code     = 0;
        postResult(CTI_ERR_OVERRUN);
        parserReset();
        return;
    }
    LOG_DEBUG("CTI_WORKER", "TX '%s'", req);
    _serial->send(reinterpret_cast<const uint8_t*>(frame), (size_t)frameLen);
    _serial->flush();
    _state      = PARSER_WAITING;
    _replyIndex = 0;
}

void CtiWorker::parserReset() {
    _state      = PARSER_IDLE;
    _replyIndex = 0;
}

void CtiWorker::parserCharHandler(char c) {
    switch (_state) {
        case PARSER_IDLE:
            // Stray byte with no request in flight -- ignore.
            break;

        case PARSER_WAITING:
            if (c == CTI_PACKET_START) {
                _state      = PARSER_IN_PACKET;
                _replyIndex = 0;
            }
            // else: junk between packets, ignore.
            break;

        case PARSER_IN_PACKET:
            if (c == CTI_PACKET_END) {
                // Packet complete. _replyBuf[0 .. _replyIndex-1] is
                // <code><data><checksum>. Validate checksum over
                // _replyBuf[0 .. _replyIndex-2], checksum is _replyBuf[_replyIndex-1].
                if (_replyIndex < 2) {
                    // runt frame -- at minimum need code+checksum
                    _current.reply[0] = '\0';
                    _current.code     = 0;
                    postResult(CTI_ERR_CHECKSUM);
                } else if (replyValid()) {
                    char codeChar = _replyBuf[0];
                    size_t dataLen = _replyIndex - 2;        // strip code + checksum
                    if (dataLen >= sizeof(_current.reply)) {
                        dataLen = sizeof(_current.reply) - 1;
                    }
                    memcpy(_current.reply, _replyBuf + 1, dataLen);
                    _current.reply[dataLen] = '\0';
                    _current.code = codeChar;

                    // A or B are success, anything else is device error.
                    if (codeChar == 'A' || codeChar == 'B') {
                        postResult(CTI_ERR_NONE);
                    } else {
                        postResult(CTI_ERR_DEVICE);
                    }
                } else {
                    LOG_ERROR("CTI_WORKER", "Checksum fail on '%s'", _current.request);
                    _current.reply[0] = '\0';
                    _current.code     = _replyBuf[0];
                    postResult(CTI_ERR_CHECKSUM);
                }
                flush();
                parserReset();
            } else {
                if (_replyIndex < CTI_REQUEST_REPLY_LEN - 1) {
                    _replyBuf[_replyIndex++] = c;
                } else {
                    LOG_ERROR("CTI_WORKER", "Overrun on '%s'", _current.request);
                    _current.reply[0] = '\0';
                    _current.code     = 0;
                    postResult(CTI_ERR_OVERRUN);
                    flush();
                    parserReset();
                }
            }
            break;
    }
}

bool CtiWorker::replyValid() {
    // _replyBuf[0 .. _replyIndex-2] = content (code + data); _replyBuf[_replyIndex-1] = checksum.
    uint8_t sent = (uint8_t)_replyBuf[_replyIndex - 1];
    uint8_t calc = ctiChecksum(_replyBuf, _replyIndex - 1);
    return sent == calc;
}

void CtiWorker::postResult(int8_t err) {
    _current.error = err;
    if (err == CTI_ERR_NONE) {
        _transactions++;
    } else {
        _errors++;
    }
    if (_current.replyQueue) {
        xQueueSendToBack(_current.replyQueue, &_current, CTI_REPLY_POST_TIMEOUT);
    }
}

void CtiWorker::flush() {
    // Pendant recipe: send '\r' as a null command, delay, drain.
    // Delay gives the pump time to respond (to this '\r' or any in-flight
    // previous command) before we drain, so late bytes cannot leak into the
    // next transaction.
    uint8_t cr = '\r';
    _serial->send(&cr, 1);
    _serial->flush();
    vTaskDelay(CTI_FLUSH_DELAY);
    _serial->drain();
}

} // namespace arturo

#endif // ARDUINO
