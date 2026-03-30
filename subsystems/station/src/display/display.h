#pragma once

#ifdef ARDUINO
#include "lvgl.h"

namespace arturo {

class Display {
public:
    bool begin();
    void loop();

    // Push state from Station — Display never reaches outside itself
    void setWifiStatus(bool connected, const char* ip, int rssi);
    void setRedisStatus(bool connected, const char* host, uint16_t port);
    void setSystemStats(uint32_t freeHeapKB, uint32_t minFreeHeapKB,
                        uint32_t freePsramKB, uint32_t uptimeSecs,
                        const char* bootReason, int watchdogResets);
    void setOpsStats(int cmdsOk, int cmdsFail,
                     int ctiTxn, int ctiErr,
                     int heartbeats,
                     int wifiReconnects, unsigned long wifiDownMs,
                     int redisReconnects);

private:
    bool _ready = false;
    unsigned long _lastUpdateMs = 0;

    // LVGL objects
    lv_obj_t* _titleLabel = nullptr;
    lv_obj_t* _statusLabel = nullptr;
    lv_obj_t* _clockLabel = nullptr;
    lv_obj_t* _systemStatsLabel = nullptr;
    lv_obj_t* _opsStatsLabel = nullptr;

    // Cached state for rendering
    bool _wifiConnected = false;
    char _wifiIp[16] = {};
    int _wifiRssi = 0;
    bool _redisConnected = false;
    char _redisHost[64] = {};
    uint16_t _redisPort = 0;

    // Cached system stats
    uint32_t _freeHeapKB = 0;
    uint32_t _minFreeHeapKB = 0;
    uint32_t _freePsramKB = 0;
    uint32_t _uptimeSecs = 0;
    char _bootReason[16] = {};
    int _watchdogResets = 0;

    // Cached ops stats
    int _cmdsOk = 0;
    int _cmdsFail = 0;
    int _ctiTxn = 0;
    int _ctiErr = 0;
    int _heartbeats = 0;
    int _wifiReconnects = 0;
    unsigned long _wifiDownMs = 0;
    int _redisReconnects = 0;

    // Last rendered text — skip redraw when unchanged
    char _lastStatusBuf[128] = {};
    char _lastSystemStatsBuf[192] = {};
    char _lastOpsStatsBuf[192] = {};

    void updateStatusLabel();
    void updateSystemStatsLabel();
    void updateOpsStatsLabel();
};

} // namespace arturo

#endif
