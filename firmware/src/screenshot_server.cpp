/*
 * Screenshot Server - WiFi-based screenshot capture for debugging
 *
 * Adapted from pendant2 for Arturo station firmware.
 * WiFi is managed externally by WifiManager — this module only runs the HTTP server.
 */

#include "config.h"

#ifdef ENABLE_SCREENSHOT_SERVER

#include "screenshot_server.h"
#include "debug_log.h"
#include "display/lvgl_port.h"
#include <WiFi.h>
#include <WebServer.h>
#include <JPEGENC.h>

#define TAG "SCREENSHOT"

// HTTP server
static WebServer server(80);

// Stats
static uint32_t captureCount = 0;
static uint32_t startTime = 0;
static bool captureInProgress = false;

// Screenshot buffer (allocated in PSRAM)
static uint8_t *screenshotBuffer = NULL;
static size_t screenshotSize = 0;
static SemaphoreHandle_t screenshotMutex = NULL;

// Forward declarations
static void captureScreenshot();
static void handleRoot();
static void handleScreenshot();
static void handleStats();
static void handleCapture();

// Initialize screenshot server (WiFi must already be connected)
void screenshot_server_init() {
    LOG_INFO(TAG, "Initializing...");

    // Initialize semaphore
    screenshotMutex = xSemaphoreCreateMutex();
    if (!screenshotMutex) {
        LOG_ERROR(TAG, "Failed to create mutex");
        return;
    }

    if (WiFi.status() != WL_CONNECTED) {
        LOG_ERROR(TAG, "WiFi not connected — cannot start HTTP server");
        return;
    }

    LOG_INFO(TAG, "WiFi IP: %s", WiFi.localIP().toString().c_str());

    // Setup HTTP server
    server.on("/", HTTP_GET, handleRoot);
    server.on("/screen.jpg", HTTP_GET, handleScreenshot);
    server.on("/stats", HTTP_GET, handleStats);
    server.on("/capture", HTTP_GET, handleCapture);

    server.begin();
    LOG_INFO(TAG, "HTTP server started");
    LOG_INFO(TAG, "Open browser to http://%s/", WiFi.localIP().toString().c_str());

    startTime = millis();
}

// Update screenshot server (call from task)
void screenshot_server_update() {
    server.handleClient();
}

// Manually trigger screenshot capture
void screenshot_server_capture() {
    if (captureInProgress) {
        LOG_ERROR(TAG, "Capture already in progress");
        return;
    }

    // Pin capture to Core 1 (same as LVGL/LCD in arturo) at lowest priority.
    // Chunked framebuffer copy with yields prevents PSRAM bus contention.
    xTaskCreatePinnedToCore([](void *param) {
        vTaskDelay(pdMS_TO_TICKS(100));
        captureScreenshot();
        vTaskDelete(NULL);
    }, "WebCapture", 65536, NULL, 1, NULL, 1);
}

// Check if capture is in progress
bool screenshot_server_busy() {
    return captureInProgress;
}

