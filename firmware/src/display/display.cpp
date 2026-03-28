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
    lv_label_set_text(_titleLabel, "Arturo Station " FIRMWARE_VERSION "\nInitializing...");
    lv_obj_set_style_text_font(_titleLabel, &lv_font_montserrat_24, 0);
    lv_obj_set_style_text_align(_titleLabel, LV_TEXT_ALIGN_CENTER, 0);
    lv_obj_set_width(_titleLabel, 500);
    lv_obj_set_pos(_titleLabel, (1024 - 500) / 2, (600 - 60) / 2);

    // Network status — top right, right-aligned
    _statusLabel = lv_label_create(lv_scr_act());
    lv_label_set_text(_statusLabel, "WiFi: --\nRedis: --");
    lv_obj_set_style_text_font(_statusLabel, &lv_font_montserrat_14, 0);
    lv_obj_set_style_text_color(_statusLabel, lv_color_black(), 0);
    lv_obj_set_style_text_align(_statusLabel, LV_TEXT_ALIGN_RIGHT, 0);
    lv_obj_set_width(_statusLabel, 280);
    lv_obj_set_pos(_statusLabel, 1024 - 280 - 10, 10);

    display_unlock();
    display_start();
    _ready = true;

    LOG_INFO("DISPLAY", "Display initialized — LVGL running");
    return true;
}

void Display::loop() {
    if (!_ready) return;

    unsigned long now = millis();
    if (now - _lastUpdateMs < 1000) return;
    _lastUpdateMs = now;

    if (!display_lock(100)) return;
    updateStatusLabel();
    display_unlock();
}

void Display::setWifiStatus(bool connected, const char* ip, int rssi) {
    _wifiConnected = connected;
    if (ip) {
        strncpy(_wifiIp, ip, sizeof(_wifiIp) - 1);
        _wifiIp[sizeof(_wifiIp) - 1] = '\0';
    } else {
        _wifiIp[0] = '\0';
    }
    _wifiRssi = rssi;
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
            "WiFi: %s  %4ddBm\nRedis: %s:%d  %s",
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

} // namespace arturo

#endif
