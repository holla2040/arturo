#ifdef ARDUINO

#include "display.h"
#include "display_init.h"
#include "../config.h"
#include "../debug_log.h"
#include <Arduino.h>

// Custom 160pt numeric font (digits, -, ., space)
extern const lv_font_t font_numeric_160;

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
    LV_SYMBOL_SETTINGS, // System
};
static const char* TAB_NAMES[] = {"Status", "Chart", "System"};

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
    lv_obj_set_style_text_font(tabBtns, &lv_font_montserrat_32, 0);

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

    // Only update the active tab (critical performance optimization)
    int tab = activeTab();
    if (tab == TAB_STATUS) updateStatusTab();
    else if (tab == TAB_CHART) updateChartTab();
    else if (tab == TAB_SYSTEM) updateSystemTab();

    display_unlock();
}

int Display::activeTab() const {
    if (!_tabview) return 0;
    return lv_tabview_get_tab_act(_tabview);
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
    _pump = telemetry;
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

    // Title — left side: "Arturo Station - station-01"
    _bannerTitle = lv_label_create(banner);
    lv_label_set_text(_bannerTitle, "Arturo Station - " STATION_INSTANCE);
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
    lv_obj_set_style_text_font(_bannerClock, &lv_font_montserrat_16, 0);
    lv_obj_set_style_text_color(_bannerClock, lv_color_hex(0x999999), 0);
    lv_obj_set_style_text_align(_bannerClock, LV_TEXT_ALIGN_RIGHT, 0);
    lv_obj_set_width(_bannerClock, 200);
    lv_obj_set_pos(_bannerClock, SCREEN_W - 210, 5);
}