// Capture screenshot to JPEG format
static void captureScreenshot() {
    captureInProgress = true;

    LOG_INFO(TAG, "Capturing screenshot...");

    const int width = LV_HOR_RES;
    const int height = LV_VER_RES;
    const size_t rgb565Size = width * height * sizeof(lv_color_t);

    lv_color_t *snapshotBuf = (lv_color_t *)heap_caps_malloc(rgb565Size, MALLOC_CAP_SPIRAM);
    if (!snapshotBuf) {
        LOG_ERROR(TAG, "Failed to allocate snapshot buffer");
        captureInProgress = false;
        return;
    }

    // Copy framebuffer in small chunks with yields between them.
    // A single 1.2MB memcpy saturates the PSRAM bus and starves the LCD RGB
    // DMA, causing visible display jitter. Chunked copy with yields gives
    // LCD DMA enough bandwidth windows to keep refreshing smoothly.
    lv_disp_t *disp = lv_disp_get_default();
    bool copyOk = false;
    if (disp && disp->driver && disp->driver->draw_buf) {
        lv_color_t *fb = (lv_color_t *)disp->driver->draw_buf->buf1;
        if (fb) {
            const size_t lineBytes = width * sizeof(lv_color_t);
            const int linesPerChunk = 20;  // ~40KB per chunk
            for (int y = 0; y < height; y += linesPerChunk) {
                int lines = ((y + linesPerChunk) <= height) ? linesPerChunk : (height - y);
                memcpy((uint8_t *)snapshotBuf + y * lineBytes,
                       (uint8_t *)fb + y * lineBytes,
                       lines * lineBytes);
                vTaskDelay(1);  // yield to let LCD DMA breathe
            }
            copyOk = true;
        }
    }

    if (!copyOk) {
        LOG_ERROR(TAG, "Frame buffer copy failed");
        heap_caps_free(snapshotBuf);
        captureInProgress = false;
        return;
    }

    LOG_DEBUG(TAG, "Frame buffer copied (chunked)");

    // Encode to JPEG
    LOG_DEBUG(TAG, "Encoding to JPEG...");

    const size_t jpegBufSize = 200 * 1024;
    uint8_t *jpegBuf = (uint8_t *)heap_caps_malloc(jpegBufSize, MALLOC_CAP_SPIRAM);
    if (!jpegBuf) {
        LOG_ERROR(TAG, "Failed to allocate JPEG buffer");
        heap_caps_free(snapshotBuf);
        captureInProgress = false;
        return;
    }

    LOG_DEBUG(TAG, "Allocated %d bytes for JPEG output", jpegBufSize);

    JPEGENC *jpeg = new JPEGENC();
    if (!jpeg) {
        LOG_ERROR(TAG, "Failed to create JPEG encoder");
        heap_caps_free(jpegBuf);
        heap_caps_free(snapshotBuf);
        captureInProgress = false;
        return;
    }

    int rc = jpeg->open(jpegBuf, jpegBufSize);
    if (rc != JPEGE_SUCCESS) {
        LOG_ERROR(TAG, "JPEG open failed: %d", rc);
        delete jpeg;
        heap_caps_free(jpegBuf);
        heap_caps_free(snapshotBuf);
        captureInProgress = false;
        return;
    }

    JPEGENCODE jpe;
    rc = jpeg->encodeBegin(&jpe, width, height, JPEGE_PIXEL_RGB565, JPEGE_SUBSAMPLE_420, JPEGE_Q_MED);
    if (rc != JPEGE_SUCCESS) {
        LOG_ERROR(TAG, "JPEG encodeBegin failed: %d", rc);
        jpeg->close();
        delete jpeg;
        heap_caps_free(jpegBuf);
        heap_caps_free(snapshotBuf);
        captureInProgress = false;
        return;
    }

    rc = jpeg->addFrame(&jpe, (uint8_t *)snapshotBuf, width * 2);
    if (rc != JPEGE_SUCCESS) {
        LOG_ERROR(TAG, "JPEG addFrame failed: %d", rc);
        jpeg->close();
        delete jpeg;
        heap_caps_free(jpegBuf);
        heap_caps_free(snapshotBuf);
        captureInProgress = false;
        return;
    }

    int finalSize = jpeg->close();
    delete jpeg;

    LOG_INFO(TAG, "JPEG encoding complete: %d bytes (%.1f KB)", finalSize, finalSize / 1024.0);

    if (xSemaphoreTake(screenshotMutex, pdMS_TO_TICKS(1000))) {
        if (screenshotBuffer) {
            heap_caps_free(screenshotBuffer);
        }

        screenshotBuffer = (uint8_t *)heap_caps_malloc(finalSize, MALLOC_CAP_SPIRAM);
        if (!screenshotBuffer) {
            LOG_ERROR(TAG, "Failed to allocate final screenshot buffer");
            xSemaphoreGive(screenshotMutex);
            heap_caps_free(jpegBuf);
            heap_caps_free(snapshotBuf);
            captureInProgress = false;
            return;
        }

        memcpy(screenshotBuffer, jpegBuf, finalSize);
        screenshotSize = finalSize;
        captureCount++;

        LOG_INFO(TAG, "Screenshot complete! Total captures: %lu", captureCount);

        xSemaphoreGive(screenshotMutex);
    }

    heap_caps_free(jpegBuf);
    heap_caps_free(snapshotBuf);

    captureInProgress = false;
}

// HTTP handler for screenshot
static void handleScreenshot() {
    LOG_DEBUG(TAG, "Screenshot request");

    if (xSemaphoreTake(screenshotMutex, pdMS_TO_TICKS(5000))) {
        if (screenshotBuffer && screenshotSize > 0) {
            LOG_DEBUG(TAG, "Sending %d bytes...", screenshotSize);

            server.sendHeader("Cache-Control", "no-cache, no-store, must-revalidate");
            server.sendHeader("Pragma", "no-cache");
            server.sendHeader("Expires", "0");
            server.setContentLength(screenshotSize);
            server.send(200, "image/jpeg", "");

            const size_t chunkSize = 16384;
            size_t bytesSent = 0;

            while (bytesSent < screenshotSize) {
                size_t remaining = screenshotSize - bytesSent;
                size_t toSend = (remaining < chunkSize) ? remaining : chunkSize;

                server.sendContent((const char*)(screenshotBuffer + bytesSent), toSend);
                bytesSent += toSend;

                yield();
            }

            LOG_DEBUG(TAG, "Screenshot sent (%d bytes)", bytesSent);
        } else {
            server.send(404, "text/plain", "No screenshot available. Click 'Capture Screenshot' first.");
        }
        xSemaphoreGive(screenshotMutex);
    } else {
        server.send(503, "text/plain", "Screenshot in progress, try again");
    }
}

