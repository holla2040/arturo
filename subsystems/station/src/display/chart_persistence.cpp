#ifdef ARDUINO

#include "chart_persistence.h"
#include "../debug_log.h"
#include <esp_littlefs.h>
#include <cstdio>
#include <cstring>

namespace arturo {

const char* ChartPersistence::FILE_PATH = "/littlefs/chart.bin";

bool ChartPersistence::init() {
    esp_vfs_littlefs_conf_t conf = {};
    conf.base_path = "/littlefs";
    conf.partition_label = "spiffs";  // Partition label in partition table
    conf.format_if_mount_failed = true;
    conf.dont_mount = false;
    conf.grow_on_mount = true;

    esp_err_t err = esp_vfs_littlefs_register(&conf);
    if (err != ESP_OK) {
        LOG_ERROR("CHART", "LittleFS mount failed: %s", esp_err_to_name(err));
        return false;
    }

    _mounted = true;
    LOG_INFO("CHART", "LittleFS mounted on /littlefs");
    return true;
}

bool ChartPersistence::save(const ChartDataPoint* points, int count, int writeIndex) {
    if (!_mounted || points == nullptr || count <= 0) return false;
    if (count > CHART_MAX_POINTS) count = CHART_MAX_POINTS;

    FILE* f = fopen(FILE_PATH, "wb");
    if (!f) {
        LOG_ERROR("CHART", "Failed to open %s for writing", FILE_PATH);
        return false;
    }

    ChartFileHeader header = {};
    header.magic = CHART_FILE_MAGIC;
    header.pointCount = (uint16_t)count;
    header.writeIndex = (uint16_t)writeIndex;

    size_t written = fwrite(&header, sizeof(header), 1, f);
    if (written != 1) {
        LOG_ERROR("CHART", "Failed to write header");
        fclose(f);
        return false;
    }

    written = fwrite(points, sizeof(ChartDataPoint), count, f);
    fclose(f);

    if ((int)written != count) {
        LOG_ERROR("CHART", "Failed to write points: %d/%d", (int)written, count);
        return false;
    }

    LOG_INFO("CHART", "Saved %d chart points (writeIndex=%d)", count, writeIndex);
    return true;
}

bool ChartPersistence::load(ChartDataPoint* points, int maxCount, int& count, int& writeIndex) {
    count = 0;
    writeIndex = 0;
    if (!_mounted || points == nullptr) return false;

    FILE* f = fopen(FILE_PATH, "rb");
    if (!f) {
        LOG_INFO("CHART", "No chart data file found — starting fresh");
        return false;
    }

    ChartFileHeader header = {};
    size_t rd = fread(&header, sizeof(header), 1, f);
    if (rd != 1 || header.magic != CHART_FILE_MAGIC) {
        LOG_ERROR("CHART", "Invalid chart file header");
        fclose(f);
        return false;
    }

    int toRead = (int)header.pointCount;
    if (toRead > maxCount) toRead = maxCount;
    if (toRead > CHART_MAX_POINTS) toRead = CHART_MAX_POINTS;

    rd = fread(points, sizeof(ChartDataPoint), toRead, f);
    fclose(f);

    if ((int)rd != toRead) {
        LOG_ERROR("CHART", "Partial read: %d/%d points", (int)rd, toRead);
        return false;
    }

    count = toRead;
    writeIndex = (int)header.writeIndex;
    if (writeIndex > count) writeIndex = count % CHART_MAX_POINTS;

    LOG_INFO("CHART", "Loaded %d chart points (writeIndex=%d)", count, writeIndex);
    return true;
}

} // namespace arturo

#endif
