#include "ota_update.h"
#include <cstdlib>
#include <cstdio>
#include <cctype>

#ifdef ARDUINO
#include <esp_ota_ops.h>
#include <esp_partition.h>
#include <esp_http_client.h>
#include <mbedtls/sha256.h>
#include "../debug_log.h"
#include "../config.h"
#endif

namespace arturo {

bool parseOTAPayload(const char* firmwareUrl, const char* version,
                     const char* sha256, bool force, OTARequest& req) {
    if (firmwareUrl == nullptr || version == nullptr || sha256 == nullptr) {
        return false;
    }

    if (strlen(firmwareUrl) == 0 || strlen(firmwareUrl) >= sizeof(req.firmwareUrl)) {
        return false;
    }
    if (strlen(version) == 0 || strlen(version) >= sizeof(req.version)) {
        return false;
    }
    if (strlen(sha256) == 0 || strlen(sha256) >= sizeof(req.sha256)) {
        return false;
    }

    strncpy(req.firmwareUrl, firmwareUrl, sizeof(req.firmwareUrl) - 1);
    req.firmwareUrl[sizeof(req.firmwareUrl) - 1] = '\0';
    strncpy(req.version, version, sizeof(req.version) - 1);
    req.version[sizeof(req.version) - 1] = '\0';
    strncpy(req.sha256, sha256, sizeof(req.sha256) - 1);
    req.sha256[sizeof(req.sha256) - 1] = '\0';
    req.force = force;

    return true;
}

bool validateOTARequest(const OTARequest& req, OTAError& error) {
    if (!isValidFirmwareURL(req.firmwareUrl)) {
        error = OTAError::INVALID_URL;
        return false;
    }
    if (!isValidSemver(req.version)) {
        error = OTAError::INVALID_VERSION;
        return false;
    }
    if (!isValidSHA256Hex(req.sha256)) {
        error = OTAError::INVALID_SHA256;
        return false;
    }
    error = OTAError::NONE;
    return true;
}

int compareSemver(const char* a, const char* b) {
    if (a == nullptr || b == nullptr) return 0;

    int aMajor = 0, aMinor = 0, aPatch = 0;
    int bMajor = 0, bMinor = 0, bPatch = 0;

    sscanf(a, "%d.%d.%d", &aMajor, &aMinor, &aPatch);
    sscanf(b, "%d.%d.%d", &bMajor, &bMinor, &bPatch);

    if (aMajor != bMajor) return aMajor - bMajor;
    if (aMinor != bMinor) return aMinor - bMinor;
    return aPatch - bPatch;
}

bool isValidSemver(const char* version) {
    if (version == nullptr) return false;

    int major = -1, minor = -1, patch = -1;
    char extra = '\0';

    int parsed = sscanf(version, "%d.%d.%d%c", &major, &minor, &patch, &extra);

    // Must parse exactly 3 integers with no trailing characters
    if (parsed != 3) return false;
    if (major < 0 || minor < 0 || patch < 0) return false;

    return true;
}

bool isValidSHA256Hex(const char* hash) {
    if (hash == nullptr) return false;

    size_t len = strlen(hash);
    if (len != 64) return false;

    for (size_t i = 0; i < 64; i++) {
        char c = hash[i];
        if (!((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'))) {
            return false;
        }
    }
    return true;
}

bool isValidFirmwareURL(const char* url) {
    if (url == nullptr) return false;

    size_t len = strlen(url);
    if (len < 8) return false; // shortest: http://x

    if (strncmp(url, "http://", 7) == 0) return true;
    if (strncmp(url, "https://", 8) == 0) return true;

    return false;
}

const char* otaStateToString(OTAState state) {
    switch (state) {
        case OTAState::IDLE:        return "idle";
        case OTAState::CHECKING:    return "checking";
        case OTAState::DOWNLOADING: return "downloading";
        case OTAState::VERIFYING:   return "verifying";
        case OTAState::APPLYING:    return "applying";
        case OTAState::REBOOTING:   return "rebooting";
        case OTAState::FAILED:      return "failed";
        default:                    return "unknown";
    }
}

const char* otaErrorToString(OTAError error) {
    switch (error) {
        case OTAError::NONE:               return "none";
        case OTAError::INVALID_URL:        return "invalid_url";
        case OTAError::INVALID_VERSION:    return "invalid_version";
        case OTAError::INVALID_SHA256:     return "invalid_sha256";
        case OTAError::SAME_VERSION:       return "same_version";
        case OTAError::DOWNLOAD_FAILED:    return "download_failed";
        case OTAError::CHECKSUM_MISMATCH:  return "checksum_mismatch";
        case OTAError::FLASH_WRITE_FAILED: return "flash_write_failed";
        case OTAError::ROLLBACK_ACTIVE:    return "rollback_active";
        case OTAError::INSUFFICIENT_SPACE: return "insufficient_space";
        case OTAError::BUSY:               return "busy";
        default:                           return "unknown";
    }
}

#ifdef ARDUINO

OTAUpdateHandler::OTAUpdateHandler()
    : _state(OTAState::IDLE)
    , _lastError(OTAError::NONE)
    , _request{}
    , _progress(0)
    , _updateCount(0)
    , _failCount(0) {}

bool OTAUpdateHandler::startUpdate(const OTARequest& req, const char* currentVersion) {
    if (_state != OTAState::IDLE && _state != OTAState::FAILED) {
        _lastError = OTAError::BUSY;
        LOG_ERROR("OTA", "Update already in progress (state=%s)", otaStateToString(_state));
        return false;
    }

    _state = OTAState::CHECKING;
    _lastError = OTAError::NONE;
    _progress = 0;
    _request = req;

    // Validate request parameters
    OTAError validationError;
    if (!validateOTARequest(req, validationError)) {
        _state = OTAState::FAILED;
        _lastError = validationError;
        _failCount++;
        LOG_ERROR("OTA", "Validation failed: %s", otaErrorToString(validationError));
        return false;
    }

    // Version check (skip if force=true)
    if (!req.force && currentVersion != nullptr) {
        int cmp = compareSemver(currentVersion, req.version);
        if (cmp == 0) {
            _state = OTAState::FAILED;
            _lastError = OTAError::SAME_VERSION;
            _failCount++;
            LOG_ERROR("OTA", "Already running version %s", currentVersion);
            return false;
        }
    }

    LOG_INFO("OTA", "Starting update: %s -> %s (force=%d)",
             currentVersion ? currentVersion : "unknown",
             req.version, req.force ? 1 : 0);

    // Perform the actual download and flash
    if (!performDownloadAndFlash()) {
        _failCount++;
        return false;
    }

    _updateCount++;
    _state = OTAState::REBOOTING;
    LOG_INFO("OTA", "Update complete, rebooting...");

    // Reboot into new firmware
    esp_restart();

    return true; // Never reached
}

void OTAUpdateHandler::cancel() {
    if (_state == OTAState::DOWNLOADING || _state == OTAState::CHECKING) {
        LOG_INFO("OTA", "Update cancelled");
        _state = OTAState::IDLE;
        _lastError = OTAError::NONE;
        _progress = 0;
    }
}

bool OTAUpdateHandler::performDownloadAndFlash() {
    _state = OTAState::DOWNLOADING;

    // Get the next OTA partition
    const esp_partition_t* updatePartition = esp_ota_get_next_update_partition(NULL);
    if (updatePartition == NULL) {
        _state = OTAState::FAILED;
        _lastError = OTAError::INSUFFICIENT_SPACE;
        LOG_ERROR("OTA", "No OTA partition available");
        return false;
    }

    LOG_INFO("OTA", "Writing to partition: %s (offset 0x%lx, size %lu)",
             updatePartition->label,
             (unsigned long)updatePartition->address,
             (unsigned long)updatePartition->size);

    // Begin OTA update
    esp_ota_handle_t otaHandle;
    esp_err_t err = esp_ota_begin(updatePartition, OTA_SIZE_UNKNOWN, &otaHandle);
    if (err != ESP_OK) {
        _state = OTAState::FAILED;
        _lastError = OTAError::FLASH_WRITE_FAILED;
        LOG_ERROR("OTA", "esp_ota_begin failed: %d", err);
        return false;
    }

    // Download firmware via HTTP
    esp_http_client_config_t httpConfig = {};
    httpConfig.url = _request.firmwareUrl;
    httpConfig.timeout_ms = 30000;

    esp_http_client_handle_t client = esp_http_client_init(&httpConfig);
    if (client == NULL) {
        esp_ota_abort(otaHandle);
        _state = OTAState::FAILED;
        _lastError = OTAError::DOWNLOAD_FAILED;
        LOG_ERROR("OTA", "HTTP client init failed");
        return false;
    }

    err = esp_http_client_open(client, 0);
    if (err != ESP_OK) {
        esp_http_client_cleanup(client);
        esp_ota_abort(otaHandle);
        _state = OTAState::FAILED;
        _lastError = OTAError::DOWNLOAD_FAILED;
        LOG_ERROR("OTA", "HTTP open failed: %d", err);
        return false;
    }

    int contentLength = esp_http_client_fetch_headers(client);
    if (contentLength <= 0) {
        esp_http_client_cleanup(client);
        esp_ota_abort(otaHandle);
        _state = OTAState::FAILED;
        _lastError = OTAError::DOWNLOAD_FAILED;
        LOG_ERROR("OTA", "Invalid content length: %d", contentLength);
        return false;
    }

    // Initialize SHA256 context for verification
    mbedtls_sha256_context sha256Ctx;
    mbedtls_sha256_init(&sha256Ctx);
    mbedtls_sha256_starts(&sha256Ctx, 0); // 0 = SHA256 (not SHA224)

    // Download and write in chunks
    uint8_t buf[1024];
    int totalRead = 0;
    int readLen;

    while ((readLen = esp_http_client_read(client, (char*)buf, sizeof(buf))) > 0) {
        // Update SHA256
        mbedtls_sha256_update(&sha256Ctx, buf, readLen);

        // Write to flash
        err = esp_ota_write(otaHandle, buf, readLen);
        if (err != ESP_OK) {
            mbedtls_sha256_free(&sha256Ctx);
            esp_http_client_cleanup(client);
            esp_ota_abort(otaHandle);
            _state = OTAState::FAILED;
            _lastError = OTAError::FLASH_WRITE_FAILED;
            LOG_ERROR("OTA", "esp_ota_write failed at offset %d: %d", totalRead, err);
            return false;
        }

        totalRead += readLen;
        _progress = (contentLength > 0) ? (totalRead * 100 / contentLength) : 0;
        LOG_DEBUG("OTA", "Progress: %d%% (%d/%d bytes)", _progress, totalRead, contentLength);
    }

    esp_http_client_cleanup(client);

    if (totalRead == 0) {
        mbedtls_sha256_free(&sha256Ctx);
        esp_ota_abort(otaHandle);
        _state = OTAState::FAILED;
        _lastError = OTAError::DOWNLOAD_FAILED;
        LOG_ERROR("OTA", "Downloaded 0 bytes");
        return false;
    }

    // Verify SHA256
    _state = OTAState::VERIFYING;
    uint8_t sha256Hash[32];
    mbedtls_sha256_finish(&sha256Ctx, sha256Hash);
    mbedtls_sha256_free(&sha256Ctx);

    // Convert hash to hex string
    char computedHex[65];
    for (int i = 0; i < 32; i++) {
        snprintf(computedHex + (i * 2), 3, "%02x", sha256Hash[i]);
    }

    if (strcmp(computedHex, _request.sha256) != 0) {
        esp_ota_abort(otaHandle);
        _state = OTAState::FAILED;
        _lastError = OTAError::CHECKSUM_MISMATCH;
        LOG_ERROR("OTA", "SHA256 mismatch: expected=%s computed=%s", _request.sha256, computedHex);
        return false;
    }

    LOG_INFO("OTA", "SHA256 verified: %s (%d bytes)", computedHex, totalRead);

    // Finalize OTA
    _state = OTAState::APPLYING;
    err = esp_ota_end(otaHandle);
    if (err != ESP_OK) {
        _state = OTAState::FAILED;
        _lastError = OTAError::FLASH_WRITE_FAILED;
        LOG_ERROR("OTA", "esp_ota_end failed: %d", err);
        return false;
    }

    // Set the new partition as boot partition
    err = esp_ota_set_boot_partition(updatePartition);
    if (err != ESP_OK) {
        _state = OTAState::FAILED;
        _lastError = OTAError::FLASH_WRITE_FAILED;
        LOG_ERROR("OTA", "esp_ota_set_boot_partition failed: %d", err);
        return false;
    }

    _progress = 100;
    return true;
}

bool OTAUpdateHandler::verifySHA256(const uint8_t* data, size_t len, const char* expectedHex) {
    mbedtls_sha256_context ctx;
    mbedtls_sha256_init(&ctx);
    mbedtls_sha256_starts(&ctx, 0);
    mbedtls_sha256_update(&ctx, data, len);

    uint8_t hash[32];
    mbedtls_sha256_finish(&ctx, hash);
    mbedtls_sha256_free(&ctx);

    char computedHex[65];
    for (int i = 0; i < 32; i++) {
        snprintf(computedHex + (i * 2), 3, "%02x", hash[i]);
    }

    return strcmp(computedHex, expectedHex) == 0;
}

#endif

} // namespace arturo
