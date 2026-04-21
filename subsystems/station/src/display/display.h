#pragma once

#ifdef ARDUINO
#include "lvgl.h"
#include "../pump_telemetry.h"
#include "../operational_mode.h"
#include "../commands/local_command.h"
#include "chart_persistence.h"
#include <freertos/FreeRTOS.h>
#include <freertos/queue.h>

namespace arturo {

// Tab indices
enum TabIndex {
    TAB_STATUS   = 0,
    TAB_CHART    = 1,
    TAB_CONTROLS = 2,
    TAB_SYSTEM   = 3,
    TAB_COUNT    = 4
};

class Display {
public:
    bool begin();
    void loop();

    // Tab control
    void setActiveTab(int tab);
    int getActiveTab();
    int getTabCount() const { return TAB_COUNT; }

    // Push state from Station — Display never reaches outside itself
    void setCommandQueue(QueueHandle_t q) { _localCmdQueue = q; }
    void setTestState(const TestState& state) { _testState = state; }
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

    // Local command queue (Display enqueues, Station's commTask drains)
    QueueHandle_t _localCmdQueue = nullptr;

    // Test state
    TestState _testState;

    // Controls tab widgets — idle mode
    lv_obj_t* _idleModePanel = nullptr;
    lv_obj_t* _swPump = nullptr;
    lv_obj_t* _swRough = nullptr;
    lv_obj_t* _swPurge = nullptr;
    lv_obj_t* _lblPumpMeaning = nullptr;
    lv_obj_t* _lblRoughMeaning = nullptr;
    lv_obj_t* _lblPurgeMeaning = nullptr;
    lv_obj_t* _lblRegenStatus = nullptr;

    // Controls tab widgets — test mode
    lv_obj_t* _testModePanel = nullptr;
    lv_obj_t* _lblTestName = nullptr;
    lv_obj_t* _lblTestElapsed = nullptr;
    lv_obj_t* _btnPauseResume = nullptr;
    lv_obj_t* _lblPauseResume = nullptr;

    // Optimistic update tracking
    uint32_t _lastOptimisticMs = 0;
    bool _suppressSwitchEvents = false;

    // Touch debounce for controls tab switches — lock CLICKABLE for a window
    // after a valid tap so bounce touches never reach the widget (no flicker).
    static constexpr uint32_t SWITCH_DEBOUNCE_MS = 400;
    static constexpr int SW_PUMP = 0;
    static constexpr int SW_ROUGH = 1;
    static constexpr int SW_PURGE = 2;
    uint32_t _swLockUntilMs[3] = {0, 0, 0};

    // Chart tab
    static const int CHART_VISIBLE_POINTS = 200;
    static const int CHART_SAMPLE_INTERVAL_MS = 30000;  // 30s
    static const int CHART_SCROLL_STEP = 20;
    lv_obj_t* _chart = nullptr;
    lv_chart_series_t* _chartSeries1 = nullptr;
    lv_chart_series_t* _chartSeries2 = nullptr;
    lv_obj_t* _chartTemp1 = nullptr;    // mono font — 1st stage
    lv_obj_t* _chartTemp2 = nullptr;    // mono font — 2nd stage
    lv_obj_t* _chartStatus = nullptr;   // fixed X — pump/valve/regen
    static const int CHART_X_TICKS = 5;
    lv_obj_t* _chartXLabels[CHART_X_TICKS] = {}; // X-axis time tick labels
    unsigned long _lastChartSampleMs = 0;

    // Chart scroll
    int _chartScrollOffset = 0;          // 0 = live, >0 = scrolled back
    lv_obj_t* _chartBtnLeft = nullptr;
    lv_obj_t* _chartBtnRight = nullptr;
    bool _chartNeedsRedraw = false;

    // Chart persistence
    ChartPersistence _chartPersist;
    ChartDataPoint _chartHistory[CHART_MAX_POINTS] = {};
    int _chartWriteIndex = 0;
    int _chartHistoryCount = 0;
    int _chartSavePending = 0;

    // Confirm dialog
    lv_obj_t* _confirmOverlay = nullptr;
    const char* _confirmCmd = nullptr;

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
    void initControlsTab(lv_obj_t* parent);
    void initSystemTab(lv_obj_t* parent);

    // Tab update helpers (only called for active tab)
    void updateBanner();
    void updateStatusTab();
    void sampleChartData();
    void updateChartTab();
    void updateChartTimeLabel();
    void redrawChartFromBuffer();
    void updateControlsTab();
    void updateSystemTab();

    // Chart scroll callbacks
    static void onChartScrollLeft(lv_event_t* e);
    static void onChartScrollRight(lv_event_t* e);

    // Controls tab helpers
    void enqueueCommand(const char* commandName);
    void lockSwitch(lv_obj_t* sw, int idx);
    void updateSwitchLocks();
    static void onPumpSwitch(lv_event_t* e);
    static void onRoughSwitch(lv_event_t* e);
    static void onPurgeSwitch(lv_event_t* e);
    static void onRegenButton(lv_event_t* e);
    static void onTestActionButton(lv_event_t* e);
    static void onConfirmDialogButton(lv_event_t* e);
    void showConfirmDialog(const char* cmd);

    // Active tab tracking
    int activeTab() const;
};

} // namespace arturo

#endif
