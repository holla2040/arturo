#ifdef ARDUINO

#include "display.h"
#include "display_init.h"
#include "../config.h"
#include "../debug_log.h"
#include "../time_utils.h"
#include <Arduino.h>

// Custom 160pt numeric font (digits, -, ., space)
extern const lv_font_t font_numeric_160;
// Montserrat with fixed-width digits for clock and chart displays
extern const lv_font_t font_mono_clock_16;
extern const lv_font_t font_mono_chart_24;

namespace arturo {

// Layout constants
static const int BANNER_HEIGHT = 28;
static const int NAV_WIDTH     = 100;
static const int SCREEN_W      = 1024;
static const int SCREEN_H      = 600;
static const int CONTENT_W     = SCREEN_W - NAV_WIDTH;  // 924
static const int CONTENT_H     = SCREEN_H - BANNER_HEIGHT;  // 572

// Colors
static const lv_color_t COL_RED    = lv_color_hex(0xFF6060);
static const lv_color_t COL_BLUE   = lv_color_hex(0x6060FF);
static const lv_color_t COL_GREEN  = lv_color_hex(0x00CC00);
static const lv_color_t COL_ORANGE = lv_color_hex(0xFF9800);
static const lv_color_t COL_GRAY   = lv_color_hex(0x666666);
static const lv_color_t COL_DARK   = lv_color_hex(0x1A1A1A);

// Tab icon characters (LVGL symbols)
static const char* TAB_ICONS[] = {
    LV_SYMBOL_HOME,     // Status
    LV_SYMBOL_IMAGE,    // Chart
    LV_SYMBOL_SETTINGS, // Controls
    LV_SYMBOL_LIST,     // System
};
static const char* TAB_NAMES[] = {"Status", "Chart", "Controls", "System"};

// Map CTI regen status character to display label (from pendant2 firmware)
static const char* regenCharToLabel(char c) {
    switch (c) {
        case 'A': case '\\': return "OFF";
        case 'P': return "READY";
        case 'B': case 'C': case 'E': case '^': case ']': return "WARMUP";
        case 'H': return "PURGE";
        case 'S': return "REPURGE";
        case 'I': case 'J': case 'K': case 'T': return "ROUGHING";
        case 'L': return "ROR TEST";
        case 'M': case 'N': return "COOLING";
        case 'U': return "FAST START";
        case 'V': return "ABORTED";
        case 'W': return "DELAY";
        case 'X': case 'Y': return "PWR FAIL";
        case 'Z': return "WAIT";
        case 'O': case '[': return "ZEROING";
        case 'D': case 'F': case 'G': case 'Q': case 'R': return "GAS FAIL";
        default: return "---";
    }
}

// Is the regen character an active (non-idle) state?
static bool regenCharIsActive(char c) {
    return c != 'A' && c != '\\' && c != 'P' && c != ' ' && c != '\0';
}

// Helper: set label text only if changed
static void setLabelIfChanged(lv_obj_t* label, const char* newText, char* cacheBuf, size_t cacheLen) {
    if (strcmp(newText, cacheBuf) != 0) {
        lv_label_set_text(label, newText);
        strncpy(cacheBuf, newText, cacheLen - 1);
        cacheBuf[cacheLen - 1] = '\0';
    }
}

// --- Public ---

bool Display::begin() {
    if (!display_init()) {
        LOG_ERROR("DISPLAY", "Display init failed — continuing without display");
        return false;
    }

    display_lock(-1);

    lv_obj_t* scr = lv_scr_act();
    lv_obj_set_style_bg_color(scr, lv_color_hex(0x000000), 0);

    // Create tabview with left-side navigation
    _tabview = lv_tabview_create(scr, LV_DIR_LEFT, NAV_WIDTH);
    lv_obj_set_size(_tabview, SCREEN_W, CONTENT_H);
    lv_obj_set_pos(_tabview, 0, BANNER_HEIGHT);
    lv_obj_set_style_anim_time(_tabview, 0, 0);

    // Add 3 tabs
    for (int i = 0; i < TAB_COUNT; i++) {
        _tabs[i] = lv_tabview_add_tab(_tabview, TAB_ICONS[i]);
        lv_obj_set_style_bg_color(_tabs[i], lv_color_hex(0xFFFFFF), 0);
        lv_obj_set_style_bg_opa(_tabs[i], LV_OPA_COVER, 0);
        lv_obj_set_style_anim_time(_tabs[i], 0, 0);
        lv_obj_clear_flag(_tabs[i], LV_OBJ_FLAG_SCROLLABLE);
        lv_obj_set_style_pad_all(_tabs[i], 0, 0);
    }

    // Disable swipe between tabs
    lv_obj_clear_flag(lv_tabview_get_content(_tabview), LV_OBJ_FLAG_SCROLLABLE);

    // Content container background
    lv_obj_t* content = lv_tabview_get_content(_tabview);
    lv_obj_set_style_bg_color(content, lv_color_hex(0xFFFFFF), 0);
    lv_obj_set_style_bg_opa(content, LV_OPA_COVER, 0);
    lv_obj_set_style_anim_time(content, 0, 0);

    // Style tab buttons
    lv_obj_t* tabBtns = lv_tabview_get_tab_btns(_tabview);
    lv_obj_set_style_anim_time(tabBtns, 0, 0);
    lv_obj_set_style_text_font(tabBtns, &lv_font_montserrat_48, 0);

    // Inactive tab buttons
    lv_obj_set_style_bg_color(tabBtns, lv_color_hex(0x000000), 0);
    lv_obj_set_style_bg_color(tabBtns, lv_color_hex(0x1A1A1A), LV_PART_ITEMS);
    lv_obj_set_style_text_color(tabBtns, lv_color_hex(0x808080), LV_PART_ITEMS);
    lv_obj_set_style_border_width(tabBtns, 1, LV_PART_ITEMS);
    lv_obj_set_style_border_color(tabBtns, lv_color_hex(0x404040), LV_PART_ITEMS);
    lv_obj_set_style_border_side(tabBtns, LV_BORDER_SIDE_BOTTOM, LV_PART_ITEMS);

    // Active tab button
    lv_obj_set_style_bg_color(tabBtns, lv_color_hex(0xFFFFFF), LV_PART_ITEMS | LV_STATE_CHECKED);
    lv_obj_set_style_bg_opa(tabBtns, LV_OPA_COVER, LV_PART_ITEMS | LV_STATE_CHECKED);
    lv_obj_set_style_text_color(tabBtns, lv_color_hex(0x000000), LV_PART_ITEMS | LV_STATE_CHECKED);

    // Add text labels below icons on tab buttons
    int btnH = CONTENT_H / TAB_COUNT;  // ~190px per button
    for (int i = 0; i < TAB_COUNT; i++) {
        lv_obj_t* lbl = lv_label_create(tabBtns);
        lv_label_set_text(lbl, TAB_NAMES[i]);
        lv_obj_set_style_text_font(lbl, &lv_font_montserrat_16, 0);
        lv_obj_set_style_text_color(lbl, lv_color_hex(0x999999), 0);
        lv_obj_set_width(lbl, NAV_WIDTH);
        lv_obj_set_style_text_align(lbl, LV_TEXT_ALIGN_CENTER, 0);
        lv_obj_set_pos(lbl, 0, i * btnH + btnH - 28);
    }

    // Initialize tab content
    initBanner(scr);
    initStatusTab(_tabs[TAB_STATUS]);
    initChartTab(_tabs[TAB_CHART]);
    initControlsTab(_tabs[TAB_CONTROLS]);
    initSystemTab(_tabs[TAB_SYSTEM]);

    lv_tabview_set_act(_tabview, TAB_STATUS, LV_ANIM_OFF);

    display_unlock();
    display_start();
    _ready = true;

    LOG_INFO("DISPLAY", "Display initialized — tabview with %d tabs", TAB_COUNT);
    return true;
}

void Display::loop() {
    if (!_ready) return;
    if (!display_lock(100)) return;

    updateBanner();

    // Sample chart data regardless of active tab (avoid gaps in history)
    sampleChartData();

    // Only update the active tab (critical performance optimization)
    int tab = activeTab();
    if (tab == TAB_STATUS) updateStatusTab();
    else if (tab == TAB_CHART) updateChartTab();
    else if (tab == TAB_CONTROLS) updateControlsTab();
    else if (tab == TAB_SYSTEM) updateSystemTab();

    display_unlock();
}

int Display::activeTab() const {
    if (!_tabview) return 0;
    return lv_tabview_get_tab_act(_tabview);
}

void Display::setActiveTab(int tab) {
    if (!_tabview || tab < 0 || tab >= TAB_COUNT) return;
    if (!display_lock(100)) return;
    lv_tabview_set_act(_tabview, tab, LV_ANIM_OFF);
    display_unlock();
}

int Display::getActiveTab() {
    if (!_tabview) return 0;
    if (!display_lock(100)) return 0;
    int tab = lv_tabview_get_tab_act(_tabview);
    display_unlock();
    return tab;
}

// --- Setters (called from displayTask, no mutex needed — same thread) ---

void Display::setWifiStatus(bool connected, const char* ip, int rssi) {
    _wifiConnected = connected;
    _wifiRssi = rssi;
    if (ip) {
        strncpy(_wifiIp, ip, sizeof(_wifiIp) - 1);
        _wifiIp[sizeof(_wifiIp) - 1] = '\0';
    } else {
        _wifiIp[0] = '\0';
    }
}

void Display::setRedisStatus(bool connected, const char* host, uint16_t port) {
    _redisConnected = connected;
    if (host) {
        strncpy(_redisHost, host, sizeof(_redisHost) - 1);
        _redisHost[sizeof(_redisHost) - 1] = '\0';
    } else {
        _redisHost[0] = '\0';
    }
    _redisPort = port;
}

void Display::setPumpTelemetry(const PumpTelemetry& telemetry) {
    // During optimistic grace period, preserve user-initiated state changes
    // so they aren't overwritten by stale telemetry
    if (millis() - _lastOptimisticMs <= 2000) {
        bool savedPumpOn = _pump.pumpOn;
        bool savedRough = _pump.roughValveOpen;
        bool savedPurge = _pump.purgeValveOpen;
        char savedRegen = _pump.regenChar;
        _pump = telemetry;
        _pump.pumpOn = savedPumpOn;
        _pump.roughValveOpen = savedRough;
        _pump.purgeValveOpen = savedPurge;
        _pump.regenChar = savedRegen;
    } else {
        _pump = telemetry;
    }
}

void Display::setSystemStats(uint32_t freeHeapKB, uint32_t minFreeHeapKB,
                             uint32_t freePsramKB, uint32_t uptimeSecs,
                             const char* bootReason, int watchdogResets) {
    _freeHeapKB = freeHeapKB;
    _minFreeHeapKB = minFreeHeapKB;
    _freePsramKB = freePsramKB;
    _uptimeSecs = uptimeSecs;
    if (bootReason) {
        strncpy(_bootReason, bootReason, sizeof(_bootReason) - 1);
        _bootReason[sizeof(_bootReason) - 1] = '\0';
    }
    _watchdogResets = watchdogResets;
}

void Display::setOpsStats(int cmdsOk, int cmdsFail,
                          int ctiTxn, int ctiErr,
                          int heartbeats,
                          int wifiReconnects, unsigned long wifiDownMs,
                          int redisReconnects) {
    _cmdsOk = cmdsOk;
    _cmdsFail = cmdsFail;
    _ctiTxn = ctiTxn;
    _ctiErr = ctiErr;
    _heartbeats = heartbeats;
    _wifiReconnects = wifiReconnects;
    _wifiDownMs = wifiDownMs;
    _redisReconnects = redisReconnects;
}

// --- Banner ---

void Display::initBanner(lv_obj_t* scr) {
    lv_obj_t* banner = lv_obj_create(scr);
    lv_obj_set_size(banner, SCREEN_W, BANNER_HEIGHT);
    lv_obj_set_pos(banner, 0, 0);
    lv_obj_set_style_bg_color(banner, COL_DARK, 0);
    lv_obj_set_style_bg_opa(banner, LV_OPA_COVER, 0);
    lv_obj_set_style_border_width(banner, 0, 0);
    lv_obj_set_style_radius(banner, 0, 0);
    lv_obj_set_style_pad_all(banner, 0, 0);
    lv_obj_clear_flag(banner, LV_OBJ_FLAG_SCROLLABLE);

    // Communication indicator dot
    _bannerCommDot = lv_obj_create(banner);
    lv_obj_set_size(_bannerCommDot, 12, 12);
    lv_obj_set_pos(_bannerCommDot, 8, 8);
    lv_obj_set_style_radius(_bannerCommDot, 6, 0);
    lv_obj_set_style_bg_color(_bannerCommDot, COL_GRAY, 0);
    lv_obj_set_style_bg_opa(_bannerCommDot, LV_OPA_COVER, 0);
    lv_obj_set_style_border_width(_bannerCommDot, 0, 0);

    // Title — left side: "Arturo - station-01"
    _bannerTitle = lv_label_create(banner);
    lv_label_set_text(_bannerTitle, "Arturo - " STATION_INSTANCE);
    lv_obj_set_style_text_font(_bannerTitle, &lv_font_montserrat_16, 0);
    lv_obj_set_style_text_color(_bannerTitle, lv_color_hex(0xCCCCCC), 0);
    lv_obj_set_pos(_bannerTitle, 28, 5);

    // IP address — centered
    _bannerIp = lv_label_create(banner);
    lv_label_set_text(_bannerIp, "");
    lv_obj_set_style_text_font(_bannerIp, &lv_font_montserrat_16, 0);
    lv_obj_set_style_text_color(_bannerIp, lv_color_hex(0xCCCCCC), 0);
    lv_obj_set_style_text_align(_bannerIp, LV_TEXT_ALIGN_CENTER, 0);
    lv_obj_set_width(_bannerIp, 300);
    lv_obj_set_pos(_bannerIp, (SCREEN_W - 300) / 2, 5);

    // Clock — right side
    _bannerClock = lv_label_create(banner);
    lv_label_set_text(_bannerClock, "00:00:00");
    lv_obj_set_style_text_font(_bannerClock, &font_mono_clock_16, 0);
    lv_obj_set_style_text_color(_bannerClock, lv_color_hex(0x999999), 0);
    lv_obj_set_style_text_align(_bannerClock, LV_TEXT_ALIGN_RIGHT, 0);
    lv_obj_set_width(_bannerClock, 200);
    lv_obj_set_pos(_bannerClock, SCREEN_W - 210, 5);
}

void Display::updateBanner() {
    // Clock — local time if NTP synced, uptime fallback otherwise
    char clockBuf[48];
    if (arturo::hasValidTime()) {
        time_t now = time(nullptr);
        struct tm ti;
        localtime_r(&now, &ti);
        strftime(clockBuf, sizeof(clockBuf), "%H:%M:%S", &ti);
    } else {
        unsigned long totalSecs = millis() / 1000;
        int h = (totalSecs / 3600) % 24;
        int m = (totalSecs / 60) % 60;
        int s = totalSecs % 60;
        snprintf(clockBuf, sizeof(clockBuf), "%02d:%02d:%02d", h, m, s);
    }
    lv_label_set_text(_bannerClock, clockBuf);

    // IP address
    if (_wifiConnected && _wifiIp[0] != '\0') {
        lv_label_set_text(_bannerIp, _wifiIp);
    } else {
        lv_label_set_text(_bannerIp, "no wifi");
    }

    // Communication indicator — green if pump is fresh, red if stale
    unsigned long ms = millis();
    if (_pump.staleCount <= 2) {
        // Blink green: alternate every 500ms
        bool on = ((ms / 500) % 2) == 0;
        lv_obj_set_style_bg_color(_bannerCommDot,
            on ? COL_GREEN : lv_color_hex(0x004400), 0);
    } else {
        lv_obj_set_style_bg_color(_bannerCommDot, lv_color_hex(0xFF0000), 0);
    }
}

// --- Status Tab ---

void Display::initStatusTab(lv_obj_t* parent) {
    // Stage 1 temperature — large display
    lv_obj_t* temp1Panel = lv_obj_create(parent);
    lv_obj_set_size(temp1Panel, 555, 150);
    lv_obj_set_pos(temp1Panel, 5, 5);
    lv_obj_set_style_border_width(temp1Panel, 2, 0);
    lv_obj_set_style_border_color(temp1Panel, lv_color_hex(0xDDDDDD), 0);
    lv_obj_set_style_radius(temp1Panel, 8, 0);
    lv_obj_set_style_bg_opa(temp1Panel, LV_OPA_TRANSP, 0);
    lv_obj_set_style_pad_all(temp1Panel, 0, 0);
    lv_obj_clear_flag(temp1Panel, LV_OBJ_FLAG_SCROLLABLE);

    lv_obj_t* temp1Hdr = lv_label_create(temp1Panel);
    lv_label_set_text(temp1Hdr, "1st STAGE");
    lv_obj_set_style_text_font(temp1Hdr, &lv_font_montserrat_16, 0);
    lv_obj_set_style_text_color(temp1Hdr, COL_GRAY, 0);
    lv_obj_set_pos(temp1Hdr, 15, 10);

    _temp1Label = lv_label_create(temp1Panel);
    lv_label_set_text(_temp1Label, "--");
    lv_obj_set_style_text_font(_temp1Label, &font_numeric_160, 0);
    lv_obj_set_style_text_color(_temp1Label, COL_RED, 0);
    lv_obj_set_style_text_align(_temp1Label, LV_TEXT_ALIGN_CENTER, 0);
    lv_obj_set_width(_temp1Label, 520);
    lv_obj_set_pos(_temp1Label, 15, 15);

    // Stage 2 temperature — large display
    lv_obj_t* temp2Panel = lv_obj_create(parent);
    lv_obj_set_size(temp2Panel, 555, 150);
    lv_obj_set_pos(temp2Panel, 5, 165);
    lv_obj_set_style_border_width(temp2Panel, 2, 0);
    lv_obj_set_style_border_color(temp2Panel, lv_color_hex(0xDDDDDD), 0);
    lv_obj_set_style_radius(temp2Panel, 8, 0);
    lv_obj_set_style_bg_opa(temp2Panel, LV_OPA_TRANSP, 0);
    lv_obj_set_style_pad_all(temp2Panel, 0, 0);
    lv_obj_clear_flag(temp2Panel, LV_OBJ_FLAG_SCROLLABLE);

    lv_obj_t* temp2Hdr = lv_label_create(temp2Panel);
    lv_label_set_text(temp2Hdr, "2nd STAGE");
    lv_obj_set_style_text_font(temp2Hdr, &lv_font_montserrat_16, 0);
    lv_obj_set_style_text_color(temp2Hdr, COL_GRAY, 0);
    lv_obj_set_pos(temp2Hdr, 15, 10);

    _temp2Label = lv_label_create(temp2Panel);
    lv_label_set_text(_temp2Label, "--");
    lv_obj_set_style_text_font(_temp2Label, &font_numeric_160, 0);
    lv_obj_set_style_text_color(_temp2Label, COL_BLUE, 0);
    lv_obj_set_style_text_align(_temp2Label, LV_TEXT_ALIGN_CENTER, 0);
    lv_obj_set_width(_temp2Label, 520);
    lv_obj_set_pos(_temp2Label, 15, 15);

    // Side panel — pump status, valves, pressure, regen
    lv_obj_t* sidePanel = lv_obj_create(parent);
    lv_obj_set_size(sidePanel, 340, 310);
    lv_obj_set_pos(sidePanel, 575, 5);
    lv_obj_set_style_border_width(sidePanel, 2, 0);
    lv_obj_set_style_border_color(sidePanel, lv_color_hex(0xDDDDDD), 0);
    lv_obj_set_style_radius(sidePanel, 8, 0);
    lv_obj_set_style_bg_opa(sidePanel, LV_OPA_TRANSP, 0);
    lv_obj_set_style_pad_all(sidePanel, 0, 0);
    lv_obj_clear_flag(sidePanel, LV_OBJ_FLAG_SCROLLABLE);

    // Pressure
    lv_obj_t* pressHdr = lv_label_create(sidePanel);
    lv_label_set_text(pressHdr, "PRESSURE");
    lv_obj_set_style_text_font(pressHdr, &lv_font_montserrat_14, 0);
    lv_obj_set_style_text_color(pressHdr, COL_GRAY, 0);
    lv_obj_set_pos(pressHdr, 15, 10);

    _pressureLabel = lv_label_create(sidePanel);
    lv_label_set_text(_pressureLabel, "-- Torr");
    lv_obj_set_style_text_font(_pressureLabel, &lv_font_montserrat_24, 0);
    lv_obj_set_pos(_pressureLabel, 15, 28);

    // Status rows
    int rowY = 70;
    int rowH = 38;

    auto makeRow = [&](const char* label, lv_obj_t** valueLabel) {
        lv_obj_t* lbl = lv_label_create(sidePanel);
        lv_label_set_text(lbl, label);
        lv_obj_set_style_text_font(lbl, &lv_font_montserrat_16, 0);
        lv_obj_set_style_text_color(lbl, COL_GRAY, 0);
        lv_obj_set_pos(lbl, 15, rowY);

        *valueLabel = lv_label_create(sidePanel);
        lv_label_set_text(*valueLabel, "--");
        lv_obj_set_style_text_font(*valueLabel, &lv_font_montserrat_16, 0);
        lv_obj_set_style_text_align(*valueLabel, LV_TEXT_ALIGN_RIGHT, 0);
        lv_obj_set_width(*valueLabel, 140);
        lv_obj_set_pos(*valueLabel, 180, rowY);

        rowY += rowH;
    };

    makeRow("PUMP", &_pumpStatusLabel);
    makeRow("ROUGH", &_roughLabel);
    makeRow("PURGE", &_purgeLabel);
    makeRow("REGEN", &_regenLabel);
    makeRow("HOURS", &_hoursLabel);

    // Test status bar — full width at bottom
    _testStatusBar = lv_obj_create(parent);
    lv_obj_set_size(_testStatusBar, CONTENT_W - 20, 55);
    lv_obj_set_pos(_testStatusBar, 5, 325);
    lv_obj_set_style_bg_color(_testStatusBar, lv_color_hex(0xE8E8E8), 0);
    lv_obj_set_style_bg_opa(_testStatusBar, LV_OPA_COVER, 0);
    lv_obj_set_style_radius(_testStatusBar, 8, 0);
    lv_obj_set_style_border_width(_testStatusBar, 0, 0);
    lv_obj_set_style_pad_all(_testStatusBar, 0, 0);
    lv_obj_clear_flag(_testStatusBar, LV_OBJ_FLAG_SCROLLABLE);

    _testStateLabel = lv_label_create(_testStatusBar);
    lv_label_set_text(_testStateLabel, "No active test");
    lv_obj_set_style_text_font(_testStateLabel, &lv_font_montserrat_24, 0);
    lv_obj_set_style_text_color(_testStateLabel, COL_GRAY, 0);
    lv_obj_set_style_text_align(_testStateLabel, LV_TEXT_ALIGN_CENTER, 0);
    lv_obj_set_width(_testStateLabel, CONTENT_W - 40);
    lv_obj_set_pos(_testStateLabel, 10, 14);
}

// Forward declaration
static void updateTestStatusBar(lv_obj_t* bar, lv_obj_t* label, const TestState& state);

void Display::updateStatusTab() {
    unsigned long now = millis();
    if (now - _lastUpdateMs < 200) return;  // 5 Hz for status tab
    _lastUpdateMs = now;

    // Temperatures
    char buf[16];
    if (_pump.staleCount <= 2) {
        snprintf(buf, sizeof(buf), "%.0f", _pump.stage1TempK);
    } else {
        snprintf(buf, sizeof(buf), "--");
    }
    setLabelIfChanged(_temp1Label, buf, _lastTemp1Buf, sizeof(_lastTemp1Buf));

    if (_pump.staleCount <= 2) {
        snprintf(buf, sizeof(buf), "%.0f", _pump.stage2TempK);
    } else {
        snprintf(buf, sizeof(buf), "--");
    }
    setLabelIfChanged(_temp2Label, buf, _lastTemp2Buf, sizeof(_lastTemp2Buf));

    // Pressure
    if (_pump.staleCount <= 2) {
        snprintf(buf, sizeof(buf), "%.1e", (double)_pump.pressureTorr);
        char pressBuf[24];
        snprintf(pressBuf, sizeof(pressBuf), "%s Torr", buf);
        lv_label_set_text(_pressureLabel, pressBuf);
    } else {
        lv_label_set_text(_pressureLabel, "-- Torr");
    }

    // Pump status
    if (_pump.staleCount <= 2) {
        lv_label_set_text(_pumpStatusLabel, _pump.pumpOn ? "ON" : "OFF");
        lv_obj_set_style_text_color(_pumpStatusLabel, _pump.pumpOn ? COL_GREEN : lv_color_hex(0xFF0000), 0);

        lv_label_set_text(_roughLabel, _pump.roughValveOpen ? "OPEN" : "CLOSED");
        lv_obj_set_style_text_color(_roughLabel, _pump.roughValveOpen ? COL_GREEN : COL_GRAY, 0);

        lv_label_set_text(_purgeLabel, _pump.purgeValveOpen ? "OPEN" : "CLOSED");
        lv_obj_set_style_text_color(_purgeLabel, _pump.purgeValveOpen ? COL_GREEN : COL_GRAY, 0);

        const char* regenLabel = regenCharToLabel(_pump.regenChar);
        lv_label_set_text(_regenLabel, regenLabel);
        lv_obj_set_style_text_color(_regenLabel,
            regenCharIsActive(_pump.regenChar) ? COL_ORANGE : COL_GRAY, 0);

        snprintf(buf, sizeof(buf), "%u", _pump.operatingHours);
        lv_label_set_text(_hoursLabel, buf);
        lv_obj_set_style_text_color(_hoursLabel, lv_color_black(), 0);
    } else {
        lv_label_set_text(_pumpStatusLabel, "--");
        lv_obj_set_style_text_color(_pumpStatusLabel, COL_GRAY, 0);
        lv_label_set_text(_roughLabel, "--");
        lv_label_set_text(_purgeLabel, "--");
        lv_label_set_text(_regenLabel, "--");
        lv_label_set_text(_hoursLabel, "--");
    }

    // Update test status bar
    updateTestStatusBar(_testStatusBar, _testStateLabel, _testState);
}

// --- Chart Tab ---

void Display::initChartTab(lv_obj_t* parent) {
    // Initialize chart persistence and restore data from flash
    if (_chartPersist.init()) {
        int count = 0, writeIdx = 0;
        if (_chartPersist.load(_chartHistory, CHART_MAX_POINTS, count, writeIdx)) {
            _chartHistoryCount = count;
            _chartWriteIndex = writeIdx;
            LOG_INFO("DISPLAY", "Restored %d chart points from flash", count);
        }
    }

    // Chart positioned with left margin for manual Y-axis labels
    static const int CHART_LEFT = 50;
    static const int CHART_TOP = 40;
    static const int CHART_BOTTOM_PANEL = 40;  // space for time label (and later scroll buttons)
    static const int CHART_W = CONTENT_W - CHART_LEFT - 15;
    static const int CHART_H = CONTENT_H - CHART_TOP - CHART_BOTTOM_PANEL;

    _chart = lv_chart_create(parent);
    lv_obj_set_size(_chart, CHART_W, CHART_H);
    lv_obj_set_pos(_chart, CHART_LEFT, CHART_TOP);
    lv_chart_set_type(_chart, LV_CHART_TYPE_LINE);
    lv_chart_set_point_count(_chart, CHART_VISIBLE_POINTS);
    lv_chart_set_range(_chart, LV_CHART_AXIS_PRIMARY_Y, 0, 320);
    lv_chart_set_update_mode(_chart, LV_CHART_UPDATE_MODE_SHIFT);

    // Grid lines — 8 divisions = 40K increments (0, 40, 80, ... 320)
    lv_chart_set_div_line_count(_chart, 8, CHART_X_TICKS - 2);
    lv_obj_set_style_line_color(_chart, lv_color_hex(0xD0D0D0), LV_PART_MAIN);
    lv_obj_set_style_line_width(_chart, 1, LV_PART_MAIN);

    // Style — sharp 90-degree corners, no rounding
    lv_obj_set_style_bg_color(_chart, lv_color_hex(0xFFFFFF), 0);
    lv_obj_set_style_bg_opa(_chart, LV_OPA_COVER, 0);
    lv_obj_set_style_border_width(_chart, 1, 0);
    lv_obj_set_style_border_color(_chart, lv_color_hex(0xBBBBBB), 0);
    lv_obj_set_style_radius(_chart, 0, 0);
    lv_obj_set_style_size(_chart, 0, LV_PART_INDICATOR);
    lv_obj_set_style_pad_all(_chart, 0, 0);

    // Series — bright red and blue, 2px line width
    _chartSeries1 = lv_chart_add_series(_chart, lv_color_hex(0xFF0000), LV_CHART_AXIS_PRIMARY_Y);
    _chartSeries2 = lv_chart_add_series(_chart, lv_color_hex(0x0000FF), LV_CHART_AXIS_PRIMARY_Y);
    lv_obj_set_style_line_width(_chart, 2, LV_PART_ITEMS);

    // Manual Y-axis labels (pendant2 pattern — LVGL 8.4 tick labels unreliable)
    // Plot area height = CHART_H (no padding)
    int plotH = CHART_H;
    const int yAxisValues[] = {0, 40, 80, 120, 160, 200, 240, 280, 320};
    for (int i = 0; i < 9; i++) {
        int v = yAxisValues[i];
        // Y position: top of chart + (320-v)/320 * plotH
        int yPos = CHART_TOP + (320 - v) * plotH / 320 - 7;  // -7 to center text on line

        lv_obj_t* lbl = lv_label_create(parent);
        lv_label_set_text_fmt(lbl, "%d", v);
        lv_obj_set_style_text_font(lbl, &lv_font_montserrat_14, 0);
        lv_obj_set_style_text_color(lbl, lv_color_hex(0x666666), 0);
        lv_obj_set_style_text_align(lbl, LV_TEXT_ALIGN_RIGHT, 0);
        lv_obj_set_width(lbl, 42);
        lv_obj_set_pos(lbl, 2, yPos);
    }

    // Temp labels — mono font, colored to match chart series
    _chartTemp1 = lv_label_create(parent);
    lv_label_set_text(_chartTemp1, "---");
    lv_obj_set_style_text_font(_chartTemp1, &lv_font_montserrat_24, 0);
    lv_obj_set_style_text_color(_chartTemp1, lv_color_hex(0xFF0000), 0);
    lv_obj_set_pos(_chartTemp1, 17, 5);

    _chartTemp2 = lv_label_create(parent);
    lv_label_set_text(_chartTemp2, "---");
    lv_obj_set_style_text_font(_chartTemp2, &lv_font_montserrat_24, 0);
    lv_obj_set_style_text_color(_chartTemp2, lv_color_hex(0x0000FF), 0);
    lv_obj_set_pos(_chartTemp2, 80, 5);

    // Status label — Montserrat, fixed X position
    _chartStatus = lv_label_create(parent);
    lv_label_set_text(_chartStatus, "");
    lv_obj_set_style_text_font(_chartStatus, &lv_font_montserrat_24, 0);
    lv_obj_set_pos(_chartStatus, CHART_LEFT + 110, 5);
    lv_label_set_recolor(_chartStatus, true);

    // X-axis time tick labels below chart
    for (int i = 0; i < CHART_X_TICKS; i++) {
        _chartXLabels[i] = lv_label_create(parent);
        lv_label_set_text(_chartXLabels[i], "");
        lv_obj_set_style_text_font(_chartXLabels[i], &lv_font_montserrat_14, 0);
        lv_obj_set_style_text_color(_chartXLabels[i], lv_color_hex(0x666666), 0);
        lv_obj_set_style_text_align(_chartXLabels[i], LV_TEXT_ALIGN_CENTER, 0);
        lv_obj_set_width(_chartXLabels[i], 60);
        // Evenly spaced across chart width
        int xPos = CHART_LEFT + i * CHART_W / (CHART_X_TICKS - 1) - 30;
        lv_obj_set_pos(_chartXLabels[i], xPos, CHART_TOP + CHART_H + 2);
    }

    // Scroll arrow buttons — tiny arrows, 50x50 touch area, bottom corners
    _chartBtnLeft = lv_btn_create(parent);
    lv_obj_set_size(_chartBtnLeft, 50, 50);
    lv_obj_set_pos(_chartBtnLeft, 0, CONTENT_H - 43);
    lv_obj_set_style_bg_opa(_chartBtnLeft, LV_OPA_TRANSP, 0);
    lv_obj_set_style_shadow_width(_chartBtnLeft, 0, 0);
    lv_obj_set_style_border_width(_chartBtnLeft, 0, 0);
    lv_obj_t* lblLeft = lv_label_create(_chartBtnLeft);
    lv_label_set_text(lblLeft, LV_SYMBOL_LEFT);
    lv_obj_set_style_text_font(lblLeft, &lv_font_montserrat_14, 0);
    lv_obj_set_style_text_color(lblLeft, lv_color_hex(0x888888), 0);
    lv_obj_center(lblLeft);
    lv_obj_add_event_cb(_chartBtnLeft, onChartScrollLeft, LV_EVENT_CLICKED, this);

    _chartBtnRight = lv_btn_create(parent);
    lv_obj_set_size(_chartBtnRight, 50, 50);
    lv_obj_set_pos(_chartBtnRight, CONTENT_W - 50, CONTENT_H - 43);
    lv_obj_set_style_bg_opa(_chartBtnRight, LV_OPA_TRANSP, 0);
    lv_obj_set_style_shadow_width(_chartBtnRight, 0, 0);
    lv_obj_set_style_border_width(_chartBtnRight, 0, 0);
    lv_obj_t* lblRight = lv_label_create(_chartBtnRight);
    lv_label_set_text(lblRight, LV_SYMBOL_RIGHT);
    lv_obj_set_style_text_font(lblRight, &lv_font_montserrat_14, 0);
    lv_obj_set_style_text_color(lblRight, lv_color_hex(0x888888), 0);
    lv_obj_center(lblRight);
    lv_obj_add_event_cb(_chartBtnRight, onChartScrollRight, LV_EVENT_CLICKED, this);

    // Populate chart from persisted data
    _chartNeedsRedraw = true;
}

void Display::sampleChartData() {
    unsigned long now = millis();

    // Sample every 30 seconds if pump data is fresh
    if (now - _lastChartSampleMs >= CHART_SAMPLE_INTERVAL_MS && _pump.staleCount <= 2) {
        _lastChartSampleMs = now;

        // Store point in buffer (epoch seconds)
        _chartHistory[_chartWriteIndex].timestamp = (uint32_t)time(nullptr);
        _chartHistory[_chartWriteIndex].stage1TempK = _pump.stage1TempK;
        _chartHistory[_chartWriteIndex].stage2TempK = _pump.stage2TempK;
        _chartWriteIndex = (_chartWriteIndex + 1) % CHART_MAX_POINTS;
        if (_chartHistoryCount < CHART_MAX_POINTS) _chartHistoryCount++;

        // Redraw chart if at live view
        if (_chartScrollOffset == 0) _chartNeedsRedraw = true;

        // Save to flash every 10 samples (~5 minutes)
        _chartSavePending++;
        if (_chartSavePending >= 10) {
            _chartPersist.save(_chartHistory, _chartHistoryCount, _chartWriteIndex);
            _chartSavePending = 0;
        }
    }
}

void Display::redrawChartFromBuffer() {
    _chartNeedsRedraw = false;

    int pointsToShow = _chartHistoryCount < CHART_VISIBLE_POINTS
                       ? _chartHistoryCount : CHART_VISIBLE_POINTS;
    if (pointsToShow == 0) return;

    // Newest point in buffer
    int newestBuf = (_chartWriteIndex - 1 + CHART_MAX_POINTS) % CHART_MAX_POINTS;
    // Newest visible point (offset back from newest)
    int newestVis = (newestBuf - _chartScrollOffset + CHART_MAX_POINTS) % CHART_MAX_POINTS;

    // Fill LVGL chart — index 0 = oldest visible, index N-1 = newest visible
    for (int i = 0; i < CHART_VISIBLE_POINTS; i++) {
        if (i < pointsToShow) {
            int bufIdx = (newestVis - pointsToShow + 1 + i + CHART_MAX_POINTS) % CHART_MAX_POINTS;
            lv_chart_set_value_by_id(_chart, _chartSeries1, i,
                (lv_coord_t)(_chartHistory[bufIdx].stage1TempK + 0.5f));
            lv_chart_set_value_by_id(_chart, _chartSeries2, i,
                (lv_coord_t)(_chartHistory[bufIdx].stage2TempK + 0.5f));
        } else {
            lv_chart_set_value_by_id(_chart, _chartSeries1, i, LV_CHART_POINT_NONE);
            lv_chart_set_value_by_id(_chart, _chartSeries2, i, LV_CHART_POINT_NONE);
        }
    }
    lv_chart_refresh(_chart);
}

void Display::onChartScrollLeft(lv_event_t* e) {
    Display* self = static_cast<Display*>(lv_event_get_user_data(e));
    int maxOffset = self->_chartHistoryCount > CHART_VISIBLE_POINTS
                    ? self->_chartHistoryCount - CHART_VISIBLE_POINTS : 0;
    self->_chartScrollOffset += CHART_SCROLL_STEP;
    if (self->_chartScrollOffset > maxOffset) self->_chartScrollOffset = maxOffset;
    self->_chartNeedsRedraw = true;
}

void Display::onChartScrollRight(lv_event_t* e) {
    Display* self = static_cast<Display*>(lv_event_get_user_data(e));
    self->_chartScrollOffset -= CHART_SCROLL_STEP;
    if (self->_chartScrollOffset < 0) self->_chartScrollOffset = 0;
    self->_chartNeedsRedraw = true;
}

void Display::updateChartTab() {
    if (_pump.staleCount <= 2) {
        // Temps — mono font, stable width
        lv_label_set_text_fmt(_chartTemp1, "1:%d", (int)(_pump.stage1TempK + 0.5f));
        lv_label_set_text_fmt(_chartTemp2, "2:%d", (int)(_pump.stage2TempK + 0.5f));

        // Status — fixed X position
        const char* regenLbl = regenCharToLabel(_pump.regenChar);
        bool regenActive = regenCharIsActive(_pump.regenChar);
        char statusBuf[128];
        snprintf(statusBuf, sizeof(statusBuf),
            "Motor: %s  Rough: %s  Purge: %s  Regen: %s%s%s",
            _pump.pumpOn ? "#22c55e ON#" : "#ef4444 OFF#",
            _pump.roughValveOpen ? "#ef4444 OPEN#" : "#22c55e CLOSED#",
            _pump.purgeValveOpen ? "#ef4444 OPEN#" : "#22c55e CLOSED#",
            regenActive ? "#FF9800 " : "",
            regenLbl,
            regenActive ? "#" : "");
        lv_label_set_text(_chartStatus, statusBuf);
    }

    if (_chartNeedsRedraw) redrawChartFromBuffer();
    updateChartTimeLabel();

    // Disable scroll buttons at edges
    int maxOffset = _chartHistoryCount > CHART_VISIBLE_POINTS
                    ? _chartHistoryCount - CHART_VISIBLE_POINTS : 0;
    if (_chartScrollOffset >= maxOffset)
        lv_obj_add_state(_chartBtnLeft, LV_STATE_DISABLED);
    else
        lv_obj_clear_state(_chartBtnLeft, LV_STATE_DISABLED);

    if (_chartScrollOffset <= 0)
        lv_obj_add_state(_chartBtnRight, LV_STATE_DISABLED);
    else
        lv_obj_clear_state(_chartBtnRight, LV_STATE_DISABLED);
}

void Display::updateChartTimeLabel() {
    int pointsToShow = _chartHistoryCount < CHART_VISIBLE_POINTS
                       ? _chartHistoryCount : CHART_VISIBLE_POINTS;
    if (pointsToShow < 2) {
        for (int i = 0; i < CHART_X_TICKS; i++)
            lv_label_set_text(_chartXLabels[i], "");
        return;
    }

    int newestBuf = (_chartWriteIndex - 1 + CHART_MAX_POINTS) % CHART_MAX_POINTS;
    int newestVis = (newestBuf - _chartScrollOffset + CHART_MAX_POINTS) % CHART_MAX_POINTS;
    int oldestVis = (newestVis - pointsToShow + 1 + CHART_MAX_POINTS) % CHART_MAX_POINTS;

    uint32_t oldestEpoch = _chartHistory[oldestVis].timestamp;
    uint32_t newestEpoch = _chartHistory[newestVis].timestamp;

    for (int i = 0; i < CHART_X_TICKS; i++) {
        time_t tickEpoch = oldestEpoch + (newestEpoch - oldestEpoch) * i / (CHART_X_TICKS - 1);
        struct tm ti;
        localtime_r(&tickEpoch, &ti);
        char buf[16];
        strftime(buf, sizeof(buf), "%H:%M", &ti);
        lv_label_set_text(_chartXLabels[i], buf);
    }
}

// --- Controls Tab ---

void Display::enqueueCommand(const char* commandName) {
    if (!_localCmdQueue) return;
    LocalCommand cmd = {};
    strncpy(cmd.commandName, commandName, sizeof(cmd.commandName) - 1);
    // deviceId left empty — defaults to sole device
    xQueueSend(_localCmdQueue, &cmd, pdMS_TO_TICKS(10));
    _lastOptimisticMs = millis();
}

void Display::onPumpSwitch(lv_event_t* e) {
    if (lv_event_get_code(e) != LV_EVENT_VALUE_CHANGED) return;
    Display* self = static_cast<Display*>(lv_event_get_user_data(e));
    lv_obj_t* sw = lv_event_get_target(e);
    bool is_on = lv_obj_has_state(sw, LV_STATE_CHECKED);

    // Optimistic update
    self->_pump.pumpOn = is_on;
    self->enqueueCommand(is_on ? "pump_on" : "pump_off");
}

void Display::onRoughSwitch(lv_event_t* e) {
    if (lv_event_get_code(e) != LV_EVENT_VALUE_CHANGED) return;
    Display* self = static_cast<Display*>(lv_event_get_user_data(e));
    lv_obj_t* sw = lv_event_get_target(e);
    bool is_on = lv_obj_has_state(sw, LV_STATE_CHECKED);

    self->_pump.roughValveOpen = is_on;
    self->enqueueCommand(is_on ? "open_rough_valve" : "close_rough_valve");
}

void Display::onPurgeSwitch(lv_event_t* e) {
    if (lv_event_get_code(e) != LV_EVENT_VALUE_CHANGED) return;
    Display* self = static_cast<Display*>(lv_event_get_user_data(e));
    lv_obj_t* sw = lv_event_get_target(e);
    bool is_on = lv_obj_has_state(sw, LV_STATE_CHECKED);

    self->_pump.purgeValveOpen = is_on;
    self->enqueueCommand(is_on ? "open_purge_valve" : "close_purge_valve");
}

void Display::onRegenButton(lv_event_t* e) {
    if (lv_event_get_code(e) != LV_EVENT_CLICKED) return;
    Display* self = static_cast<Display*>(lv_event_get_user_data(e));
    const char* cmd = static_cast<const char*>(lv_obj_get_user_data(lv_event_get_target(e)));
    if (!cmd) return;

    // Optimistic update — show regen state change immediately
    if (strcmp(cmd, "start_regen") == 0) {
        self->_pump.regenChar = 'B';  // WARMUP
    } else if (strcmp(cmd, "start_fast_regen") == 0) {
        self->_pump.regenChar = 'U';  // FAST START
    } else if (strcmp(cmd, "abort_regen") == 0) {
        self->_pump.regenChar = 'A';  // OFF
    }

    self->enqueueCommand(cmd);
}

void Display::onTestActionButton(lv_event_t* e) {
    if (lv_event_get_code(e) != LV_EVENT_CLICKED) return;
    Display* self = static_cast<Display*>(lv_event_get_user_data(e));
    const char* cmd = static_cast<const char*>(lv_obj_get_user_data(lv_event_get_target(e)));
    if (cmd) self->enqueueCommand(cmd);
}

// Helper: create a control switch with labels
static lv_obj_t* createControlSwitch(lv_obj_t* parent, int x, int y,
                                      lv_color_t onColor, const char* funcLabel,
                                      lv_obj_t** meaningLabel,
                                      const char* defaultMeaning,
                                      lv_event_cb_t cb, void* userData) {
    lv_obj_t* sw = lv_switch_create(parent);
    lv_obj_set_size(sw, 80, 40);
    lv_obj_set_pos(sw, x, y);
    lv_obj_add_event_cb(sw, cb, LV_EVENT_VALUE_CHANGED, userData);
    lv_obj_set_style_bg_color(sw, onColor, LV_PART_INDICATOR | LV_STATE_CHECKED);
    lv_obj_set_style_anim_time(sw, 0, LV_PART_MAIN);

    // Meaning label above switch
    *meaningLabel = lv_label_create(parent);
    lv_label_set_text(*meaningLabel, defaultMeaning);
    lv_obj_set_width(*meaningLabel, 150);
    lv_obj_set_style_text_font(*meaningLabel, &lv_font_montserrat_24, 0);
    lv_obj_set_style_text_align(*meaningLabel, LV_TEXT_ALIGN_CENTER, 0);
    lv_obj_set_style_pad_all(*meaningLabel, 0, 0);
    lv_obj_align_to(*meaningLabel, sw, LV_ALIGN_OUT_TOP_MID, 0, -2);

    // Function label below switch
    lv_obj_t* lbl = lv_label_create(parent);
    lv_label_set_text(lbl, funcLabel);
    lv_obj_set_style_text_font(lbl, &lv_font_montserrat_24, 0);
    lv_obj_set_style_text_align(lbl, LV_TEXT_ALIGN_CENTER, 0);
    lv_obj_set_style_pad_all(lbl, 0, 0);
    lv_obj_align_to(lbl, sw, LV_ALIGN_OUT_BOTTOM_MID, 0, 5);

    return sw;
}

// Helper: create a regen button
static lv_obj_t* createRegenButton(lv_obj_t* parent, int x, int y,
                                    lv_color_t bgColor, const char* label,
                                    const char* cmdName,
                                    lv_event_cb_t cb, void* userData) {
    lv_obj_t* btn = lv_btn_create(parent);
    lv_obj_set_size(btn, 150, 60);
    lv_obj_set_pos(btn, x, y);
    lv_obj_set_style_bg_color(btn, bgColor, 0);
    lv_obj_set_user_data(btn, (void*)cmdName);
    lv_obj_add_event_cb(btn, cb, LV_EVENT_CLICKED, userData);

    lv_obj_t* lbl = lv_label_create(btn);
    lv_label_set_text(lbl, label);
    lv_obj_set_style_text_font(lbl, &lv_font_montserrat_24, 0);
    lv_obj_center(lbl);

    return btn;
}

// Helper: create a test action button
static lv_obj_t* createTestActionButton(lv_obj_t* parent, int x, int y,
                                         lv_color_t bgColor, const char* label,
                                         const char* cmdName,
                                         lv_event_cb_t cb, void* userData) {
    lv_obj_t* btn = lv_btn_create(parent);
    lv_obj_set_size(btn, 200, 80);
    lv_obj_set_pos(btn, x, y);
    lv_obj_set_style_bg_color(btn, bgColor, 0);
    lv_obj_set_user_data(btn, (void*)cmdName);
    lv_obj_add_event_cb(btn, cb, LV_EVENT_CLICKED, userData);

    lv_obj_t* lbl = lv_label_create(btn);
    lv_label_set_text(lbl, label);
    lv_obj_set_style_text_font(lbl, &lv_font_montserrat_32, 0);
    lv_obj_center(lbl);

    return btn;
}

void Display::initControlsTab(lv_obj_t* parent) {
    // === IDLE MODE PANEL (visible when not testing) ===
    _idleModePanel = lv_obj_create(parent);
    lv_obj_set_size(_idleModePanel, CONTENT_W, CONTENT_H);
    lv_obj_set_pos(_idleModePanel, 0, 0);
    lv_obj_set_style_bg_opa(_idleModePanel, LV_OPA_TRANSP, 0);
    lv_obj_set_style_border_width(_idleModePanel, 0, 0);
    lv_obj_set_style_pad_all(_idleModePanel, 0, 0);
    lv_obj_clear_flag(_idleModePanel, LV_OBJ_FLAG_SCROLLABLE);

    // Mode indicator bar
    lv_obj_t* modeBar = lv_obj_create(_idleModePanel);
    lv_obj_set_size(modeBar, CONTENT_W - 20, 40);
    lv_obj_set_pos(modeBar, 5, 5);
    lv_obj_set_style_bg_color(modeBar, lv_color_hex(0x006600), 0);
    lv_obj_set_style_bg_opa(modeBar, LV_OPA_COVER, 0);
    lv_obj_set_style_radius(modeBar, 8, 0);
    lv_obj_set_style_border_width(modeBar, 0, 0);
    lv_obj_clear_flag(modeBar, LV_OBJ_FLAG_SCROLLABLE);

    lv_obj_t* modeLbl = lv_label_create(modeBar);
    lv_label_set_text(modeLbl, "MANUAL CONTROL");
    lv_obj_set_style_text_font(modeLbl, &lv_font_montserrat_24, 0);
    lv_obj_set_style_text_color(modeLbl, lv_color_white(), 0);
    lv_obj_center(modeLbl);

    // Control switches — 3 switches equally spaced across width
    // Content width = 924, 3 segments = 308px each, switch at center of each
    int switchY = 120;  // vertical center for switches
    int seg = (CONTENT_W - 20) / 3;  // ~301px per segment

    _swPump = createControlSwitch(_idleModePanel, 5 + seg * 0 + seg / 2 - 40, switchY,
                                   lv_color_hex(0x00AA00), "PUMP\nPOWER", &_lblPumpMeaning,
                                   "OFF", onPumpSwitch, this);

    _swRough = createControlSwitch(_idleModePanel, 5 + seg * 1 + seg / 2 - 40, switchY,
                                    lv_color_hex(0x0066AA), "ROUGH\nVALVE", &_lblRoughMeaning,
                                    "CLOSED", onRoughSwitch, this);

    _swPurge = createControlSwitch(_idleModePanel, 5 + seg * 2 + seg / 2 - 40, switchY,
                                    lv_color_hex(0x0066AA), "PURGE\nVALVE", &_lblPurgeMeaning,
                                    "CLOSED", onPurgeSwitch, this);

    // Regen section — label + 3 buttons
    lv_obj_t* regenLbl = lv_label_create(_idleModePanel);
    lv_label_set_text(regenLbl, "REGENERATION");
    lv_obj_set_style_text_font(regenLbl, &lv_font_montserrat_24, 0);
    lv_obj_set_style_text_color(regenLbl, lv_color_black(), 0);
    lv_obj_set_pos(regenLbl, 20, 290);

    createRegenButton(_idleModePanel, 20, 330,
                       lv_color_hex(0x00AA00), "FULL", "start_regen",
                       onRegenButton, this);
    createRegenButton(_idleModePanel, 185, 330,
                       lv_color_hex(0x0066AA), "FAST", "start_fast_regen",
                       onRegenButton, this);
    createRegenButton(_idleModePanel, 350, 330,
                       lv_color_hex(0xCC0000), "CANCEL", "abort_regen",
                       onRegenButton, this);

    // Regen status label
    _lblRegenStatus = lv_label_create(_idleModePanel);
    lv_label_set_text(_lblRegenStatus, "STATUS: OFF");
    lv_obj_set_style_text_font(_lblRegenStatus, &lv_font_montserrat_24, 0);
    lv_obj_set_style_text_color(_lblRegenStatus, COL_GRAY, 0);
    lv_obj_set_pos(_lblRegenStatus, 550, 350);

    // === TEST MODE PANEL (visible when test in progress) ===
    _testModePanel = lv_obj_create(parent);
    lv_obj_set_size(_testModePanel, CONTENT_W, CONTENT_H);
    lv_obj_set_pos(_testModePanel, 0, 0);
    lv_obj_set_style_bg_opa(_testModePanel, LV_OPA_TRANSP, 0);
    lv_obj_set_style_border_width(_testModePanel, 0, 0);
    lv_obj_set_style_pad_all(_testModePanel, 0, 0);
    lv_obj_clear_flag(_testModePanel, LV_OBJ_FLAG_SCROLLABLE);

    // Alert banner
    lv_obj_t* alertBar = lv_obj_create(_testModePanel);
    lv_obj_set_size(alertBar, CONTENT_W - 20, 60);
    lv_obj_set_pos(alertBar, 5, 5);
    lv_obj_set_style_bg_color(alertBar, lv_color_hex(0xFF9800), 0);
    lv_obj_set_style_bg_opa(alertBar, LV_OPA_COVER, 0);
    lv_obj_set_style_radius(alertBar, 8, 0);
    lv_obj_set_style_border_width(alertBar, 0, 0);
    lv_obj_clear_flag(alertBar, LV_OBJ_FLAG_SCROLLABLE);

    _lblTestName = lv_label_create(alertBar);
    lv_label_set_text(_lblTestName, "TEST IN PROGRESS");
    lv_obj_set_style_text_font(_lblTestName, &lv_font_montserrat_24, 0);
    lv_obj_set_style_text_color(_lblTestName, lv_color_white(), 0);
    lv_obj_center(_lblTestName);

    // Elapsed time display
    _lblTestElapsed = lv_label_create(_testModePanel);
    lv_label_set_text(_lblTestElapsed, "Elapsed: 0:00:00");
    lv_obj_set_style_text_font(_lblTestElapsed, &lv_font_montserrat_32, 0);
    lv_obj_set_style_text_color(_lblTestElapsed, lv_color_black(), 0);
    lv_obj_set_style_text_align(_lblTestElapsed, LV_TEXT_ALIGN_CENTER, 0);
    lv_obj_set_width(_lblTestElapsed, CONTENT_W);
    lv_obj_set_pos(_lblTestElapsed, 0, 140);

    // Test action buttons — centered row
    int btnY = 300;
    int btnSpacing = 230;
    int btnStartX = (CONTENT_W - 3 * 200 - 2 * 30) / 2;  // center 3 buttons with 30px gaps

    createTestActionButton(_testModePanel, btnStartX, btnY,
                            lv_color_hex(0xCC0000), "ABORT", "test_abort",
                            onTestActionButton, this);
    createTestActionButton(_testModePanel, btnStartX + btnSpacing, btnY,
                            lv_color_hex(0xCC9900), "PAUSE", "test_pause",
                            onTestActionButton, this);
    createTestActionButton(_testModePanel, btnStartX + btnSpacing * 2, btnY,
                            lv_color_hex(0x00AA00), "CONTINUE", "test_continue",
                            onTestActionButton, this);

    // Start in idle mode — hide test panel
    lv_obj_add_flag(_testModePanel, LV_OBJ_FLAG_HIDDEN);
}

void Display::updateControlsTab() {
    // Toggle panel visibility based on mode
    if (_testState.mode == OperationalMode::TESTING) {
        if (!lv_obj_has_flag(_testModePanel, LV_OBJ_FLAG_HIDDEN)) {
            // Already visible — update content
        } else {
            lv_obj_clear_flag(_testModePanel, LV_OBJ_FLAG_HIDDEN);
            lv_obj_add_flag(_idleModePanel, LV_OBJ_FLAG_HIDDEN);
        }

        // Update test info
        if (_testState.testName[0] != '\0') {
            char buf[96];
            snprintf(buf, sizeof(buf), "TEST IN PROGRESS: %s", _testState.testName);
            lv_label_set_text(_lblTestName, buf);
        }

        uint32_t s = _testState.elapsedSecs;
        int h = s / 3600;
        int m = (s % 3600) / 60;
        int sec = s % 60;
        char timeBuf[32];
        snprintf(timeBuf, sizeof(timeBuf), "Elapsed: %d:%02d:%02d", h, m, sec);
        lv_label_set_text(_lblTestElapsed, timeBuf);
    } else {
        if (!lv_obj_has_flag(_idleModePanel, LV_OBJ_FLAG_HIDDEN)) {
            // Already visible
        } else {
            lv_obj_clear_flag(_idleModePanel, LV_OBJ_FLAG_HIDDEN);
            lv_obj_add_flag(_testModePanel, LV_OBJ_FLAG_HIDDEN);
        }

        // Sync switch states from telemetry (skip if optimistic update is recent)
        if (millis() - _lastOptimisticMs > 2000) {
            // Pump switch
            bool swPumpOn = lv_obj_has_state(_swPump, LV_STATE_CHECKED);
            if (_pump.pumpOn != swPumpOn) {
                if (_pump.pumpOn) lv_obj_add_state(_swPump, LV_STATE_CHECKED);
                else lv_obj_clear_state(_swPump, LV_STATE_CHECKED);
            }

            // Rough valve switch
            bool swRoughOn = lv_obj_has_state(_swRough, LV_STATE_CHECKED);
            if (_pump.roughValveOpen != swRoughOn) {
                if (_pump.roughValveOpen) lv_obj_add_state(_swRough, LV_STATE_CHECKED);
                else lv_obj_clear_state(_swRough, LV_STATE_CHECKED);
            }

            // Purge valve switch
            bool swPurgeOn = lv_obj_has_state(_swPurge, LV_STATE_CHECKED);
            if (_pump.purgeValveOpen != swPurgeOn) {
                if (_pump.purgeValveOpen) lv_obj_add_state(_swPurge, LV_STATE_CHECKED);
                else lv_obj_clear_state(_swPurge, LV_STATE_CHECKED);
            }
        }

        // Update meaning labels
        lv_label_set_text(_lblPumpMeaning, _pump.pumpOn ? "ON" : "OFF");
        lv_label_set_text(_lblRoughMeaning, _pump.roughValveOpen ? "OPEN" : "CLOSED");
        lv_label_set_text(_lblPurgeMeaning, _pump.purgeValveOpen ? "OPEN" : "CLOSED");

        // Regen status
        const char* regenLabel = regenCharToLabel(_pump.regenChar);
        char regenBuf[32];
        snprintf(regenBuf, sizeof(regenBuf), "STATUS: %s", regenLabel);
        lv_label_set_text(_lblRegenStatus, regenBuf);
        lv_obj_set_style_text_color(_lblRegenStatus,
            regenCharIsActive(_pump.regenChar) ? COL_ORANGE : COL_GRAY, 0);
    }
}

// --- Test status bar update on Status Tab ---

// Update the test status bar appearance on the Status tab based on test state.
// Called from updateStatusTab() — reflects test mode even on the telemetry view.
static void updateTestStatusBar(lv_obj_t* bar, lv_obj_t* label,
                                 const TestState& state) {
    if (state.mode == OperationalMode::TESTING) {
        if (state.paused) {
            lv_obj_set_style_bg_color(bar, lv_color_hex(0xFFEB3B), 0);  // Yellow
            char buf[96];
            snprintf(buf, sizeof(buf), "PAUSED: %s", state.testName);
            lv_label_set_text(label, buf);
            lv_obj_set_style_text_color(label, lv_color_black(), 0);
        } else {
            lv_obj_set_style_bg_color(bar, lv_color_hex(0xFF9800), 0);  // Amber
            uint32_t s = state.elapsedSecs;
            char buf[96];
            snprintf(buf, sizeof(buf), "TEST: %s | %lu:%02lu:%02lu",
                     state.testName,
                     (unsigned long)(s / 3600),
                     (unsigned long)((s % 3600) / 60),
                     (unsigned long)(s % 60));
            lv_label_set_text(label, buf);
            lv_obj_set_style_text_color(label, lv_color_white(), 0);
        }
    } else {
        lv_obj_set_style_bg_color(bar, lv_color_hex(0xE8E8E8), 0);  // Gray
        lv_label_set_text(label, "No active test");
        lv_obj_set_style_text_color(label, COL_GRAY, 0);
    }
}

// --- System Tab ---

void Display::initSystemTab(lv_obj_t* parent) {
    // Left column — connectivity + system health
    _sysConnLabel = lv_label_create(parent);
    lv_label_set_text(_sysConnLabel, "CONNECTIVITY\n...");
    lv_obj_set_style_text_font(_sysConnLabel, &lv_font_montserrat_16, 0);
    lv_obj_set_style_text_color(_sysConnLabel, lv_color_black(), 0);
    lv_obj_set_width(_sysConnLabel, 430);
    lv_obj_set_pos(_sysConnLabel, 20, 15);

    _sysHealthLabel = lv_label_create(parent);
    lv_label_set_text(_sysHealthLabel, "SYSTEM\n...");
    lv_obj_set_style_text_font(_sysHealthLabel, &lv_font_montserrat_16, 0);
    lv_obj_set_style_text_color(_sysHealthLabel, lv_color_black(), 0);
    lv_obj_set_width(_sysHealthLabel, 430);
    lv_obj_set_pos(_sysHealthLabel, 20, 200);

    // Right column — operations
    _sysOpsLabel = lv_label_create(parent);
    lv_label_set_text(_sysOpsLabel, "OPERATIONS\n...");
    lv_obj_set_style_text_font(_sysOpsLabel, &lv_font_montserrat_16, 0);
    lv_obj_set_style_text_color(_sysOpsLabel, lv_color_black(), 0);
    lv_obj_set_width(_sysOpsLabel, 430);
    lv_obj_set_pos(_sysOpsLabel, 480, 15);
}

void Display::updateSystemTab() {
    // Connectivity
    char buf[256];
    snprintf(buf, sizeof(buf),
        "CONNECTIVITY\n"
        "WiFi: %s  %s  %d dBm\n"
        "Redis: %s:%d  %s",
        _wifiConnected ? "Connected" : "Disconnected",
        _wifiIp, _wifiRssi,
        _redisHost, _redisPort,
        _redisConnected ? "Connected" : "Disconnected");
    setLabelIfChanged(_sysConnLabel, buf, _lastSysConnBuf, sizeof(_lastSysConnBuf));

    // System health
    unsigned long s = _uptimeSecs;
    int h = s / 3600;
    int m = (s % 3600) / 60;
    int sec = s % 60;

    snprintf(buf, sizeof(buf),
        "SYSTEM\n"
        "Heap: %luKB / Min: %luKB\n"
        "PSRAM: %luKB free\n"
        "Uptime: %dh %02dm %02ds\n"
        "Boot: %s\n"
        "WDT resets: %d\n"
        "Firmware: " FIRMWARE_VERSION,
        (unsigned long)_freeHeapKB, (unsigned long)_minFreeHeapKB,
        (unsigned long)_freePsramKB,
        h, m, sec,
        _bootReason,
        _watchdogResets);
    setLabelIfChanged(_sysHealthLabel, buf, _lastSysHealthBuf, sizeof(_lastSysHealthBuf));

    // Operations
    snprintf(buf, sizeof(buf),
        "OPERATIONS\n"
        "Commands: %d ok / %d fail\n"
        "CTI: %d txn / %d err\n"
        "Heartbeats: %d\n"
        "WiFi reconn: %d / down %lums\n"
        "Redis reconn: %d",
        _cmdsOk, _cmdsFail,
        _ctiTxn, _ctiErr,
        _heartbeats,
        _wifiReconnects, _wifiDownMs,
        _redisReconnects);
    setLabelIfChanged(_sysOpsLabel, buf, _lastSysOpsBuf, sizeof(_lastSysOpsBuf));
}

} // namespace arturo

#endif
