#pragma once

#ifdef ARDUINO

#include "config.h"
#include "network/wifi_manager.h"
#include "network/redis_client.h"
#include "commands/command_handler.h"
#include "commands/device_registry.h"
#include "devices/serial_device.h"
#include "devices/cti_onboard_device.h"
#include "safety/watchdog.h"
#include "safety/ota_update.h"
#include "display/display.h"
#include "pump_telemetry.h"
#include "operational_mode.h"
#include "commands/local_command.h"
#include "screenshot_server.h"
#include <freertos/FreeRTOS.h>
#include <freertos/task.h>
#include <freertos/semphr.h>

namespace arturo {

class Station {
public:
    Station();

    bool begin();   // Full setup sequence — creates FreeRTOS tasks
    void loop();    // Idles — all work in FreeRTOS tasks

private:
    // Subsystems — owned, not global
    WifiManager _wifi;
    RedisClient _redis;
    RedisClient _redisSub;
    CommandHandler* _cmdHandler = nullptr;
    Watchdog _watchdog;
    Display _display;
    SerialDevice _ctiSerial;
    CtiOnBoardDevice _ctiDevice;
    OTAUpdateHandler _otaHandler;

    // Pump telemetry (read by displayTask, written by pumpPollTask)
    PumpTelemetry _pumpTelemetry;
    SemaphoreHandle_t _pumpTelemetryMutex = nullptr;

    // CTI device mutex — guards _ctiDevice access between commTask and pumpPollTask
    SemaphoreHandle_t _ctiMutex = nullptr;

    // Local command queue — Display enqueues, commTask drains
    QueueHandle_t _localCmdQueue = nullptr;

    // Test state (updated from Redis test.state.update messages)
    TestState _testState;
    SemaphoreHandle_t _testStateMutex = nullptr;

    // Priority poll — set by commTask after local command, read by pumpPollTask
    volatile bool _pollNow = false;

    // Timing
    unsigned long _lastHeartbeatMs = 0;
    int _heartbeatCount = 0;
    bool _ntpSynced = false;
    char _bootReasonStr[16] = {};

    // FreeRTOS tasks
    static void commTaskEntry(void* param);
    static void displayTaskEntry(void* param);
    static void pumpPollTaskEntry(void* param);
    void commTask();
    void displayTask();
    void pumpPollTask();

    // Methods extracted from main.cpp
    bool connectRedis();
    bool connectRedisSub();
    bool refreshPresence();
    bool publishHeartbeat(const char* status);

    // Utility
    static void generateUUID(char* buf, size_t len);
    void buildPresenceKey(char* buf, size_t len);
};

} // namespace arturo

#endif
