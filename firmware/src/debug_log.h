#pragma once
#include "config.h"

#ifdef ARDUINO
#include <Arduino.h>
#endif

#if DEBUG_LEVEL >= DEBUG_LEVEL_ERROR
#define LOG_ERROR(tag, fmt, ...) Serial.printf("[%lu] [" tag "] " fmt "\n", (unsigned long)millis(), ##__VA_ARGS__)
#else
#define LOG_ERROR(tag, fmt, ...)
#endif

#if DEBUG_LEVEL >= DEBUG_LEVEL_INFO
#define LOG_INFO(tag, fmt, ...) Serial.printf("[%lu] [" tag "] " fmt "\n", (unsigned long)millis(), ##__VA_ARGS__)
#else
#define LOG_INFO(tag, fmt, ...)
#endif

#if DEBUG_LEVEL >= DEBUG_LEVEL_DEBUG
#define LOG_DEBUG(tag, fmt, ...) Serial.printf("[%lu] [" tag "] " fmt "\n", (unsigned long)millis(), ##__VA_ARGS__)
#else
#define LOG_DEBUG(tag, fmt, ...)
#endif

#if DEBUG_LEVEL >= DEBUG_LEVEL_TRACE
#define LOG_TRACE(tag, fmt, ...) Serial.printf("[%lu] [" tag "] " fmt "\n", (unsigned long)millis(), ##__VA_ARGS__)
#else
#define LOG_TRACE(tag, fmt, ...)
#endif
