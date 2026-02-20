#pragma once

// Credentials injected from .env via build flags — these are fallbacks
#ifndef WIFI_SSID
#define WIFI_SSID          "not-configured"
#endif
#ifndef WIFI_PASSWORD
#define WIFI_PASSWORD      "not-configured"
#endif
#ifndef REDIS_HOST
#define REDIS_HOST         "127.0.0.1"
#endif
#ifndef REDIS_PORT
#define REDIS_PORT         6379
#endif
#ifndef REDIS_USERNAME
#define REDIS_USERNAME     ""
#endif
#ifndef REDIS_PASSWORD
#define REDIS_PASSWORD     ""
#endif
#ifndef STATION_INSTANCE
#define STATION_INSTANCE   "station-01"
#endif

// Station identity
#define STATION_SERVICE    "arturo-station"
#define STATION_VERSION    "1.0.0"
#define FIRMWARE_VERSION   "1.0.0"

// Devices managed by this station
#define DEVICE_COUNT       1
static const char* DEVICE_IDS[] = {"PUMP-01"};

// CTI OnBoard serial port pins (UART1 via MAX3232)
#define CTI_UART_NUM       1
#define CTI_RX_PIN         17
#define CTI_TX_PIN         18

// Heartbeat (must be < registry stale threshold of 5s)
#define HEARTBEAT_INTERVAL_MS  3000
#define PRESENCE_TTL_SECONDS   90

// Redis channels (from ARCHITECTURE.md section 2.3)
#define CHANNEL_HEARTBEAT        "events:heartbeat"
#define CHANNEL_COMMANDS_PREFIX  "commands:"
#define CHANNEL_RESPONSES_PREFIX "responses:"
#define PRESENCE_KEY_PREFIX      "device:"
#define PRESENCE_KEY_SUFFIX      ":alive"

// Debug levels (compile-time, from ARCHITECTURE.md section 6.2)
#define DEBUG_LEVEL_NONE   0
#define DEBUG_LEVEL_ERROR  1
#define DEBUG_LEVEL_INFO   2
#define DEBUG_LEVEL_DEBUG  3
#define DEBUG_LEVEL_TRACE  4

#ifndef DEBUG_LEVEL
#define DEBUG_LEVEL        DEBUG_LEVEL_INFO
#endif
