#include "wifi_manager.h"
#include "../config.h"
#include "../debug_log.h"

namespace arturo {

bool WifiManager::connect() {
    LOG_INFO("WIFI", "Connecting to %s...", WIFI_SSID);

    WiFi.mode(WIFI_STA);
    WiFi.begin(WIFI_SSID, WIFI_PASSWORD);

    int attempts = 0;
    while (WiFi.status() != WL_CONNECTED && attempts < 20) {
        delay(500);
        attempts++;
        LOG_DEBUG("WIFI", "Waiting for connection... attempt %d/20", attempts);
    }

    if (WiFi.status() == WL_CONNECTED) {
        LOG_INFO("WIFI", "Connected rssi=%d", WiFi.RSSI());
        _backoffMs = 1000;
        return true;
    }

    LOG_ERROR("WIFI", "Failed to connect after %d attempts", attempts);
    return false;
}

bool WifiManager::isConnected() {
    return WiFi.status() == WL_CONNECTED;
}

void WifiManager::checkAndReconnect() {
    if (WiFi.status() == WL_CONNECTED) {
        return;
    }

    unsigned long now = millis();
    if (now - _lastAttempt < (unsigned long)_backoffMs) {
        return;
    }
    _lastAttempt = now;

    _reconnects++;
    LOG_INFO("WIFI", "DISCONNECTED — Reconnecting attempt %d (backoff %dms)...", _reconnects, _backoffMs);

    WiFi.disconnect();
    WiFi.begin(WIFI_SSID, WIFI_PASSWORD);

    // Non-blocking: we just initiate and check next loop iteration
    // If connected, reset backoff on next call
    if (WiFi.status() == WL_CONNECTED) {
        LOG_INFO("WIFI", "Reconnected rssi=%d", WiFi.RSSI());
        _backoffMs = 1000;
    } else {
        _backoffMs = (_backoffMs * 2 > 30000) ? 30000 : _backoffMs * 2;
    }
}

int WifiManager::rssi() {
    return WiFi.RSSI();
}

int WifiManager::reconnectCount() {
    return _reconnects;
}

} // namespace arturo
