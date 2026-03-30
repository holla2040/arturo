/**
 * @file display_init.cpp
 * @brief Display subsystem initialization for Arturo station firmware
 *
 * Encapsulates the full init sequence for the Waveshare ESP32-S3-Touch-LCD-7B:
 * I2C -> IO expander -> touch (GT911) -> RGB LCD -> backlight -> LVGL
 */

#include "display_init.h"
#include "gt911.h"
#include "rgb_lcd_port.h"
#include "lvgl_port.h"
#include "esp_log.h"

static const char *TAG = "display";

bool display_init(void)
{
    // 1. Initialize touch controller (internally does I2C init, IO expander init,
    //    USB mux fix IO5=0, touch reset sequence, GT911 driver creation)
    ESP_LOGI(TAG, "Initializing touch controller...");
    esp_lcd_touch_handle_t tp = touch_gt911_init();
    if (!tp) {
        ESP_LOGE(TAG, "Touch controller init failed");
        return false;
    }

    // 2. Initialize RGB LCD panel (PSRAM framebuffers, vsync callbacks)
    ESP_LOGI(TAG, "Initializing RGB LCD panel...");
    esp_lcd_panel_handle_t lcd = waveshare_esp32_s3_rgb_lcd_init();
    if (!lcd) {
        ESP_LOGE(TAG, "RGB LCD init failed");
        return false;
    }

    // 3. Turn on backlight via IO expander
    wavesahre_rgb_lcd_bl_on();

    // 4. Initialize LVGL (creates display/input drivers and FreeRTOS task in waiting state)
    ESP_LOGI(TAG, "Initializing LVGL...");
    esp_err_t err = lvgl_port_init(lcd, tp);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "LVGL port init failed: %s", esp_err_to_name(err));
        return false;
    }

    ESP_LOGI(TAG, "Display subsystem initialized");
    return true;
}

void display_start(void)
{
    lvgl_port_task_start();
}

bool display_lock(int timeout_ms)
{
    return lvgl_port_lock(timeout_ms);
}

void display_unlock(void)
{
    lvgl_port_unlock();
}
