#include <Arduino.h>
#include "messaging/envelope.h"
#include "messaging/heartbeat.h"

void setup() {
    Serial.begin(115200);
    delay(2000);

    Serial.println("=== Arturo compile check ===");

    // Build a heartbeat to verify messaging library works on ESP32
    JsonDocument doc;
    arturo::Source src = {"arturo-station", "station-01", "1.0.0"};

    const char* devices[] = {"DMM-01", nullptr};
    arturo::HeartbeatData data = {};
    data.status = "running";
    data.uptimeSeconds = 0;
    data.devices = devices;
    data.deviceCount = 1;
    data.freeHeap = (int64_t)ESP.getFreeHeap();
    data.minFreeHeap = (int64_t)ESP.getMinFreeHeap();
    data.wifiRssi = 0;
    data.firmwareVersion = "1.0.0";

    bool ok = arturo::buildHeartbeat(doc, src, "compile-check", millis() / 1000, data);

    char buffer[512];
    serializeJsonPretty(doc, buffer, sizeof(buffer));
    Serial.println(ok ? "buildHeartbeat: OK" : "buildHeartbeat: FAIL");
    Serial.println(buffer);
    Serial.printf("Free heap: %lu bytes\n", (unsigned long)ESP.getFreeHeap());
}

void loop() {
    delay(5000);
}
