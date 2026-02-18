# System OTA Request Schema v1.0.0

## Overview

| Property | Value |
|----------|-------|
| Version | v1.0.0 |
| Format | JSON |
| Message Type | `system.ota.request` |
| Transport | Redis Stream |
| Channel | `commands:{instance-id}` (per-station stream) |
| Direction | Controller -> Station |
| Status | Active |

Request to update firmware on a station via OTA (Over-The-Air). The controller publishes this to the station's command stream. The station downloads the firmware binary over HTTP, writes it to the inactive OTA partition, verifies the SHA256 checksum, and reboots.

OTA uses the ESP-IDF dual-partition system called from Arduino code. If the new firmware fails to connect to Redis within 30 seconds of boot, the bootloader automatically rolls back to the previous partition.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Transport | Redis Stream (not Pub/Sub) | OTA is a targeted command to a specific station. Must be reliably delivered. |
| Binary delivery | HTTP download | Station pulls the binary from the controller over HTTP. Simpler than chunking through Redis. |
| Verification | SHA256 checksum | Validates binary integrity after download. Station rejects mismatched checksums. |
| Rollback | Automatic (bootloader) | ESP-IDF's built-in rollback. No custom rollback logic needed. |
| Version check | Semver comparison | Skip update if already running the target version (unless `force: true`). |

## JSON Schema Definition

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://github.com/holla2040/arturo/schemas/v1.0.0/system-ota-request.json",
  "title": "System OTA Request",
  "description": "Request to update firmware on a station via OTA.",
  "type": "object",
  "required": ["envelope", "payload"],
  "additionalProperties": false,
  "properties": {
    "envelope": {
      "$ref": "../envelope/schema-definition.md#envelope",
      "properties": {
        "type": { "const": "system.ota.request" }
      },
      "required": ["id", "timestamp", "source", "schema_version", "type", "correlation_id", "reply_to"]
    },
    "payload": {
      "type": "object",
      "required": ["firmware_url", "version", "sha256"],
      "additionalProperties": false,
      "properties": {
        "firmware_url": {
          "type": "string",
          "description": "HTTP URL to download the firmware binary.",
          "format": "uri",
          "pattern": "^https?://",
          "maxLength": 512
        },
        "version": {
          "type": "string",
          "description": "Target firmware version. Semver format.",
          "pattern": "^[0-9]+\\.[0-9]+\\.[0-9]+$"
        },
        "sha256": {
          "type": "string",
          "description": "SHA256 hex digest of the firmware binary.",
          "pattern": "^[0-9a-f]{64}$"
        },
        "force": {
          "type": "boolean",
          "description": "If true, skip version check and install regardless.",
          "default": false
        }
      }
    }
  }
}
```

## Field Descriptions

### Envelope Fields (Required for this type)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `correlation_id` | string | Yes | UUIDv4 linking this request to the OTA response. Controller tracks OTA progress by this ID. |
| `reply_to` | string | Yes | Redis Stream for the station to report OTA result. |

### Payload Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `firmware_url` | string | Yes | -- | HTTP URL to the firmware `.bin` file. The station downloads this over WiFi. Must be reachable on the LAN. |
| `version` | string | Yes | -- | Target firmware version in semver format (e.g., `1.1.0`). Station compares this against its running version. |
| `sha256` | string | Yes | -- | SHA256 hex digest (64 characters) of the firmware binary. Station verifies the download matches before flashing. |
| `force` | boolean | No | `false` | If `true`, skip version comparison and flash regardless. Useful for rollbacks or re-flashing the same version. |

## OTA Update Flow

```
1. Controller:  XADD commands:dmm-station-01 * message <ota-request-json>
2. Station:     Receives OTA request via XREAD
3. Station:     Compare version against running firmware
4.              If same version and force=false -> respond with success (no-op)
5. Station:     HTTP GET firmware_url -> download .bin to inactive partition
6. Station:     Calculate SHA256 of downloaded binary
7.              If SHA256 mismatch -> respond with E_VALIDATION_FAILED
8. Station:     Mark inactive partition as boot target
9. Station:     Respond with success (pre-reboot)
10. Station:    Reboot
11. Station:    Boot from new partition
12. Station:    Connect to WiFi + Redis within 30 seconds
13.             If connection fails -> bootloader rolls back automatically
14. Station:    Publish heartbeat with new firmware_version
15. Controller: Detects version change in heartbeat -> OTA confirmed
```

## OTA Response

The station responds using a standard `device.command.response` message with `command_name: "ota_update"`:

### Success (Pre-Reboot)

```json
{
  "payload": {
    "device_id": "dmm-station-01",
    "command_name": "ota_update",
    "success": true,
    "response": "OTA verified, rebooting to v1.1.0",
    "duration_ms": 12500
  }
}
```

### Failure (Checksum Mismatch)

```json
{
  "payload": {
    "device_id": "dmm-station-01",
    "command_name": "ota_update",
    "success": false,
    "error": {
      "code": "E_VALIDATION_FAILED",
      "message": "SHA256 mismatch after download",
      "details": {
        "expected": "abc123...",
        "actual": "def456..."
      }
    },
    "duration_ms": 15200
  }
}
```

## Firmware Binary Hosting

The controller hosts firmware binaries over HTTP:

```
http://192.168.1.10:8080/firmware/arturo-tcp-bridge-v1.1.0.bin
http://192.168.1.10:8080/firmware/arturo-relay-controller-v1.1.0.bin
```

File naming convention: `arturo-{variant}-v{version}.bin`

| Variant | Description |
|---------|-------------|
| `tcp-bridge` | TCP/SCPI instrument bridge station |
| `serial-bridge` | Serial instrument bridge station |
| `relay-controller` | GPIO relay controller station |
| `estop` | Emergency stop station |

## Implementation Details

### Station Firmware (C++)

```cpp
#include <esp_ota_ops.h>
#include <esp_partition.h>
#include <esp_https_ota.h>

