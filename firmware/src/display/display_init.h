#pragma once

#include <stdbool.h>

// Initialize I2C bus, IO expander, touch controller, RGB LCD, and LVGL.
// Creates the LVGL FreeRTOS task (in waiting state). Returns true on success.
bool display_init(void);

// Signal the LVGL task to start running (call after initial UI is created).
void display_start(void);

// Lock/unlock LVGL mutex for thread-safe UI updates from other tasks.
bool display_lock(int timeout_ms);
void display_unlock(void);
