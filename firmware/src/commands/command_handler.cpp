#include "command_handler.h"
#include "../config.h"
#include "device_registry.h"
#include <cstring>

#ifdef ARDUINO
#include "../debug_log.h"
#include "../network/redis_client.h"
#include "../devices/cti_onboard_device.h"
#endif

namespace arturo {

bool parseCommandRequest(const char* json, JsonDocument& doc, CommandRequest& req) {
    DeserializationError err = deserializeJson(doc, json);
    if (err) {
        return false;
    }

    JsonObjectConst envelope = doc["envelope"];
    if (envelope.isNull()) return false;

    // Validate type
    const char* type = envelope["type"];
    if (type == nullptr || strcmp(type, "device.command.request") != 0) {
        return false;
    }

    // Required envelope fields for command request
    if (!envelope["correlation_id"].is<const char*>()) return false;
    if (!envelope["reply_to"].is<const char*>()) return false;

    req.correlationId = envelope["correlation_id"].as<const char*>();
    req.replyTo = envelope["reply_to"].as<const char*>();

    // Payload fields
    JsonObjectConst payload = doc["payload"];
    if (payload.isNull()) return false;

    if (!payload["device_id"].is<const char*>()) return false;
    if (!payload["command_name"].is<const char*>()) return false;

    req.deviceId = payload["device_id"].as<const char*>();
    req.commandName = payload["command_name"].as<const char*>();
    req.timeoutMs = payload["timeout_ms"] | 5000;

    return true;
}

bool buildCommandResponse(JsonDocument& doc, const Source& source,
                          const char* id, int64_t timestamp,
                          const char* correlationId,
                          const char* deviceId, const char* commandName,
                          bool success, const char* response,
                          const char* errorCode, const char* errorMessage,
                          int durationMs) {
    if (!buildEnvelope(doc, source, "device.command.response",
                       id, timestamp, correlationId)) {
        return false;
    }

    JsonObject payload = doc["payload"].to<JsonObject>();
    payload["device_id"] = deviceId;
    payload["command_name"] = commandName;
    payload["success"] = success;

    if (success) {
        payload["response"] = response;
    } else {
        JsonObject error = payload["error"].to<JsonObject>();
        error["code"] = errorCode;
        error["message"] = errorMessage;
    }

    payload["duration_ms"] = durationMs;

    return true;
}

#ifdef ARDUINO
CommandHandler::CommandHandler(RedisClient& redis, const char* instance)
    : _redis(redis), _instance(instance) {
    strcpy(_lastStreamId, "0");
    snprintf(_streamName, sizeof(_streamName), "%s%s",
             CHANNEL_COMMANDS_PREFIX, _instance);
    LOG_INFO("CMD", "Listening on stream: %s", _streamName);
}

bool CommandHandler::poll(unsigned long blockMs) {
    char field[32];
    char value[2048];

    int result = _redis.xreadBlock(_streamName, _lastStreamId, blockMs,
                                    field, sizeof(field),
                                    value, sizeof(value));
    if (result <= 0) {
        return false;
    }

    // Update last stream ID for next read
    const char* entryId = _redis.lastEntryId();
    if (entryId[0] != '\0') {
        strncpy(_lastStreamId, entryId, sizeof(_lastStreamId) - 1);
        _lastStreamId[sizeof(_lastStreamId) - 1] = '\0';
    }

    handleMessage(value);
    return true;
}

void CommandHandler::handleMessage(const char* messageJson) {
    JsonDocument reqDoc;
    CommandRequest req;

    if (!parseCommandRequest(messageJson, reqDoc, req)) {
        LOG_ERROR("CMD", "Failed to parse command request");
        _failed++;
        return;
    }

    LOG_INFO("CMD", "Command: %s for device %s (corr=%s)",
             req.commandName, req.deviceId, req.correlationId);

    unsigned long startMs = millis();

    // Look up the device in the registry
    const DeviceInfo* device = getDevice(req.deviceId);
    bool success = false;
    char responseBuf[256] = {0};
    const char* errorCode = nullptr;
    const char* errorMessage = nullptr;

    if (device == nullptr) {
        errorCode = "device_not_found";
        errorMessage = "Device not registered on this station";
        LOG_ERROR("CMD", "Unknown device: %s", req.deviceId);
    } else if (strcmp(device->protocolType, "cti") == 0) {
        // CTI protocol dispatch
        if (_ctiOnBoardDevice == nullptr) {
            errorCode = "device_unavailable";
            errorMessage = "CTI OnBoard device not initialized";
            LOG_ERROR("CMD", "CTI OnBoard device not available for %s", req.deviceId);
        } else {
            const char* ctiCmd = ctiOnBoardLookupCommand(req.commandName);
            if (ctiCmd == nullptr) {
                errorCode = "unknown_command";
                errorMessage = "Command not in CTI OnBoard command table";
                LOG_ERROR("CMD", "Unknown CTI OnBoard command: %s", req.commandName);
            } else {
                success = _ctiOnBoardDevice->executeCommand(ctiCmd, responseBuf, sizeof(responseBuf));
                if (!success) {
                    errorCode = "device_error";
                    errorMessage = "CTI OnBoard command failed";
                }
            }
        }
    } else {
        // Other protocols (scpi, modbus) — placeholder for future dispatch
        errorCode = "unsupported_protocol";
        errorMessage = "Protocol dispatch not yet implemented";
        LOG_ERROR("CMD", "No dispatcher for protocol: %s", device->protocolType);
    }

    unsigned long durationMs = millis() - startMs;

    // Build response
    JsonDocument respDoc;
    Source src = { STATION_SERVICE, STATION_INSTANCE, STATION_VERSION };

    // Generate a simple ID (in main.cpp we have generateUUID, but here we use a counter-based ID)
    char respId[48];
    snprintf(respId, sizeof(respId), "resp-%s-%d", _instance, _processed);

    // Use millis as timestamp fallback (main.cpp has getTimestamp, but we keep it simple here)
    int64_t timestamp = (int64_t)(millis() / 1000);

    if (!buildCommandResponse(respDoc, src, respId, timestamp,
                              req.correlationId, req.deviceId, req.commandName,
                              success,
                              success ? responseBuf : nullptr,
                              errorCode, errorMessage,
                              (int)durationMs)) {
        LOG_ERROR("CMD", "Failed to build command response");
        _failed++;
        return;
    }

    char buffer[2048];
    serializeJson(respDoc, buffer, sizeof(buffer));

    // XADD response to reply_to stream
    char entryId[32];
    if (!_redis.xadd(req.replyTo, "message", buffer, entryId, sizeof(entryId))) {
        LOG_ERROR("CMD", "Failed to XADD response to %s", req.replyTo);
        _failed++;
        return;
    }

    _processed++;
    LOG_INFO("CMD", "Response sent to %s (entry=%s)", req.replyTo, entryId);
}
#endif

} // namespace arturo
