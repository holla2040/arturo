#pragma once
#include <ArduinoJson.h>
#include "../messaging/envelope.h"

namespace arturo {

struct CommandRequest {
    const char* correlationId;
    const char* replyTo;
    const char* deviceId;
    const char* commandName;
    int timeoutMs;
};

// Parse a command request JSON string into a CommandRequest struct.
// The JsonDocument must outlive the CommandRequest (pointers reference doc memory).
// Returns true if parsing succeeded and type is "device.command.request".
bool parseCommandRequest(const char* json, JsonDocument& doc, CommandRequest& req);

// Build a command response JSON message.
// For success: pass response string, errorCode=nullptr, errorMessage=nullptr.
// For error: pass response=nullptr, errorCode and errorMessage.
bool buildCommandResponse(JsonDocument& doc, const Source& source,
                          const char* id, int64_t timestamp,
                          const char* correlationId,
                          const char* deviceId, const char* commandName,
                          bool success, const char* response,
                          const char* errorCode, const char* errorMessage,
                          int durationMs);

#ifdef ARDUINO
class RedisClient; // forward declare
class CtiOnBoardDevice;   // forward declare

class CommandHandler {
public:
    CommandHandler(RedisClient& redis, const char* instance);
    // Poll for one command with given block timeout.
    // Returns true if a command was processed.
    bool poll(unsigned long blockMs = 100);
    int commandsProcessed() const { return _processed; }
    int commandsFailed() const { return _failed; }

    // Register a CTI OnBoard device for command dispatch
    void setCtiOnBoardDevice(CtiOnBoardDevice* device) { _ctiOnBoardDevice = device; }

private:
    RedisClient& _redis;
    const char* _instance;
    char _lastStreamId[32];
    int _processed = 0;
    int _failed = 0;
    char _streamName[64];
    CtiOnBoardDevice* _ctiOnBoardDevice = nullptr;

    void handleMessage(const char* messageJson);
};
#endif

} // namespace arturo
