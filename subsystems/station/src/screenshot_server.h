/*
 * Screenshot Server - WiFi-based screenshot capture for debugging
 *
 * Adapted from pendant2 for Arturo station firmware.
 * Enable/disable with ENABLE_SCREENSHOT_SERVER in config.h
 *
 * Features:
 * - HTTP server with JPEG screenshot endpoint
 * - Web interface with auto-refresh
 * - Manual capture via button or HTTP
 * - Stats endpoint for monitoring
 *
 * Memory Requirements:
 * - ~1.2MB PSRAM for RGB565 snapshot buffer (temporary)
 * - ~200KB PSRAM for JPEG output buffer (temporary)
 *
 * HTTP Endpoints:
 * - GET /            - Web interface with auto-refresh controls
 * - GET /screen.jpg  - JPEG screenshot download
 * - GET /stats       - JSON statistics
 * - GET /capture     - Trigger screenshot capture
 */

#pragma once

#include "config.h"

#ifdef ENABLE_SCREENSHOT_SERVER

#include <Arduino.h>

// Initialize screenshot server (HTTP server only — WiFi must already be connected)
// Call from setup() after LVGL and WiFi initialization
void screenshot_server_init();

// Update screenshot server (handle HTTP requests)
// Call from a task or loop periodically
void screenshot_server_update();

// Manually trigger screenshot capture
void screenshot_server_capture();

// Check if capture is in progress
bool screenshot_server_busy();

#endif // ENABLE_SCREENSHOT_SERVER
