#pragma once
#include <cstdint>
#include <cstring>

namespace arturo {

// OTA update states
enum class OTAState : uint8_t {
    IDLE      = 0,  // No update in progress
    CHECKING  = 1,  // Validating request parameters
    DOWNLOADING = 2, // Downloading firmware binary
    VERIFYING = 3,  // Verifying SHA256 checksum
    APPLYING  = 4,  // Writing to flash partition
    REBOOTING = 5,  // About to reboot into new firmware
    FAILED    = 6   // Update failed, rolled back
};

// OTA error codes
enum class OTAError : uint8_t {
    NONE               = 0,
    INVALID_URL        = 1,
    INVALID_VERSION    = 2,
    INVALID_SHA256     = 3,
    SAME_VERSION       = 4,  // Already running this version (not forced)
    DOWNLOAD_FAILED    = 5,
    CHECKSUM_MISMATCH  = 6,
    FLASH_WRITE_FAILED = 7,
    ROLLBACK_ACTIVE    = 8,  // Previous update pending verification
    INSUFFICIENT_SPACE = 9,
    BUSY               = 10  // Update already in progress
};

// OTA request parameters (parsed from system.ota.request)
struct OTARequest {
    char firmwareUrl[512];
    char version[32];
    char sha256[65];  // 64 hex chars + null
    bool force;
};

// Parse an OTA request from JSON payload fields
// Returns true if all required fields are present and valid
bool parseOTAPayload(const char* firmwareUrl, const char* version,
                     const char* sha256, bool force, OTARequest& req);

// Validate OTA request parameters (URL format, version format, SHA256 format)
bool validateOTARequest(const OTARequest& req, OTAError& error);

// Compare semver strings: returns <0 if a<b, 0 if a==b, >0 if a>b
int compareSemver(const char* a, const char* b);

// Check if a version string is valid semver (X.Y.Z)
bool isValidSemver(const char* version);

// Validate SHA256 hex string (64 lowercase hex characters)
bool isValidSHA256Hex(const char* hash);

// Check if URL starts with http:// or https://
bool isValidFirmwareURL(const char* url);

// Get human-readable string for OTA state
const char* otaStateToString(OTAState state);

// Get human-readable string for OTA error
const char* otaErrorToString(OTAError error);

#ifdef ARDUINO

// OTA update handler — manages the full update lifecycle
class OTAUpdateHandler {
public:
    OTAUpdateHandler();

    // Process an OTA request. Returns true if update was started.
    // currentVersion is compared against requested version unless force=true.
    bool startUpdate(const OTARequest& req, const char* currentVersion);

    // Cancel an in-progress update
    void cancel();

    // Current state
    OTAState state() const { return _state; }
    OTAError lastError() const { return _lastError; }
    int progress() const { return _progress; } // 0-100
    const char* targetVersion() const { return _request.version; }

    // Statistics
    int updateCount() const { return _updateCount; }
    int failCount() const { return _failCount; }

private:
    OTAState _state;
    OTAError _lastError;
    OTARequest _request;
    int _progress;
    int _updateCount;
    int _failCount;

    bool performDownloadAndFlash();
    bool verifySHA256(const uint8_t* data, size_t len, const char* expectedHex);
};

#endif

} // namespace arturo
