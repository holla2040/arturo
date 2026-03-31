#pragma once

#include <cstdint>

namespace arturo {

struct ChartDataPoint {
    uint32_t timestamp;     // millis() when sampled
    float stage1TempK;
    float stage2TempK;
};

static const uint32_t CHART_FILE_MAGIC = 0x43485254;  // "CHRT"
static const int CHART_MAX_POINTS = 200;

struct ChartFileHeader {
    uint32_t magic;
    uint16_t pointCount;
    uint16_t writeIndex;    // Next write position (circular)
};

#ifdef ARDUINO

class ChartPersistence {
public:
    bool init();
    bool save(const ChartDataPoint* points, int count, int writeIndex);
    bool load(ChartDataPoint* points, int maxCount, int& count, int& writeIndex);

private:
    bool _mounted = false;
    static const char* FILE_PATH;
};

#endif

} // namespace arturo
