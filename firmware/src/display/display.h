#pragma once

#ifdef ARDUINO
#include "lvgl.h"

namespace arturo {

class Display {
public:
    bool begin();
    void loop();

    // Push state from Station — Display never reaches outside itself
    void setWifiStatus(bool connected, const char* ip);
    void setRedisStatus(bool connected, const char* host, uint16_t port);

private:
    bool _ready = false;
    unsigned long _lastUpdateMs = 0;

    // LVGL objects
    lv_obj_t* _titleLabel = nullptr;
    lv_obj_t* _statusLabel = nullptr;

    // Cached state for rendering
    bool _wifiConnected = false;
    char _wifiIp[16] = {};
    bool _redisConnected = false;
    char _redisHost[64] = {};
    uint16_t _redisPort = 0;

    // Last rendered status text — skip redraw when unchanged
    char _lastStatusBuf[128] = {};

    void updateStatusLabel();
};

} // namespace arturo

#endif
