#include "command_handler.h"
#include "../config.h"
#include "../time_utils.h"
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
CommandHandler::CommandHandler(RedisClient& subRedis, RedisClient& pubRedis, const char* instance)
    : _subRedis(subRedis), _pubRedis(pubRedis), _instance(instance) {
    snprintf(_channelName, sizeof(_channelName), "%s%s",
             CHANNEL_COMMANDS_PREFIX, _instance);
    LOG_INFO("CMD", "Listening on channel: %s", _channelName);
}

bool CommandHandler::poll(unsigned long timeoutMs) {
    char value[2048];

    int result = _subRedis.readMessage(value, sizeof(value), timeoutMs);
    if (result <= 0) {
        return false;
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
    } else if (strcmp(type, "test.state.update") == 0) {
        handleTestStateUpdate(doc);
    } else if (strcmp(type, "system.ota.request") == 0) {
        handleOTARequest(doc);
    } else {
        LOG_ERROR("CMD", "Unknown message type: %s", type);
        _failed++;
    }
}

bool CommandHandler::dispatchToDevice(const char* deviceId, const char* commandName,
                                      char* responseBuf, size_t responseBufLen,
                                      const char*& errorCode, const char*& errorMessage) {
    // Look up the device in the registry.
    // If device_id is empty (station-scoped scripts), default to the sole device.
    const DeviceInfo* device = nullptr;
    if (deviceId == nullptr || deviceId[0] == '\0') {
        int count = 0;
        const DeviceInfo* all = getDevices(count);
        if (count == 1) {
            device = &all[0];
            LOG_INFO("CMD", "Empty device_id, defaulting to %s", device->deviceId);
        }
    } else {
        device = getDevice(deviceId);
    }

    if (device == nullptr) {
        errorCode = "device_not_found";
        errorMessage = "Device not registered on this station";
        LOG_ERROR("CMD", "Unknown device: %s", deviceId ? deviceId : "(null)");
        return false;
    }

    if (strcmp(device->protocolType, "cti") == 0) {
        if (_ctiOnBoardDevice == nullptr) {
            errorCode = "device_unavailable";
            errorMessage = "CTI OnBoard device not initialized";
            LOG_ERROR("CMD", "CTI OnBoard device not available for %s", device->deviceId);
            return false;
        }
        const char* ctiCmd = ctiOnBoardLookupCommand(commandName);
        if (ctiCmd == nullptr) {
            errorCode = "unknown_command";
            errorMessage = "Command not in CTI OnBoard command table";
            LOG_ERROR("CMD", "Unknown CTI OnBoard command: %s", commandName);
            return false;
        }
        bool success = _ctiOnBoardDevice->executeCommand(ctiCmd, responseBuf, responseBufLen);
        if (!success) {
            errorCode = "device_error";
            errorMessage = "CTI OnBoard command failed";
        }
        return success;
    }

    // Other protocols (scpi, modbus) — placeholder for future dispatch
    errorCode = "unsupported_protocol";
    errorMessage = "Protocol dispatch not yet implemented";
    LOG_ERROR("CMD", "No dispatcher for protocol: %s", device->protocolType);
    return false;
}

bool CommandHandler::executeLocal(const char* commandName, char* responseBuf, size_t responseBufLen) {
    const char* errorCode = nullptr;
    const char* errorMessage = nullptr;

    LOG_INFO("CMD", "Local command: %s", commandName);
    bool success = dispatchToDevice(nullptr, commandName, responseBuf, responseBufLen,
                                    errorCode, errorMessage);
    if (success) {
        LOG_INFO("CMD", "Local command OK: %s -> '%s'", commandName, responseBuf);
    } else {
        LOG_ERROR("CMD", "Local command failed: %s (%s)", commandName,
                  errorMessage ? errorMessage : "unknown");
    }
    return success;
}

void CommandHandler::handleTestStateUpdate(JsonDocument& doc) {
    JsonObjectConst payload = doc["payload"];
    if (payload.isNull()) {
        LOG_ERROR("CMD", "test.state.update missing payload");
        return;
    }

    const char* state = payload["state"];
    const char* testId = payload["test_id"];
    const char* testName = payload["test_name"];
    uint32_t elapsed = payload["elapsed_seconds"] | 0;

    if (state == nullptr) {
        LOG_ERROR("CMD", "test.state.update missing state field");
        return;
    }

    if (strcmp(state, "running") == 0) {
        _testState.mode = OperationalMode::TESTING;
        _testState.paused = false;
    } else if (strcmp(state, "paused") == 0) {
        _testState.mode = OperationalMode::TESTING;
        _testState.paused = true;
    } else if (strcmp(state, "completed") == 0 || strcmp(state, "aborted") == 0) {
        _testState.mode = OperationalMode::IDLE;
        _testState.paused = false;
    } else {
        LOG_ERROR("CMD", "Unknown test state: %s", state);
        return;
    }

    if (testId) strncpy(_testState.testId, testId, sizeof(_testState.testId) - 1);
    if (testName) strncpy(_testState.testName, testName, sizeof(_testState.testName) - 1);
    _testState.elapsedSecs = elapsed;
    _testState.lastUpdateMs = millis();

    LOG_INFO("CMD", "Test state: %s (id=%s name=%s elapsed=%lu)",
             state, _testState.testId, _testState.testName,
             (unsigned long)_testState.elapsedSecs);
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

    char responseBuf[256] = {0};
    const char* errorCode = nullptr;
    const char* errorMessage = nullptr;
    bool success = dispatchToDevice(req.deviceId, req.commandName,
                                    responseBuf, sizeof(responseBuf),
                                    errorCode, errorMessage);

    unsigned long durationMs = millis() - startMs;

    // Build response
    JsonDocument respDoc;
    Source src = { STATION_SERVICE, STATION_INSTANCE, STATION_VERSION };

    char respId[48];
    snprintf(respId, sizeof(respId), "resp-%s-%d", _instance, _processed);

    int64_t timestamp = arturo::getTimestamp();

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

    // PUBLISH response to reply_to channel
    if (!_pubRedis.publish(req.replyTo, buffer)) {
        LOG_ERROR("CMD", "Failed to PUBLISH response to %s", req.replyTo);
        _failed++;
        return;
    }

    _processed++;
    LOG_INFO("CMD", "Response published to %s", req.replyTo);
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
    int64_t timestamp = arturo::getTimestamp();

    if (!buildCommandResponse(respDoc, src, respId, timestamp,
                              correlationId, STATION_INSTANCE, "ota_update",
                              success, response, errorCode, errorMessage, 0)) {
        LOG_ERROR("OTA", "Failed to build OTA response");
        return;
    }

    char buffer[2048];
    serializeJson(respDoc, buffer, sizeof(buffer));

    if (!_pubRedis.publish(replyTo, buffer)) {
        LOG_ERROR("OTA", "Failed to PUBLISH OTA response to %s", replyTo);
        return;
    }

    LOG_INFO("OTA", "OTA response published to %s", replyTo);
}
#endif

} // namespace arturo
