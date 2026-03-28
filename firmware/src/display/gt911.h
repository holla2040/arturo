/*
 * SPDX-FileCopyrightText: 2022-2024 Espressif Systems (Shanghai) CO LTD
 * SPDX-License-Identifier: Apache-2.0
 *
 * Ported from pendant2 drivers/gt911.h
 */

#ifndef __GT911_H
#define __GT911_H

#include "touch.h"

#define ESP_LCD_TOUCH_IO_I2C_GT911_ADDRESS          (0x5D)
#define ESP_LCD_TOUCH_IO_I2C_GT911_ADDRESS_BACKUP   (0x14)

#define EXAMPLE_PIN_NUM_TOUCH_RST       (GPIO_NUM_NC)
#define EXAMPLE_PIN_NUM_TOUCH_INT       (GPIO_NUM_4)

typedef struct {
    uint8_t dev_addr;
} esp_lcd_touch_io_gt911_config_t;

typedef struct {
    uint16_t x[ESP_LCD_TOUCH_MAX_POINTS];
    uint16_t y[ESP_LCD_TOUCH_MAX_POINTS];
    uint8_t cnt;
} touch_gt911_point_t;

esp_err_t esp_lcd_touch_new_i2c_gt911(const esp_lcd_panel_io_handle_t io, const esp_lcd_touch_config_t *config, esp_lcd_touch_handle_t *out_touch);
esp_lcd_touch_handle_t touch_gt911_init();
touch_gt911_point_t touch_gt911_read_point(uint8_t max_touch_cnt);

#define ESP_LCD_TOUCH_IO_I2C_GT911_CONFIG()           \
    {                                                 \
        .dev_addr = ESP_LCD_TOUCH_IO_I2C_GT911_ADDRESS,   \
        .control_phase_bytes = 1,                         \
        .dc_bit_offset = 0,                               \
        .lcd_cmd_bits = 16,                               \
        .flags =                                          \
        {                                                 \
            .disable_control_phase = 1,                   \
        },                                                \
        .scl_speed_hz = 400 * 1000,                       \
    }

#endif
