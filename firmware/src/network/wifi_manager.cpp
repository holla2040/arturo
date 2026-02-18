#include "wifi_manager.h"
#include "../config.h"
#include "../debug_log.h"

namespace arturo {

// Static pointer for WiFi event callback (ESP32 WiFi events are C-style callbacks)
static WifiManager* _instance = nullptr;

static void wifiEventHandler(WiFiEvent_t event) {
    if (!_instance) return;
    switch (event) {
        case ARDUINO_EVENT_WIFI_STA_DISCONNECTED:
            _instance->onDisconnected();
            break;
        case ARDUINO_EVENT_WIFI_STA_GOT_IP:
            _instance->onConnected();
            break;
        default:
            break;
    }
}

void WifiManager::registerEvents() {
    _instance = this;
    WiFi.onEvent(wifiEventHandler);
    LOG_INFO("WIFI", "Event handlers registered");
}

void WifiManager::onDisconnected() {
    if (_state == WifiState::DISCONNECTED) return;

    unsigned long now = millis();
    _state = WifiState::DISCONNECTED;
    _currentOutageStartMs = now;
    _lastDisconnectedMs = now;

    LOG_ERROR("WIFI", "DISCONNECTED (reconnects=%d)", _reconnects);
}

void WifiManager::onConnected() {
    unsigned long now = millis();
    _state = WifiState::CONNECTED;
    _lastConnectedMs = now;
    _backoffMs = BACKOFF_DEFAULT.initialMs;
    _failedAttempts = 0;

    // Track outage duration
    if (_currentOutageStartMs > 0) {
        unsigned long outageDur = outrageDuration(_currentOutageStartMs, now);
        _totalDisconnectedMs += outageDur;
        if (outageDur > _longestOutageMs) {
            _longestOutageMs = outageDur;
        }
        LOG_INFO("WIFI", "Reconnected after %lu ms outage (total=%lu ms)",
                 outageDur, _totalDisconnectedMs);
        _currentOutageStartMs = 0;
    }

    LOG_INFO("WIFI", "Connected rssi=%d", WiFi.RSSI());
}

bool WifiManager::connect() {
    LOG_INFO("WIFI", "Connecting to %s...", WIFI_SSID);

    _state = WifiState::CONNECTING;
    WiFi.mode(WIFI_STA);
    WiFi.begin(WIFI_SSID, WIFI_PASSWORD);

    int attempts = 0;
    while (WiFi.status() != WL_CONNECTED && attempts < 20) {
        delay(500);
        attempts++;
        LOG_DEBUG("WIFI", "Waiting for connection... attempt %d/20", attempts);
    }

    if (WiFi.status() == WL_CONNECTED) {
        _state = WifiState::CONNECTED;
        _backoffMs = BACKOFF_DEFAULT.initialMs;
        _failedAttempts = 0;
        _lastConnectedMs = millis();
        LOG_INFO("WIFI", "Connected rssi=%d", WiFi.RSSI());
        return true;
    }

    _state = WifiState::DISCONNECTED;
    _failedAttempts++;
    LOG_ERROR("WIFI", "Failed to connect after %d attempts (total failures=%d)",
              attempts, _failedAttempts);
    return false;
}

bool WifiManager::isConnected() {
    return WiFi.status() == WL_CONNECTED;
}

void WifiManager::checkAndReconnect() {
    if (WiFi.status() == WL_CONNECTED) {
        if (_state != WifiState::CONNECTED) {
            _state = WifiState::CONNECTED;
        }
        return;
    }

    unsigned long now = millis();
    if (!backoffReady(_lastAttempt, now, _backoffMs)) {
        return;
    }
    _lastAttempt = now;

    _reconnects++;
    _state = WifiState::CONNECTING;
    LOG_INFO("WIFI", "Reconnecting attempt %d (backoff %dms, failures=%d)...",
             _reconnects, _backoffMs, _failedAttempts);

    WiFi.disconnect();
    WiFi.begin(WIFI_SSID, WIFI_PASSWORD);

    // Non-blocking: we just initiate and check next loop iteration
    if (WiFi.status() == WL_CONNECTED) {
        _state = WifiState::CONNECTED;
        _backoffMs = BACKOFF_DEFAULT.initialMs;
        _failedAttempts = 0;
        _lastConnectedMs = now;
        LOG_INFO("WIFI", "Reconnected rssi=%d", WiFi.RSSI());
    } else {
        _failedAttempts++;
        _backoffMs = backoffNext(_backoffMs, BACKOFF_DEFAULT.multiplier, BACKOFF_DEFAULT.maxMs);
    }
}

int WifiManager::rssi() {
    return WiFi.RSSI();
}

int WifiManager::reconnectCount() {
    return _reconnects;
}

} // namespace arturo
