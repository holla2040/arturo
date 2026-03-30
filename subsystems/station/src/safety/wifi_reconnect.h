#pragma once
#include <cstdint>

namespace arturo {

// WiFi connection states
enum class WifiState : uint8_t {
    DISCONNECTED = 0,
    CONNECTING   = 1,
    CONNECTED    = 2
};

// Backoff configuration
struct BackoffConfig {
    int initialMs;   // Starting backoff interval
    int maxMs;       // Maximum backoff interval
    int multiplier;  // Multiplier per failure (typically 2)
};

// Default backoff: 1s initial, 30s max, doubling
extern const BackoffConfig BACKOFF_DEFAULT;

// Calculate next backoff interval after a failure
// Returns clamped value: min(currentMs * multiplier, maxMs)
int backoffNext(int currentMs, int multiplier, int maxMs);

// Check if enough time has elapsed since last attempt for the given backoff
bool backoffReady(unsigned long lastAttemptMs, unsigned long nowMs, int backoffMs);

// Calculate how many consecutive failures before reaching max backoff
// Given initial=1000, multiplier=2, max=30000 -> 5 failures (1000,2000,4000,8000,16000,30000)
int backoffStepsToMax(int initialMs, int multiplier, int maxMs);

// --- Command Queue for preserving commands during WiFi outages ---

static const int COMMAND_QUEUE_MAX = 16;
static const int COMMAND_QUEUE_ENTRY_SIZE = 256;

// Circular queue entry
struct QueueEntry {
    char data[COMMAND_QUEUE_ENTRY_SIZE];
    int length;
    bool occupied;
};

// Check if a circular queue has space
bool queueHasSpace(int head, int tail, int capacity);

// Calculate number of items in circular queue
int queueCount(int head, int tail, int capacity);

// Advance a circular queue index
int queueAdvance(int index, int capacity);

// Connection metrics — testable snapshot
struct WifiMetrics {
    int reconnectCount;
    int failedAttempts;
    unsigned long totalDisconnectedMs;
    unsigned long longestOutageMs;
    unsigned long lastConnectedMs;
    unsigned long lastDisconnectedMs;
    int queuedCommands;
    int droppedCommands;
};

// Calculate outage duration from disconnect to reconnect
unsigned long outrageDuration(unsigned long disconnectedMs, unsigned long reconnectedMs);

#ifdef ARDUINO

class CommandQueue {
public:
    CommandQueue();

    // Enqueue a command (returns false if queue full, command dropped)
    bool enqueue(const char* data, int length);

    // Dequeue next command (returns false if empty)
    bool dequeue(char* data, int maxLength, int* outLength);

    // Peek at front without removing
    bool peek(char* data, int maxLength, int* outLength) const;

    int count() const;
    bool isEmpty() const;
    bool isFull() const;
    void clear();

    int droppedCount() const { return _dropped; }

private:
    QueueEntry _entries[COMMAND_QUEUE_MAX];
    int _head;  // Next slot to dequeue from
    int _tail;  // Next slot to enqueue to
    int _count;
    int _dropped;
};

#endif

} // namespace arturo