// HTTP handler for stats
static void handleStats() {
    char stats[256];
    uint32_t uptime = (millis() - startTime) / 1000;

    snprintf(stats, sizeof(stats),
             "{\"captures\":%lu,\"uptime\":%lu,\"freeHeap\":%d,\"freePsram\":%d,\"screenshotSize\":%d}",
             captureCount, uptime, ESP.getFreeHeap(), ESP.getFreePsram(), screenshotSize);

    server.send(200, "application/json", stats);
}

// HTTP handler to trigger capture
static void handleCapture() {
    LOG_DEBUG(TAG, "Capture trigger request");

    if (captureInProgress) {
        server.send(503, "text/plain", "Capture already in progress");
        return;
    }

    xTaskCreatePinnedToCore([](void *param) {
        vTaskDelay(pdMS_TO_TICKS(100));
        captureScreenshot();
        vTaskDelete(NULL);
    }, "WebCapture", 65536, NULL, 1, NULL, 1);

    server.send(200, "text/plain", "Capture started - check stats for screenshot size");
}

// HTTP handler for main page
static void handleRoot() {
    LOG_DEBUG(TAG, "Root page request");

    const char html[] = R"rawliteral(
<!DOCTYPE html>
<html>
<head>
    <title>Arturo Screenshot Monitor</title>
    <style>
        body { font-family: Arial; margin: 20px; background: #222; color: #fff; }
        h1 { color: #4CAF50; }
        button { background: #4CAF50; color: white; border: none; padding: 10px 20px; margin: 5px; cursor: pointer; }
        .stats { background: #333; padding: 10px; margin-top: 10px; font-family: monospace; }
        .screen { background: #000; padding: 10px; margin-top: 10px; text-align: center; }
        img { max-width: 100%; border: 2px solid #444; }
    </style>
</head>
<body>
    <h1>Arturo Screenshot Monitor</h1>
    <button onclick="updateStats()">Refresh Stats</button>
    <button onclick="captureNew()">Capture Screenshot</button>
    <br>
    <label>
        <input type="checkbox" id="autoRefresh" onchange="toggleAutoRefresh()">
        Auto-refresh every <input type="number" id="interval" value="5" min="1" max="60" style="width:60px"> seconds
    </label>
    <div class="stats" id="stats">Loading stats...</div>
    <div class="screen">
        <p id="msg">Click 'Capture Screenshot' to begin</p>
        <img id="img" style="display:none" alt="Screenshot">
    </div>
    <script>
        function updateStats() {
            fetch('/stats')
                .then(r => r.json())
                .then(d => {
                    const h = Math.floor(d.uptime / 3600);
                    const m = Math.floor((d.uptime % 3600) / 60);
                    const s = d.uptime % 60;
                    document.getElementById('stats').innerHTML =
                        'Captures: ' + d.captures + '<br>' +
                        'Uptime: ' + h + 'h ' + m + 'm ' + s + 's<br>' +
                        'Free Heap: ' + (d.freeHeap / 1024).toFixed(1) + ' KB<br>' +
                        'Free PSRAM: ' + (d.freePsram / 1024 / 1024).toFixed(1) + ' MB<br>' +
                        'Screenshot: ' + (d.screenshotSize / 1024).toFixed(1) + ' KB';
                });
        }
        function captureNew() {
            document.getElementById('msg').textContent = 'Capturing...';
            fetch('/capture')
                .then(r => r.text())
                .then(t => {
                    document.getElementById('msg').textContent = 'Loading screenshot...';
                    setTimeout(() => {
                        const img = document.getElementById('img');
                        img.src = '/screen.jpg?t=' + new Date().getTime();
                        img.style.display = 'block';
                        document.getElementById('msg').style.display = 'none';
                        updateStats();
                    }, 2000);
                });
        }
        let autoRefreshTimer = null;
        function toggleAutoRefresh() {
            const checkbox = document.getElementById('autoRefresh');
            const interval = parseInt(document.getElementById('interval').value) * 1000;
            if (checkbox.checked) {
                autoRefreshTimer = setInterval(captureNew, interval);
                document.getElementById('msg').textContent = 'Auto-refresh enabled';
            } else {
                if (autoRefreshTimer) {
                    clearInterval(autoRefreshTimer);
                    autoRefreshTimer = null;
                }
                document.getElementById('msg').textContent = 'Auto-refresh disabled';
            }
        }
        updateStats();
        setInterval(updateStats, 5000);
    </script>
</body>
</html>
)rawliteral";

    server.send(200, "text/html", html);
    LOG_DEBUG(TAG, "Root page sent");
}

#endif // ENABLE_SCREENSHOT_SERVER
