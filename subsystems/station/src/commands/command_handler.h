#pragma once
#include <ArduinoJson.h>
#include "../messaging/envelope.h"
#include "../operational_mode.h"

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
class RedisClient;        // forward declare
class CtiOnBoardDevice;   // forward declare
class OTAUpdateHandler;   // forward declare

class CommandHandler {
public:
    // subRedis: subscribed client for reading commands
    // pubRedis: general client for publishing responses
    CommandHandler(RedisClient& subRedis, RedisClient& pubRedis, const char* instance);
    // Poll for one command with given timeout.
    // Returns true if a command was processed.
    bool poll(unsigned long timeoutMs = 100);
    int commandsProcessed() const { return _processed; }
    int commandsFailed() const { return _failed; }

    // Execute a command locally (from UI controls, not Redis).
    // Reuses device registry lookup + protocol dispatch without JSON/Redis overhead.
    // Caller must hold _ctiMutex.
    bool executeLocal(const char* commandName, char* responseBuf, size_t responseBufLen);

    // Current test state (updated from test.state.update messages)
    const TestState& testState() const { return _testState; }

    // Register a CTI OnBoard device for command dispatch
    void setCtiOnBoardDevice(CtiOnBoardDevice* device) { _ctiOnBoardDevice = device; }

    // Register an OTA update handler
    void setOTAHandler(OTAUpdateHandler* handler) { _otaHandler = handler; }

private:
    RedisClient& _subRedis;
    RedisClient& _pubRedis;
    const char* _instance;
    int _processed = 0;
    int _failed = 0;
    char _channelName[64];
    CtiOnBoardDevice* _ctiOnBoardDevice = nullptr;
    OTAUpdateHandler* _otaHandler = nullptr;
    TestState _testState;

    void handleMessage(const char* messageJson);
    void handleDeviceCommand(const char* messageJson);
    void handleTestStateUpdate(JsonDocument& doc);
    void handleOTARequest(JsonDocument& doc);
    void sendOTAResponse(const char* correlationId, const char* replyTo,
                         bool success, const char* response,
                         const char* errorCode, const char* errorMessage);

    // Shared dispatch logic used by both handleDeviceCommand() and executeLocal()
    bool dispatchToDevice(const char* deviceId, const char* commandName,
                          char* responseBuf, size_t responseBufLen,
                          const char*& errorCode, const char*& errorMessage);
};
#endif

} // namespace arturo
