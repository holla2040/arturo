#include "wifi_reconnect.h"
#include <cstring>

#ifdef ARDUINO
#include "../debug_log.h"
#endif

namespace arturo {

const BackoffConfig BACKOFF_DEFAULT = {
    1000,   // initialMs
    30000,  // maxMs
    2       // multiplier (doubling)
};

int backoffNext(int currentMs, int multiplier, int maxMs) {
    if (currentMs <= 0) return maxMs > 0 ? 1 : 0;
    if (multiplier <= 1) return currentMs;

    // Check for overflow before multiplying
    int next = currentMs * multiplier;
    if (next / multiplier != currentMs) {
        // Overflow — clamp to max
        return maxMs;
    }
    return (next > maxMs) ? maxMs : next;
}

bool backoffReady(unsigned long lastAttemptMs, unsigned long nowMs, int backoffMs) {
    unsigned long elapsed = nowMs - lastAttemptMs;
    return elapsed >= (unsigned long)backoffMs;
}

int backoffStepsToMax(int initialMs, int multiplier, int maxMs) {
    if (initialMs <= 0 || multiplier <= 1 || maxMs <= 0) return 0;
    if (initialMs >= maxMs) return 0;

    int steps = 0;
    int current = initialMs;
    while (current < maxMs) {
        current = backoffNext(current, multiplier, maxMs);
        steps++;
    }
    return steps;
}

bool queueHasSpace(int head, int tail, int capacity) {
    if (capacity <= 0) return false;
    int count = (tail - head + capacity) % capacity;
    return count < capacity - 1;
}

int queueCount(int head, int tail, int capacity) {
    if (capacity <= 0) return 0;
    return (tail - head + capacity) % capacity;
}

int queueAdvance(int index, int capacity) {
    return (index + 1) % capacity;
}

unsigned long outrageDuration(unsigned long disconnectedMs, unsigned long reconnectedMs) {
    return reconnectedMs - disconnectedMs;
}

#ifdef ARDUINO

CommandQueue::CommandQueue()
    : _head(0), _tail(0), _count(0), _dropped(0) {
    for (int i = 0; i < COMMAND_QUEUE_MAX; i++) {
        _entries[i].occupied = false;
        _entries[i].length = 0;
        _entries[i].data[0] = '\0';
    }
}

bool CommandQueue::enqueue(const char* data, int length) {
    if (_count >= COMMAND_QUEUE_MAX) {
        _dropped++;
        LOG_ERROR("CMDQ", "Queue full, dropping command (dropped=%d)", _dropped);
        return false;
    }

    int copyLen = length;
    if (copyLen >= COMMAND_QUEUE_ENTRY_SIZE) {
        copyLen = COMMAND_QUEUE_ENTRY_SIZE - 1;
    }

    memcpy(_entries[_tail].data, data, copyLen);
    _entries[_tail].data[copyLen] = '\0';
    _entries[_tail].length = copyLen;
    _entries[_tail].occupied = true;

    _tail = (_tail + 1) % COMMAND_QUEUE_MAX;
    _count++;

    LOG_DEBUG("CMDQ", "Enqueued command (%d bytes), count=%d", copyLen, _count);
    return true;
}

bool CommandQueue::dequeue(char* data, int maxLength, int* outLength) {
    if (_count <= 0) return false;

    int copyLen = _entries[_head].length;
    if (copyLen >= maxLength) {
        copyLen = maxLength - 1;
    }

    memcpy(data, _entries[_head].data, copyLen);
    data[copyLen] = '\0';
    if (outLength) *outLength = copyLen;

    _entries[_head].occupied = false;
    _head = (_head + 1) % COMMAND_QUEUE_MAX;
    _count--;

    LOG_DEBUG("CMDQ", "Dequeued command (%d bytes), count=%d", copyLen, _count);
    return true;
}

bool CommandQueue::peek(char* data, int maxLength, int* outLength) const {
    if (_count <= 0) return false;

    int copyLen = _entries[_head].length;
    if (copyLen >= maxLength) {
        copyLen = maxLength - 1;
    }

    memcpy(data, _entries[_head].data, copyLen);
    data[copyLen] = '\0';
    if (outLength) *outLength = copyLen;

    return true;
}

int CommandQueue::count() const {
    return _count;
}

bool CommandQueue::isEmpty() const {
    return _count == 0;
}

bool CommandQueue::isFull() const {
    return _count >= COMMAND_QUEUE_MAX;
}

void CommandQueue::clear() {
    _head = 0;
    _tail = 0;
    _count = 0;
    for (int i = 0; i < COMMAND_QUEUE_MAX; i++) {
        _entries[i].occupied = false;
    }
    LOG_DEBUG("CMDQ", "Queue cleared");
}

#endif

} // namespace arturo