void Display::updateBanner() {
    // Clock — updates every loop call (100ms)
    unsigned long ms = millis();
    unsigned long totalSecs = ms / 1000;
    int h = (totalSecs / 3600) % 24;
    int m = (totalSecs / 60) % 60;
    int s = totalSecs % 60;
    char clockBuf[48];
    snprintf(clockBuf, sizeof(clockBuf), "%02d:%02d:%02d", h, m, s);
    lv_label_set_text(_bannerClock, clockBuf);

    // IP address
    if (_wifiConnected && _wifiIp[0] != '\0') {
        lv_label_set_text(_bannerIp, _wifiIp);
    } else {
        lv_label_set_text(_bannerIp, "no wifi");
    }

    // Communication indicator — green if pump is fresh, red if stale
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

void Display::updateStatusTab() {
    unsigned long now = millis();
    if (now - _lastUpdateMs < 500) return;  // 2 Hz for status tab
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

        const char* regenStates[] = {"OFF", "WARMUP", "PURGE", "ROUGH", "ROR", "COOL"};
        int step = _pump.regenStep;
        if (step >= 0 && step <= 5) {
            lv_label_set_text(_regenLabel, regenStates[step]);
            lv_obj_set_style_text_color(_regenLabel, step > 0 ? COL_ORANGE : COL_GRAY, 0);
        }

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
}

// --- Chart Tab ---

void Display::initChartTab(lv_obj_t* parent) {
    // Chart positioned with left margin for manual Y-axis labels
    static const int CHART_LEFT = 50;
    static const int CHART_TOP = 40;
    static const int CHART_W = CONTENT_W - CHART_LEFT - 15;
    static const int CHART_H = CONTENT_H - CHART_TOP - 10;

    _chart = lv_chart_create(parent);
    lv_obj_set_size(_chart, CHART_W, CHART_H);
    lv_obj_set_pos(_chart, CHART_LEFT, CHART_TOP);
    lv_chart_set_type(_chart, LV_CHART_TYPE_LINE);
    lv_chart_set_point_count(_chart, CHART_POINTS);
    lv_chart_set_range(_chart, LV_CHART_AXIS_PRIMARY_Y, 0, 320);
    lv_chart_set_update_mode(_chart, LV_CHART_UPDATE_MODE_SHIFT);

    // Grid lines — 8 divisions = 40K increments (0, 40, 80, ... 320)
    lv_chart_set_div_line_count(_chart, 8, 0);
    lv_obj_set_style_line_color(_chart, lv_color_hex(0xD0D0D0), LV_PART_MAIN);
    lv_obj_set_style_line_width(_chart, 1, LV_PART_MAIN);

    // Style
    lv_obj_set_style_bg_color(_chart, lv_color_hex(0xFFFFFF), 0);
    lv_obj_set_style_bg_opa(_chart, LV_OPA_COVER, 0);
    lv_obj_set_style_border_width(_chart, 1, 0);
    lv_obj_set_style_border_color(_chart, lv_color_hex(0xBBBBBB), 0);
    lv_obj_set_style_size(_chart, 0, LV_PART_INDICATOR);
    lv_obj_set_style_pad_all(_chart, 5, 0);

    // Series — bright red and blue, 2px line width
    _chartSeries1 = lv_chart_add_series(_chart, lv_color_hex(0xFF0000), LV_CHART_AXIS_PRIMARY_Y);
    _chartSeries2 = lv_chart_add_series(_chart, lv_color_hex(0x0000FF), LV_CHART_AXIS_PRIMARY_Y);
    lv_obj_set_style_line_width(_chart, 2, LV_PART_ITEMS);

    // Manual Y-axis labels (pendant2 pattern — LVGL 8.4 tick labels unreliable)
    // Plot area height = CHART_H - 10 (pad top + pad bottom)
    int plotH = CHART_H - 10;
    const int yAxisValues[] = {0, 40, 80, 120, 160, 200, 240, 280, 320};
    for (int i = 0; i < 9; i++) {
        int v = yAxisValues[i];
        // Y position: top of chart + pad + (320-v)/320 * plotH
        int yPos = CHART_TOP + 5 + (320 - v) * plotH / 320 - 7;  // -7 to center text on line

        lv_obj_t* lbl = lv_label_create(parent);
        lv_label_set_text_fmt(lbl, "%d", v);
        lv_obj_set_style_text_font(lbl, &lv_font_montserrat_14, 0);
        lv_obj_set_style_text_color(lbl, lv_color_hex(0x666666), 0);
        lv_obj_set_style_text_align(lbl, LV_TEXT_ALIGN_RIGHT, 0);
        lv_obj_set_width(lbl, 42);
        lv_obj_set_pos(lbl, 2, yPos);
    }

    // Legend with live values — above chart
    _chartLegend1 = lv_label_create(parent);
    lv_label_set_text(_chartLegend1, "#FF6060 1st: ---#");
    lv_obj_set_style_text_font(_chartLegend1, &lv_font_montserrat_24, 0);
    lv_obj_set_pos(_chartLegend1, CHART_LEFT + 10, 5);
    lv_label_set_recolor(_chartLegend1, true);

    _chartLegend2 = lv_label_create(parent);
    lv_label_set_text(_chartLegend2, "#6060FF 2nd: ---#");
    lv_obj_set_style_text_font(_chartLegend2, &lv_font_montserrat_24, 0);
    lv_obj_set_pos(_chartLegend2, CHART_LEFT + 250, 5);
    lv_label_set_recolor(_chartLegend2, true);
}

void Display::updateChartTab() {
    unsigned long now = millis();

    // Sample every 30 seconds if pump data is fresh
    if (now - _lastChartSampleMs >= CHART_SAMPLE_INTERVAL_MS && _pump.staleCount <= 2) {
        _lastChartSampleMs = now;
        lv_chart_set_next_value(_chart, _chartSeries1, (lv_coord_t)(_pump.stage1TempK + 0.5f));
        lv_chart_set_next_value(_chart, _chartSeries2, (lv_coord_t)(_pump.stage2TempK + 0.5f));
        _chartSampleCount++;
        lv_chart_refresh(_chart);
    }

    // Update legend with current values
    if (_pump.staleCount <= 2) {
        lv_label_set_text_fmt(_chartLegend1, "#FF6060 1st: %d#", (int)(_pump.stage1TempK + 0.5f));
        lv_label_set_text_fmt(_chartLegend2, "#6060FF 2nd: %d#", (int)(_pump.stage2TempK + 0.5f));
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
