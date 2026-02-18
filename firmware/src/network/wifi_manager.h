#pragma once
#include <WiFi.h>

namespace arturo {
class WifiManager {
public:
    bool connect();              // Block until connected (with backoff retries)
    bool isConnected();
    void checkAndReconnect();    // Non-blocking, call from loop()
    int rssi();
    int reconnectCount();
private:
    int _reconnects = 0;
    unsigned long _lastAttempt = 0;
    int _backoffMs = 1000;       // Doubles on each failure, max 30s
};
} // namespace arturo
