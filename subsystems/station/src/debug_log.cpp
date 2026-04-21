#ifdef ARDUINO

#include "debug_log.h"
#include "config.h"
#include <Arduino.h>
#include <freertos/FreeRTOS.h>
#include <freertos/semphr.h>
#include <esp_log.h>
#include <WiFi.h>
#include <WiFiUdp.h>
#include <cstdarg>
#include <cstdio>

namespace arturo {

// Recursive so that WiFi/LwIP code called during UDP send — which itself
// emits ESP_LOG from the same task — can re-enter the log path safely.
static SemaphoreHandle_t _logMutex = nullptr;
static WiFiUDP           _logUdp;
static const IPAddress   _logBroadcast(255, 255, 255, 255);

static int logVprintfImpl(const char* fmt, va_list args) {
    char buf[256];
    int n = vsnprintf(buf, sizeof(buf), fmt, args);
    if (n < 0) return 0;
    if (n > (int)sizeof(buf) - 1) n = sizeof(buf) - 1;

    if (_logMutex) xSemaphoreTakeRecursive(_logMutex, portMAX_DELAY);

    Serial.write(reinterpret_cast<const uint8_t*>(buf), (size_t)n);

#if LOG_UDP_PORT
    if (WiFi.isConnected()) {
        if (_logUdp.beginPacket(_logBroadcast, LOG_UDP_PORT)) {
            _logUdp.write(reinterpret_cast<const uint8_t*>(buf), (size_t)n);
            _logUdp.endPacket();
        }
    }
#endif

    if (_logMutex) xSemaphoreGiveRecursive(_logMutex);
    return n;
}

void initLogging() {
    if (_logMutex == nullptr) {
        _logMutex = xSemaphoreCreateRecursiveMutex();
    }
    esp_log_set_vprintf(logVprintfImpl);
}

void logPrintf(const char* fmt, ...) {
    va_list args;
    va_start(args, fmt);
    logVprintfImpl(fmt, args);
    va_end(args);
}

} // namespace arturo

#endif // ARDUINO
