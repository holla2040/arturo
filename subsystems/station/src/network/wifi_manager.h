#pragma once
#include <WiFi.h>
#include "../safety/wifi_reconnect.h"

namespace arturo {
class WifiManager {
public:
    bool connect();              // Block until connected (with backoff retries)
    bool isConnected();
    void checkAndReconnect();    // Non-blocking, call from loop()
    int rssi();
    int reconnectCount();

    // Enhanced state tracking
    WifiState state() const { return _state; }
    int failedAttempts() const { return _failedAttempts; }
    unsigned long totalDisconnectedMs() const { return _totalDisconnectedMs; }
    unsigned long longestOutageMs() const { return _longestOutageMs; }
    unsigned long lastConnectedMs() const { return _lastConnectedMs; }
    unsigned long lastDisconnectedMs() const { return _lastDisconnectedMs; }

    // Register WiFi event callbacks (call once in setup)
    void registerEvents();

    // Event callbacks (called from WiFi event system)
    void onDisconnected();
    void onConnected();

private:
    int _reconnects = 0;
    int _failedAttempts = 0;
    unsigned long _lastAttempt = 0;
    int _backoffMs = 1000;       // Doubles on each failure, max 30s
    WifiState _state = WifiState::DISCONNECTED;

    // Outage tracking
    unsigned long _lastConnectedMs = 0;
    unsigned long _lastDisconnectedMs = 0;
    unsigned long _totalDisconnectedMs = 0;
    unsigned long _longestOutageMs = 0;
    unsigned long _currentOutageStartMs = 0;
};
} // namespace arturo