bool performOTA(const char* firmwareUrl, const char* expectedSha256, const char* targetVersion) {
    // 1. Version check
    if (!force && strcmp(targetVersion, FIRMWARE_VERSION) == 0) {
        return true;  // Already running this version
    }

    // 2. Get next OTA partition
    const esp_partition_t* update = esp_ota_get_next_update_partition(NULL);
    if (!update) return false;

    // 3. Begin OTA
    esp_ota_handle_t otaHandle;
    esp_err_t err = esp_ota_begin(update, OTA_SIZE_UNKNOWN, &otaHandle);
    if (err != ESP_OK) return false;

    // 4. Download and write chunks
    HTTPClient http;
    http.begin(firmwareUrl);
    int httpCode = http.GET();
    if (httpCode != HTTP_CODE_OK) return false;

    WiFiClient* stream = http.getStreamPtr();
    uint8_t buf[1024];
    int bytesRead;
    mbedtls_sha256_context sha256ctx;
    mbedtls_sha256_init(&sha256ctx);
    mbedtls_sha256_starts(&sha256ctx, 0);

    while ((bytesRead = stream->readBytes(buf, sizeof(buf))) > 0) {
        esp_ota_write(otaHandle, buf, bytesRead);
        mbedtls_sha256_update(&sha256ctx, buf, bytesRead);
    }

    // 5. Verify SHA256
    uint8_t hash[32];
    mbedtls_sha256_finish(&sha256ctx, hash);
    char hashHex[65];
    for (int i = 0; i < 32; i++) sprintf(hashHex + i*2, "%02x", hash[i]);
    if (strcmp(hashHex, expectedSha256) != 0) {
        esp_ota_abort(otaHandle);
        return false;
    }

    // 6. Finalize and set boot partition
    esp_ota_end(otaHandle);
    esp_ota_set_boot_partition(update);
    return true;  // Caller reboots after sending response
}
```

### Controller (Go)

```go
// Trigger OTA update on a specific station
func (c *Controller) RequestOTA(stationID, firmwareURL, version, sha256 string, force bool) (string, error) {
    correlationID := uuid.New().String()
    msg := map[string]interface{}{
        "envelope": map[string]interface{}{
            "id":             uuid.New().String(),
            "timestamp":      time.Now().Unix(),
            "source":         c.source,
            "schema_version": "v1.0.0",
            "type":           "system.ota.request",
            "correlation_id": correlationID,
            "reply_to":       fmt.Sprintf("responses:controller:%s", c.instanceID),
        },
        "payload": map[string]interface{}{
            "firmware_url": firmwareURL,
            "version":      version,
            "sha256":       sha256,
            "force":        force,
        },
    }
    streamKey := fmt.Sprintf("commands:%s", stationID)
    c.redis.XAdd(ctx, &redis.XAddArgs{Stream: streamKey, Values: map[string]interface{}{"message": marshal(msg)}})
    return correlationID, nil
}
```

## Version History

### v1.0.0 (Current)
- Initial OTA request definition
- HTTP-based binary download
- SHA256 verification before flashing
- ESP-IDF dual-partition with automatic rollback
- Force flag for version override
