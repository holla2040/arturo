#include <Arduino.h>
#include "station.h"

arturo::Station station;

void setup() {
    Serial.begin(115200);
    delay(2000);
    station.begin();
}

void loop() {
    station.loop();
}
