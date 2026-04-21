#ifdef ARDUINO

#include "debug_log.h"
#include <Arduino.h>
#include <freertos/FreeRTOS.h>
#include <freertos/semphr.h>
#include <esp_log.h>
#include <cstdarg>

namespace arturo {

static SemaphoreHandle_t _logMutex = nullptr;

// Single point where bytes go to Serial. Both our logPrintf() and ESP-IDF's
// esp_log path route through here, so the mutex is held for every byte.
static int logVprintfImpl(const char* fmt, va_list args) {
    if (_logMutex) xSemaphoreTake(_logMutex, portMAX_DELAY);
    int ret = Serial.vprintf(fmt, args);
    if (_logMutex) xSemaphoreGive(_logMutex);
    return ret;
}

void initLogging() {
    if (_logMutex == nullptr) {
        _logMutex = xSemaphoreCreateMutex();
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
