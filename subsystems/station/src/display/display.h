#pragma once

#ifdef ARDUINO
#include "lvgl.h"
#include "../pump_telemetry.h"

namespace arturo {

// Tab indices
enum TabIndex {
    TAB_STATUS = 0,
    TAB_CHART  = 1,
    TAB_SYSTEM = 2,
    TAB_COUNT  = 3
};

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
    void setPumpTelemetry(const PumpTelemetry& telemetry);

private:
    bool _ready = false;
    unsigned long _lastUpdateMs = 0;

    // Tabview
    lv_obj_t* _tabview = nullptr;
    lv_obj_t* _tabs[TAB_COUNT] = {};

    // Banner
    lv_obj_t* _bannerCommDot = nullptr;
    lv_obj_t* _bannerTitle = nullptr;
    lv_obj_t* _bannerIp = nullptr;
    lv_obj_t* _bannerClock = nullptr;

    // Status tab widgets
    lv_obj_t* _temp1Label = nullptr;
    lv_obj_t* _temp2Label = nullptr;
    lv_obj_t* _pressureLabel = nullptr;
    lv_obj_t* _pumpStatusLabel = nullptr;
    lv_obj_t* _roughLabel = nullptr;
    lv_obj_t* _purgeLabel = nullptr;
    lv_obj_t* _regenLabel = nullptr;
    lv_obj_t* _hoursLabel = nullptr;
    lv_obj_t* _testStatusBar = nullptr;
    lv_obj_t* _testStateLabel = nullptr;
    lv_obj_t* _testInfoLabel = nullptr;

    // System tab widgets
    lv_obj_t* _sysConnLabel = nullptr;
    lv_obj_t* _sysHealthLabel = nullptr;
    lv_obj_t* _sysOpsLabel = nullptr;

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

    // Cached pump telemetry
    PumpTelemetry _pump;

    // Last rendered text — skip redraw when unchanged
    char _lastTemp1Buf[16] = {};
    char _lastTemp2Buf[16] = {};
    char _lastSysConnBuf[256] = {};
    char _lastSysHealthBuf[256] = {};
    char _lastSysOpsBuf[256] = {};

    // Tab setup helpers
    void initBanner(lv_obj_t* scr);
    void initStatusTab(lv_obj_t* parent);
    void initChartTab(lv_obj_t* parent);
    void initSystemTab(lv_obj_t* parent);

    // Tab update helpers (only called for active tab)
    void updateBanner();
    void updateStatusTab();
    void updateChartTab();
    void updateSystemTab();

    // Active tab tracking
    int activeTab() const;
};

} // namespace arturo

#endif
