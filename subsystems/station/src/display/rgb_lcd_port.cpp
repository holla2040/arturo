/*****************************************************************************
 * | File         :   rgb_lcd_port.cpp
 * | Author       :   Waveshare team
 * | Function     :   RGB LCD driver
 * | Info         :   Ported from pendant2 drivers/rgb_lcd_port.cpp
 *                    Fixed: TAG made static to avoid linker conflicts
 ******************************************************************************/

#include "rgb_lcd_port.h"
#include "lvgl_port.h"

static const char *TAG = "rgb_lcd_port";

static esp_lcd_panel_handle_t panel_handle = NULL;

IRAM_ATTR static bool rgb_lcd_on_vsync_event(esp_lcd_panel_handle_t panel, const esp_lcd_rgb_panel_event_data_t *edata, void *user_ctx)
{
    return lvgl_port_notify_rgb_vsync();
}

esp_lcd_panel_handle_t waveshare_esp32_s3_rgb_lcd_init()
{
    ESP_LOGI(TAG, "Install RGB LCD panel driver");

    esp_lcd_rgb_panel_config_t panel_config = {
        .clk_src = LCD_CLK_SRC_DEFAULT,
        .timings = {
            .pclk_hz = EXAMPLE_LCD_PIXEL_CLOCK_HZ,
            .h_res = EXAMPLE_LCD_H_RES,
            .v_res = EXAMPLE_LCD_V_RES,
            .hsync_pulse_width = 162,
            .hsync_back_porch = 152,
            .hsync_front_porch = 48,
            .vsync_pulse_width = 45,
            .vsync_back_porch = 13,
            .vsync_front_porch = 3,
            .flags = {
                .pclk_active_neg = 1,
            },
        },
        .data_width = EXAMPLE_RGB_DATA_WIDTH,
        .bits_per_pixel = EXAMPLE_RGB_BIT_PER_PIXEL,
        .num_fbs = EXAMPLE_LCD_RGB_BUFFER_NUMS,
        .bounce_buffer_size_px = EXAMPLE_RGB_BOUNCE_BUFFER_SIZE,
        .sram_trans_align = 4,
        .psram_trans_align = 64,
        .hsync_gpio_num = EXAMPLE_LCD_IO_RGB_HSYNC,
        .vsync_gpio_num = EXAMPLE_LCD_IO_RGB_VSYNC,
        .de_gpio_num = EXAMPLE_LCD_IO_RGB_DE,
        .pclk_gpio_num = EXAMPLE_LCD_IO_RGB_PCLK,
        .disp_gpio_num = EXAMPLE_LCD_IO_RGB_DISP,
        .data_gpio_nums = {
            EXAMPLE_LCD_IO_RGB_DATA0,
            EXAMPLE_LCD_IO_RGB_DATA1,
            EXAMPLE_LCD_IO_RGB_DATA2,
            EXAMPLE_LCD_IO_RGB_DATA3,
            EXAMPLE_LCD_IO_RGB_DATA4,
            EXAMPLE_LCD_IO_RGB_DATA5,
            EXAMPLE_LCD_IO_RGB_DATA6,
            EXAMPLE_LCD_IO_RGB_DATA7,
            EXAMPLE_LCD_IO_RGB_DATA8,
            EXAMPLE_LCD_IO_RGB_DATA9,
            EXAMPLE_LCD_IO_RGB_DATA10,
            EXAMPLE_LCD_IO_RGB_DATA11,
            EXAMPLE_LCD_IO_RGB_DATA12,
            EXAMPLE_LCD_IO_RGB_DATA13,
            EXAMPLE_LCD_IO_RGB_DATA14,
            EXAMPLE_LCD_IO_RGB_DATA15,
        },
        .flags = {
            .fb_in_psram = 1,
        },
    };

    ESP_ERROR_CHECK(esp_lcd_new_rgb_panel(&panel_config, &panel_handle));

    ESP_LOGI(TAG, "Register vsync callbacks");
    esp_lcd_rgb_panel_event_callbacks_t cbs = {
        .on_vsync = rgb_lcd_on_vsync_event,
        .on_bounce_frame_finish = rgb_lcd_on_vsync_event,
    };
    ESP_ERROR_CHECK(esp_lcd_rgb_panel_register_event_callbacks(panel_handle, &cbs, NULL));

    ESP_LOGI(TAG, "Initialize RGB LCD panel");
    ESP_ERROR_CHECK(esp_lcd_panel_init(panel_handle));

    return panel_handle;
}

void wavesahre_rgb_lcd_display_window(int16_t Xstart, int16_t Ystart, int16_t Xend, int16_t Yend, uint8_t *Image)
{
    if (Xstart < 0) Xstart = 0;
    else if (Xend > EXAMPLE_LCD_H_RES) Xend = EXAMPLE_LCD_H_RES;

    if (Ystart < 0) Ystart = 0;
    else if (Yend > EXAMPLE_LCD_V_RES) Yend = EXAMPLE_LCD_V_RES;

    int crop_width = Xend - Xstart;
    int crop_height = Yend - Ystart;

    uint8_t *dst_data = (uint8_t *)malloc(crop_width * crop_height * 2);
    if (!dst_data) {
        printf("Error: Failed to allocate memory for cropped bitmap.\n");
        return;
    }

    for (int y = 0; y < crop_height; y++) {
        const uint8_t *src_row = Image + ((Ystart + y) * EXAMPLE_LCD_H_RES + Xstart) * 2;
        uint8_t *dst_row = dst_data + y * crop_width * 2;
        memcpy(dst_row, src_row, crop_width * 2);
    }

    esp_lcd_panel_draw_bitmap(panel_handle, Xstart, Ystart, Xend, Yend, dst_data);
    free(dst_data);
}

void wavesahre_rgb_lcd_display(uint8_t *Image)
{
    esp_lcd_panel_draw_bitmap(panel_handle, 0, 0, EXAMPLE_LCD_H_RES, EXAMPLE_LCD_V_RES, Image);
}

void waveshare_get_frame_buffer(void **buf1, void **buf2)
{
    ESP_ERROR_CHECK(esp_lcd_rgb_panel_get_frame_buffer(panel_handle, 2, buf1, buf2));
}

void wavesahre_rgb_lcd_bl_on()
{
    IO_EXTENSION_Output(IO_EXTENSION_IO_2, 1);
}

void wavesahre_rgb_lcd_bl_off()
{
    IO_EXTENSION_Output(IO_EXTENSION_IO_2, 0);
}
