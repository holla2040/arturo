#pragma once

#include <stdint.h>

#include "esp_err.h"
#include "esp_lcd_types.h"
#include "touch.h"
#include "lvgl.h"
#include "rgb_lcd_port.h"
#include "gt911.h"

/**
 * LVGL display parameters
 */
#define LVGL_PORT_H_RES             (EXAMPLE_LCD_H_RES)
#define LVGL_PORT_V_RES             (EXAMPLE_LCD_V_RES)
#define LVGL_PORT_TICK_PERIOD_MS    (2)

/**
 * LVGL timer handle task parameters
 */
#define LVGL_PORT_TASK_MAX_DELAY_MS (500)
#define LVGL_PORT_TASK_MIN_DELAY_MS (1)
#define LVGL_PORT_TASK_STACK_SIZE   (6 * 1024)
#define LVGL_PORT_TASK_PRIORITY     (10)
#define LVGL_PORT_TASK_CORE         (1)   // Core 1 — separated from WiFi stack on Core 0

/**
 * LVGL buffer configuration
 */
#define CONFIG_EXAMPLE_LVGL_PORT_BUF_PSRAM 1
#define CONFIG_EXAMPLE_LVGL_PORT_BUF_INTERNAL 0

#if CONFIG_EXAMPLE_LVGL_PORT_BUF_PSRAM
#define LVGL_PORT_BUFFER_MALLOC_CAPS    (MALLOC_CAP_SPIRAM)
#elif CONFIG_EXAMPLE_LVGL_PORT_BUF_INTERNAL
#define LVGL_PORT_BUFFER_MALLOC_CAPS    (MALLOC_CAP_INTERNAL | MALLOC_CAP_8BIT)
#endif
#define LVGL_PORT_BUFFER_HEIGHT         (100)

/**
 * Avoid tearing configuration
 */
#define LVGL_PORT_AVOID_TEAR_ENABLE     (1)
#if LVGL_PORT_AVOID_TEAR_ENABLE
#define LVGL_PORT_AVOID_TEAR_MODE       (3) // Double-buffer + direct-mode (recommended)
#define EXAMPLE_LVGL_PORT_ROTATION_DEGREE  (0)

#if LVGL_PORT_AVOID_TEAR_MODE == 1
#define LVGL_PORT_LCD_RGB_BUFFER_NUMS   (2)
#define LVGL_PORT_FULL_REFRESH          (1)
#elif LVGL_PORT_AVOID_TEAR_MODE == 2
#define LVGL_PORT_LCD_RGB_BUFFER_NUMS   (3)
#define LVGL_PORT_FULL_REFRESH          (1)
#elif LVGL_PORT_AVOID_TEAR_MODE == 3
#define LVGL_PORT_LCD_RGB_BUFFER_NUMS   (2)
#define LVGL_PORT_DIRECT_MODE           (1)
#endif

#if EXAMPLE_LVGL_PORT_ROTATION_DEGREE == 0
#define EXAMPLE_LVGL_PORT_ROTATION_0    (1)
#else
#if EXAMPLE_LVGL_PORT_ROTATION_DEGREE == 90
#define EXAMPLE_LVGL_PORT_ROTATION_90   (1)
#elif EXAMPLE_LVGL_PORT_ROTATION_DEGREE == 180
#define EXAMPLE_LVGL_PORT_ROTATION_180  (1)
#elif EXAMPLE_LVGL_PORT_ROTATION_DEGREE == 270
#define EXAMPLE_LVGL_PORT_ROTATION_270  (1)
#endif
#ifdef LVGL_PORT_LCD_RGB_BUFFER_NUMS
#undef LVGL_PORT_LCD_RGB_BUFFER_NUMS
#define LVGL_PORT_LCD_RGB_BUFFER_NUMS   (3)
#endif
#endif
#else
#define LVGL_PORT_LCD_RGB_BUFFER_NUMS   (1)
#define LVGL_PORT_FULL_REFRESH          (0)
#define LVGL_PORT_DIRECT_MODE           (0)
#endif

esp_err_t lvgl_port_init(esp_lcd_panel_handle_t lcd_handle, esp_lcd_touch_handle_t tp_handle);
bool lvgl_port_lock(int timeout_ms);
void lvgl_port_unlock(void);
bool lvgl_port_notify_rgb_vsync(void);
void lvgl_port_task_start(void);
TaskHandle_t lvgl_port_get_task_handle(void);
