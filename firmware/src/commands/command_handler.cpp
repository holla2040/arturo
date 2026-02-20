#include "command_handler.h"
#include "../config.h"
#include "device_registry.h"
#include <cstring>

#ifdef ARDUINO
#include "../debug_log.h"
#include "../network/redis_client.h"
#include "../devices/cti_onboard_device.h"
#include "../safety/ota_update.h"
#include <esp_system.h>
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
    // Extract envelope.type to route to the correct handler
    JsonDocument doc;
    DeserializationError err = deserializeJson(doc, messageJson);
    if (err) {
        LOG_ERROR("CMD", "Failed to parse message JSON: %s", err.c_str());
        _failed++;
        return;
    }

    const char* type = doc["envelope"]["type"];
    if (type == nullptr) {
        LOG_ERROR("CMD", "Message missing envelope.type");
        _failed++;
        return;
    }

    if (strcmp(type, "device.command.request") == 0) {
        handleDeviceCommand(messageJson);
    } else if (strcmp(type, "system.ota.request") == 0) {
        handleOTARequest(doc);
    } else {
        LOG_ERROR("CMD", "Unknown message type: %s", type);
        _failed++;
    }
}

void CommandHandler::handleDeviceCommand(const char* messageJson) {
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

void CommandHandler::handleOTARequest(JsonDocument& doc) {
    const char* correlationId = doc["envelope"]["correlation_id"];
    const char* replyTo = doc["envelope"]["reply_to"];

    if (correlationId == nullptr || replyTo == nullptr) {
        LOG_ERROR("OTA", "OTA request missing correlation_id or reply_to");
        _failed++;
        return;
    }

    // Parse OTA payload fields
    JsonObjectConst payload = doc["payload"];
    const char* firmwareUrl = payload["firmware_url"];
    const char* version = payload["version"];
    const char* sha256 = payload["sha256"];
    bool force = payload["force"] | false;

    LOG_INFO("OTA", "OTA request: version=%s url=%s force=%d (corr=%s)",
             version ? version : "null",
             firmwareUrl ? firmwareUrl : "null",
             force ? 1 : 0, correlationId);

    if (_otaHandler == nullptr) {
        LOG_ERROR("OTA", "OTA handler not initialized");
        sendOTAResponse(correlationId, replyTo, false, nullptr,
                        "ota_unavailable", "OTA handler not initialized");
        _failed++;
        return;
    }

    // Parse and validate request
    OTARequest req;
    if (!parseOTAPayload(firmwareUrl, version, sha256, force, req)) {
        LOG_ERROR("OTA", "Failed to parse OTA payload");
        sendOTAResponse(correlationId, replyTo, false, nullptr,
                        "invalid_payload", "Missing or invalid OTA payload fields");
        _failed++;
        return;
    }

    // Attempt the update (downloads, verifies SHA256, writes to flash)
    bool success = _otaHandler->startUpdate(req, FIRMWARE_VERSION);

    if (success) {
        // Send success response BEFORE rebooting
        char responseBuf[128];
        snprintf(responseBuf, sizeof(responseBuf),
                 "OTA update to %s complete, rebooting", req.version);
        sendOTAResponse(correlationId, replyTo, true, responseBuf, nullptr, nullptr);
        _processed++;
        LOG_INFO("OTA", "Response sent, rebooting in 500ms...");
        delay(500);
        esp_restart();
    } else {
        const char* errStr = otaErrorToString(_otaHandler->lastError());
        LOG_ERROR("OTA", "OTA update failed: %s", errStr);
        sendOTAResponse(correlationId, replyTo, false, nullptr, errStr, errStr);
        _failed++;
    }
}

void CommandHandler::sendOTAResponse(const char* correlationId, const char* replyTo,
                                     bool success, const char* response,
                                     const char* errorCode, const char* errorMessage) {
    JsonDocument respDoc;
    Source src = { STATION_SERVICE, STATION_INSTANCE, STATION_VERSION };

    char respId[48];
    snprintf(respId, sizeof(respId), "resp-%s-%d", _instance, _processed);
    int64_t timestamp = (int64_t)(millis() / 1000);

    if (!buildCommandResponse(respDoc, src, respId, timestamp,
                              correlationId, STATION_INSTANCE, "ota_update",
                              success, response, errorCode, errorMessage, 0)) {
        LOG_ERROR("OTA", "Failed to build OTA response");
        return;
    }

    char buffer[2048];
    serializeJson(respDoc, buffer, sizeof(buffer));

    char entryId[32];
    if (!_redis.xadd(replyTo, "message", buffer, entryId, sizeof(entryId))) {
        LOG_ERROR("OTA", "Failed to XADD OTA response to %s", replyTo);
        return;
    }

    LOG_INFO("OTA", "OTA response sent to %s (entry=%s)", replyTo, entryId);
}
#endif

} // namespace arturo
