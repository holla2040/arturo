#include "watchdog.h"

#ifdef ARDUINO
#include <esp_task_wdt.h>
#include "../debug_log.h"
#include "../config.h"
#endif

namespace arturo {

bool watchdogFeedDue(unsigned long lastFeedMs, unsigned long nowMs,
                     unsigned long intervalMs) {
    // Handle millis() overflow
    unsigned long elapsed = nowMs - lastFeedMs;
    return elapsed >= intervalMs;
}

#ifdef ARDUINO
Watchdog::Watchdog()
    : _lastFeedMs(0), _resetCount(0), _initialized(false) {}

bool Watchdog::init(unsigned long timeoutMs) {
    // Check if last reset was caused by watchdog
    esp_reset_reason_t reason = esp_reset_reason();
    if (reason == ESP_RST_TASK_WDT || reason == ESP_RST_WDT) {
        _resetCount++;
        LOG_ERROR("WDT", "Previous reset was watchdog! count=%d", _resetCount);
    }

    // Configure the Task Watchdog Timer
    // timeoutMs converted to seconds (minimum 1s)
    uint32_t timeoutS = timeoutMs / 1000;
    if (timeoutS == 0) timeoutS = 1;

    esp_task_wdt_config_t config = {
        .timeout_ms = timeoutMs,
        .idle_core_mask = 0,    // don't watch idle tasks
        .trigger_panic = true   // reset on timeout
    };
    esp_err_t err = esp_task_wdt_reconfigure(&config);
    if (err != ESP_OK) {
        // Try init if not yet initialized
        err = esp_task_wdt_init(&config);
        if (err != ESP_OK) {
            LOG_ERROR("WDT", "Init failed: %d", err);
            return false;
        }
    }

    // Subscribe current task to watchdog
    err = esp_task_wdt_add(NULL);
    if (err != ESP_OK && err != ESP_ERR_INVALID_STATE) {
        LOG_ERROR("WDT", "Failed to subscribe task: %d", err);
        return false;
    }

    _initialized = true;
    _lastFeedMs = millis();

    LOG_INFO("WDT", "Initialized: %lu ms timeout", timeoutMs);
    return true;
}

void Watchdog::feed() {
    if (!_initialized) return;

    esp_task_wdt_reset();
    _lastFeedMs = millis();

    LOG_TRACE("WDT", "Fed at %lu ms", _lastFeedMs);
}
#endif

} // namespace arturo
