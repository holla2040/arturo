#pragma once
#include "config.h"

#ifdef ARDUINO
#include <Arduino.h>

namespace arturo {

// Call once from Station::begin() before any log output is expected from
// multiple tasks. Creates the log mutex and installs a vprintf handler with
// esp_log_set_vprintf() so that ESP-IDF's own ESP_LOG* macros serialize
// through the same mutex as our LOG_* macros.
void initLogging();

// Thread-safe formatted print to Serial. All LOG_* macros funnel through
// this so every byte written to the USB-CDC goes through the same mutex.
void logPrintf(const char* fmt, ...) __attribute__((format(printf, 1, 2)));

} // namespace arturo

#define _LOG_IMPL(tag, fmt, ...) \
    ::arturo::logPrintf("[%lu] [" tag "] " fmt "\n", (unsigned long)millis(), ##__VA_ARGS__)
#else
#define _LOG_IMPL(tag, fmt, ...) ((void)0)
#endif

#if DEBUG_LEVEL >= DEBUG_LEVEL_ERROR
#define LOG_ERROR(tag, fmt, ...) _LOG_IMPL(tag, fmt, ##__VA_ARGS__)
#else
#define LOG_ERROR(tag, fmt, ...)
#endif

#if DEBUG_LEVEL >= DEBUG_LEVEL_INFO
#define LOG_INFO(tag, fmt, ...) _LOG_IMPL(tag, fmt, ##__VA_ARGS__)
#else
#define LOG_INFO(tag, fmt, ...)
#endif

#if DEBUG_LEVEL >= DEBUG_LEVEL_DEBUG
#define LOG_DEBUG(tag, fmt, ...) _LOG_IMPL(tag, fmt, ##__VA_ARGS__)
#else
#define LOG_DEBUG(tag, fmt, ...)
#endif

#if DEBUG_LEVEL >= DEBUG_LEVEL_TRACE
#define LOG_TRACE(tag, fmt, ...) _LOG_IMPL(tag, fmt, ##__VA_ARGS__)
#else
#define LOG_TRACE(tag, fmt, ...)
#endif
