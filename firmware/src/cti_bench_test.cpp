// Standalone CTI bench test — no WiFi, no Redis, just serial.
// Flash with: cd firmware && pio run -e cti_test -t upload
// Monitor with: pio device monitor --baud 115200
//
// Sends A? every 3 seconds and prints raw TX/RX bytes.

#ifdef CTI_BENCH_TEST

#include <Arduino.h>
#include "protocols/cti.h"

static const uint8_t RX_PIN = 17;
static const uint8_t TX_PIN = 18;

HardwareSerial CtiSerial(1);

void hexDump(const char* label, const uint8_t* data, size_t len) {
    Serial.printf("  %s (%zu bytes): ", label, len);
    for (size_t i = 0; i < len; i++) {
        Serial.printf("%02X ", data[i]);
    }
    Serial.print(" | ");
    for (size_t i = 0; i < len; i++) {
        char c = (char)data[i];
        Serial.print((c >= 0x20 && c < 0x7F) ? c : '.');
    }
    Serial.println();
}

void sendAndPrint(const char* ctiCmd) {
    char frame[64];
    int frameLen = arturo::ctiBuildFrame(ctiCmd, frame, sizeof(frame));
    if (frameLen < 0) {
        Serial.printf("ERROR: ctiBuildFrame failed for '%s'\n", ctiCmd);
        return;
    }

    Serial.printf("\n--- %s ---\n", ctiCmd);
    hexDump("TX", (const uint8_t*)frame, frameLen);

    // Drain stale RX
    while (CtiSerial.available()) CtiSerial.read();

    // Send
    CtiSerial.write((const uint8_t*)frame, frameLen);
    CtiSerial.flush();

    // Receive (up to 1000ms, stop at \r)
    char rxBuf[128];
    size_t pos = 0;
    unsigned long start = millis();

    while (millis() - start < 1000 && pos < sizeof(rxBuf) - 1) {
        if (CtiSerial.available()) {
            char c = CtiSerial.read();
            rxBuf[pos++] = c;
            if (c == '\r') break;
        }
    }
    rxBuf[pos] = '\0';

    if (pos == 0) {
        Serial.println("  RX: ** TIMEOUT — no response **");
        return;
    }

    hexDump("RX", (const uint8_t*)rxBuf, pos);

    // Parse
    arturo::CtiResponse resp;
    if (arturo::ctiParseFrame(rxBuf, pos, resp)) {
        Serial.printf("  Code: %c  Data: '%s'  Checksum: %s\n",
                       (char)resp.code, resp.data,
                       resp.checksumValid ? "OK" : "FAIL");
    } else {
        Serial.println("  Parse: FAILED (bad frame structure)");
    }
}

void setup() {
    Serial.begin(115200);
    delay(2000);

    Serial.println();
    Serial.println("================================");
    Serial.println("  CTI Bench Test");
    Serial.println("  UART1: 2400 7E1");
    Serial.printf("  Pins: RX=%d TX=%d\n", RX_PIN, TX_PIN);
    Serial.println("================================");
    Serial.println();

    CtiSerial.begin(2400, SERIAL_7E1, RX_PIN, TX_PIN);
    CtiSerial.setTimeout(100);

    Serial.println("Serial1 ready. Sending A? in 2 seconds...");
    delay(2000);
}

void loop() {
    sendAndPrint("A?");
    delay(3000);
}

#endif // CTI_BENCH_TEST
