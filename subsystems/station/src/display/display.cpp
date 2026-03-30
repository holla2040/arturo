#ifdef ARDUINO

#include "display.h"
#include "display_init.h"
#include "../config.h"
#include "../debug_log.h"
#include <Arduino.h>

namespace arturo {

bool Display::begin() {
    if (!display_init()) {
        LOG_ERROR("DISPLAY", "Display init failed — continuing without display");
        return false;
    }

    display_lock(-1);

    // Station name — fixed position, no layout recalculation on text change
    _titleLabel = lv_label_create(lv_scr_act());
    lv_label_set_text(_titleLabel, "Arturo Station " FIRMWARE_VERSION);
    lv_obj_set_style_text_font(_titleLabel, &lv_font_montserrat_24, 0);
    lv_obj_set_style_text_align(_titleLabel, LV_TEXT_ALIGN_CENTER, 0);
    lv_obj_set_width(_titleLabel, 500);
    lv_obj_set_pos(_titleLabel, (1024 - 500) / 2, (600 - 60) / 2);

    // Clock — top left, updates every 100ms to verify display refresh
    _clockLabel = lv_label_create(lv_scr_act());
    lv_label_set_text(_clockLabel, "00:00:00.0");
    lv_obj_set_style_text_font(_clockLabel, &lv_font_montserrat_24, 0);
    lv_obj_set_style_text_color(_clockLabel, lv_color_make(0x66, 0x66, 0x66), 0);
    lv_obj_set_pos(_clockLabel, 10, 10);

    // Network status — top right, right-aligned
    _statusLabel = lv_label_create(lv_scr_act());
    lv_label_set_text(_statusLabel, "WiFi: --\nRedis: --");
    lv_obj_set_style_text_font(_statusLabel, &lv_font_montserrat_14, 0);
    lv_obj_set_style_text_color(_statusLabel, lv_color_black(), 0);
    lv_obj_set_style_text_align(_statusLabel, LV_TEXT_ALIGN_RIGHT, 0);
    lv_obj_set_width(_statusLabel, 280);
    lv_obj_set_pos(_statusLabel, 1024 - 280 - 10, 10);

    // System stats — left column, below title
    _systemStatsLabel = lv_label_create(lv_scr_act());
    lv_label_set_text(_systemStatsLabel, "");
    lv_obj_set_style_text_font(_systemStatsLabel, &lv_font_montserrat_16, 0);
    lv_obj_set_style_text_color(_systemStatsLabel, lv_color_make(0x66, 0x66, 0x66), 0);
    lv_obj_set_width(_systemStatsLabel, 440);
    lv_obj_set_pos(_systemStatsLabel, 60, 360);

    // Ops stats — right column, below title
    _opsStatsLabel = lv_label_create(lv_scr_act());
    lv_label_set_text(_opsStatsLabel, "");
    lv_obj_set_style_text_font(_opsStatsLabel, &lv_font_montserrat_16, 0);
    lv_obj_set_style_text_color(_opsStatsLabel, lv_color_make(0x66, 0x66, 0x66), 0);
    lv_obj_set_width(_opsStatsLabel, 440);
    lv_obj_set_pos(_opsStatsLabel, 524, 360);

    display_unlock();
    display_start();
    _ready = true;

    LOG_INFO("DISPLAY", "Display initialized — LVGL running");
    return true;
}

void Display::loop() {
    if (!_ready) return;
    if (!display_lock(100)) return;

    // Clock — update every call (100ms from displayTask)
    unsigned long ms = millis();
    unsigned long totalSecs = ms / 1000;
    int h = (totalSecs / 3600) % 24;
    int m = (totalSecs / 60) % 60;
    int s = totalSecs % 60;
    int tenths = (ms / 100) % 10;
    char clockBuf[16];
    snprintf(clockBuf, sizeof(clockBuf), "%02d:%02d:%02d.%d", h, m, s, tenths);
    lv_label_set_text(_clockLabel, clockBuf);

    // Status — update once per second
    unsigned long now = millis();
    if (now - _lastUpdateMs >= 1000) {
        _lastUpdateMs = now;
        updateStatusLabel();
        updateSystemStatsLabel();
        updateOpsStatsLabel();
    }

    display_unlock();
}

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

void Display::updateStatusLabel() {
    char buf[128];
    if (_wifiConnected) {
        snprintf(buf, sizeof(buf),
            "WiFi: %s  %d dBm\nRedis: %s:%d  %s",
            _wifiIp, _wifiRssi,
            _redisHost, _redisPort,
            _redisConnected ? LV_SYMBOL_OK : LV_SYMBOL_CLOSE);
    } else {
        snprintf(buf, sizeof(buf),
            "WiFi: disconnected\nRedis: %s:%d  --",
            _redisHost, _redisPort);
    }

    // Only update the label when text actually changed — avoids unnecessary invalidation
    if (strcmp(buf, _lastStatusBuf) != 0) {
        lv_label_set_text(_statusLabel, buf);
        memcpy(_lastStatusBuf, buf, sizeof(_lastStatusBuf));
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

void Display::updateSystemStatsLabel() {
    unsigned long s = _uptimeSecs;
    int h = s / 3600;
    int m = (s % 3600) / 60;
    int sec = s % 60;

    char buf[192];
    snprintf(buf, sizeof(buf),
        "SYSTEM\n"
        "Heap: %luKB / Min: %luKB\n"
        "PSRAM: %luKB free\n"
        "Uptime: %dh %02dm %02ds\n"
        "Boot: %s\n"
        "WDT resets: %d",
        (unsigned long)_freeHeapKB, (unsigned long)_minFreeHeapKB,
        (unsigned long)_freePsramKB,
        h, m, sec,
        _bootReason,
        _watchdogResets);

    if (strcmp(buf, _lastSystemStatsBuf) != 0) {
        lv_label_set_text(_systemStatsLabel, buf);
        memcpy(_lastSystemStatsBuf, buf, sizeof(_lastSystemStatsBuf));
    }
}

void Display::updateOpsStatsLabel() {
    char buf[192];
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

    if (strcmp(buf, _lastOpsStatsBuf) != 0) {
        lv_label_set_text(_opsStatsLabel, buf);
        memcpy(_lastOpsStatsBuf, buf, sizeof(_lastOpsStatsBuf));
    }
}

} // namespace arturo

#endif
