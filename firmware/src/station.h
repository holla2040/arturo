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
#include <freertos/FreeRTOS.h>
#include <freertos/task.h>

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

    // Timing
    unsigned long _lastHeartbeatMs = 0;
    int _heartbeatCount = 0;

    // FreeRTOS tasks
    static void commTaskEntry(void* param);
    static void displayTaskEntry(void* param);
    void commTask();
    void displayTask();

    // Methods extracted from main.cpp
    bool connectRedis();
    bool connectRedisSub();
    bool refreshPresence();
    bool publishHeartbeat(const char* status);

    // Utility
    static void generateUUID(char* buf, size_t len);
    static int64_t getTimestamp();
    void buildPresenceKey(char* buf, size_t len);
};

} // namespace arturo

#endif
