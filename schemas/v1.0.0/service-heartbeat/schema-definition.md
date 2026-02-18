# Service Heartbeat Schema v1.0.0

## Overview

| Property | Value |
|----------|-------|
| Version | v1.0.0 |
| Format | JSON |
| Message Type | `service.heartbeat` |
| Transport | Redis Pub/Sub |
| Channel | `events:heartbeat` |
| Direction | ESP32 -> Server |
| Interval | Every 30 seconds |
| Status | Active |

Periodic health report published by each ESP32 node. The server uses heartbeats to track which nodes are alive, monitor hardware health, and detect failures. If heartbeats stop arriving, the server marks the node as unreachable.

Each ESP32 also maintains a Redis presence key (`device:{instance}:alive`) with a 90-second TTL, refreshed with each heartbeat. This provides a simple liveness check without Pub/Sub.

## JSON Schema Definition

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://github.com/holla2040/arturo/schemas/v1.0.0/service-heartbeat.json",
  "title": "Service Heartbeat",
  "description": "Periodic health report from an ESP32 node. Published via Redis Pub/Sub.",
  "type": "object",
  "required": ["envelope", "payload"],
  "additionalProperties": false,
  "properties": {
    "envelope": {
      "$ref": "../envelope/schema-definition.md#envelope",
      "properties": {
        "type": { "const": "service.heartbeat" }
      },
      "required": ["id", "timestamp", "source", "schema_version", "type"]
    },
    "payload": {
      "type": "object",
      "required": ["status", "uptime_seconds", "devices", "free_heap", "wifi_rssi", "firmware_version"],
      "additionalProperties": false,
      "properties": {
        "status": {
          "type": "string",
          "description": "Current node status.",
          "enum": ["starting", "running", "degraded", "stopping"]
        },
        "uptime_seconds": {
          "type": "integer",
          "description": "Seconds since boot.",
          "minimum": 0
        },
        "devices": {
          "type": "array",
          "description": "List of connected device IDs.",
          "items": {
            "type": "string",
            "pattern": "^[a-zA-Z0-9][a-zA-Z0-9_-]*$"
          }
        },
        "free_heap": {
          "type": "integer",
          "description": "Current free heap memory in bytes.",
          "minimum": 0
        },
        "min_free_heap": {
          "type": "integer",
          "description": "Lowest free heap since boot (high-water mark). Indicates memory pressure.",
          "minimum": 0
        },
        "wifi_rssi": {
          "type": "integer",
          "description": "WiFi signal strength in dBm. Typical range: -30 (excellent) to -90 (poor).",
          "minimum": -127,
          "maximum": 0
        },
        "wifi_reconnects": {
          "type": "integer",
          "description": "Number of WiFi reconnections since boot.",
          "minimum": 0,
          "default": 0
        },
        "redis_reconnects": {
          "type": "integer",
          "description": "Number of Redis reconnections since boot.",
          "minimum": 0,
          "default": 0
        },
        "commands_processed": {
          "type": "integer",
          "description": "Total commands executed since boot.",
          "minimum": 0,
          "default": 0
        },
        "commands_failed": {
          "type": "integer",
          "description": "Total commands that returned errors since boot.",
          "minimum": 0,
          "default": 0
        },
        "last_error": {
          "type": ["string", "null"],
          "description": "Most recent error message, or null if no errors.",
          "maxLength": 256
        },
        "watchdog_resets": {
          "type": "integer",
          "description": "Number of watchdog-triggered resets since last clean boot.",
          "minimum": 0,
          "default": 0
        },
        "firmware_version": {
          "type": "string",
          "description": "Currently running firmware version. Semver format.",
          "pattern": "^[0-9]+\\.[0-9]+\\.[0-9]+$"
        }
      }
    }
  }
}
```

## Field Descriptions

### Envelope Fields

No `correlation_id` or `reply_to` -- heartbeats are fire-and-forget over Pub/Sub.

### Payload Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `status` | string | Yes | -- | Node status: `starting` (boot sequence), `running` (normal), `degraded` (device errors), `stopping` (shutdown). |
| `uptime_seconds` | integer | Yes | -- | Seconds since boot. Resets on watchdog restart or OTA reboot. |
| `devices` | array | Yes | -- | List of device IDs connected to this node (e.g., `["fluke-8846a"]`). Empty array if no devices. |
| `free_heap` | integer | Yes | -- | Current free heap memory in bytes. ESP32-S3 starts with ~360KB free after initialization. |
| `min_free_heap` | integer | No | -- | Lowest free heap value since boot. Tracks memory leaks. If this drops below ~50KB, investigate. |
| `wifi_rssi` | integer | Yes | -- | WiFi signal strength in dBm. Below -80 dBm may cause packet loss. |
| `wifi_reconnects` | integer | No | `0` | WiFi reconnection count. Non-zero indicates WiFi instability. |
| `redis_reconnects` | integer | No | `0` | Redis reconnection count. Non-zero indicates network or server issues. |
| `commands_processed` | integer | No | `0` | Lifetime command counter. Useful for throughput monitoring. |
| `commands_failed` | integer | No | `0` | Failed command counter. High ratio to processed indicates device problems. |
| `last_error` | string/null | No | `null` | Most recent error for quick triage without reading logs. |
| `watchdog_resets` | integer | No | `0` | Watchdog reset count. Non-zero means firmware crashed or hung. |
| `firmware_version` | string | Yes | -- | Running firmware version. Used by OTA to determine if update is needed. |

## Node Status Values

| Status | Meaning | Typical Duration |
|--------|---------|-----------------|
| `starting` | Boot sequence in progress. WiFi/Redis connecting. | 5-15 seconds |
| `running` | Normal operation. All devices connected. | Indefinite |
| `degraded` | One or more devices have errors. Commands still accepted. | Until device recovers |
| `stopping` | Graceful shutdown in progress. No new commands accepted. | 1-5 seconds |

## Presence Key

In addition to Pub/Sub heartbeats, each ESP32 maintains a Redis key for simple liveness checks:

```
SET device:{instance}:alive "1" EX 90
```

- Key: `device:dmm-station-01:alive`
- TTL: 90 seconds (3x heartbeat interval)
- If the key expires, the node is considered dead
- Refreshed with every heartbeat publication

The server can check liveness with a simple `EXISTS` command without subscribing to Pub/Sub.

## First Heartbeat

The first heartbeat after boot serves as a startup announcement. The server should:

1. Register the node as alive
2. Record the firmware version
3. Note the device list
4. Begin monitoring for subsequent heartbeats

If the first heartbeat has `status: "starting"`, the server should wait for a `status: "running"` heartbeat before sending commands.

## Implementation Details

### ESP32 Firmware (C++)

```cpp
void heartbeatTask(void* param) {
    for (;;) {
        StaticJsonDocument<768> doc;
        JsonObject envelope = doc.createNestedObject("envelope");
        envelope["id"] = generateUUID();
        envelope["timestamp"] = getISO8601Timestamp();
        JsonObject source = envelope.createNestedObject("source");
        source["service"] = SERVICE_NAME;
        source["instance"] = INSTANCE_ID;
        source["version"] = FIRMWARE_VERSION;
        envelope["schema_version"] = "v1.0.0";
        envelope["type"] = "service.heartbeat";

        JsonObject payload = doc.createNestedObject("payload");
        payload["status"] = getNodeStatus();
        payload["uptime_seconds"] = (uint32_t)(millis() / 1000);
        JsonArray devices = payload.createNestedArray("devices");
        for (int i = 0; i < deviceCount; i++) {
            devices.add(deviceIDs[i]);
        }
        payload["free_heap"] = ESP.getFreeHeap();
        payload["min_free_heap"] = ESP.getMinFreeHeap();
        payload["wifi_rssi"] = WiFi.RSSI();
        payload["wifi_reconnects"] = wifiReconnectCount;
        payload["redis_reconnects"] = redisReconnectCount;
        payload["commands_processed"] = commandsProcessed;
        payload["commands_failed"] = commandsFailed;
        payload["last_error"] = lastError;
        payload["watchdog_resets"] = watchdogResets;
        payload["firmware_version"] = FIRMWARE_VERSION;

        char json[768];
        serializeJson(doc, json, sizeof(json));
        redisPublish("events:heartbeat", json);

        // Refresh presence key
        redisCommand("SET device:%s:alive 1 EX 90", INSTANCE_ID);

        vTaskDelay(pdMS_TO_TICKS(30000));
    }
}
```

### Go Server (Heartbeat Monitoring)

```go
// Subscribe and monitor heartbeats
func (s *Server) MonitorHeartbeats() {
    sub := s.redis.Subscribe(ctx, "events:heartbeat")
    for msg := range sub.Channel() {
        var hb HeartbeatMessage
        json.Unmarshal([]byte(msg.Payload), &hb)

        instance := hb.Envelope.Source.Instance
        s.nodes[instance] = NodeState{
            LastSeen:  time.Now(),
            Status:    hb.Payload.Status,
            Devices:   hb.Payload.Devices,
            FreeHeap:  hb.Payload.FreeHeap,
            RSSI:      hb.Payload.WifiRSSI,
            Version:   hb.Payload.FirmwareVersion,
        }
    }
}
```

## Version History

### v1.0.0 (Current)
- Initial heartbeat definition
- 30-second interval with 90-second presence key TTL
- ESP32 hardware diagnostics: heap, RSSI, uptime, counters
- Four node status values: starting, running, degraded, stopping
