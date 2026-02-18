#pragma once
#include <cstdint>

namespace arturo {

// Boot reason categories (mapped from ESP32 reset reasons)
enum class BootReason : uint8_t {
    POWER_ON     = 0,  // Normal power-on or EN pin reset
    SOFTWARE     = 1,  // Software reset (esp_restart)
    WATCHDOG     = 2,  // Watchdog timer reset (task or interrupt WDT)
    BROWNOUT     = 3,  // Supply voltage dropped below threshold
    PANIC        = 4,  // Software panic / unhandled exception
    DEEP_SLEEP   = 5,  // Wake from deep sleep
    UNKNOWN      = 6   // Unknown or unrecognized reason
};

// Check if a boot reason indicates an abnormal shutdown
// (watchdog, brownout, panic = abnormal; power_on, software, deep_sleep = normal)
bool isAbnormalBoot(BootReason reason);

// Check if a boot reason indicates a power-related issue
// (brownout or power_on after previous unexpected state)
bool isPowerRelatedBoot(BootReason reason);

// Get human-readable string for boot reason
const char* bootReasonToString(BootReason reason);

// Safe-state verification flags
// Each bit represents a subsystem that has been verified safe after boot
struct SafeStateFlags {
    bool relaysOff;        // All relay channels set to OFF
    bool outputsSafe;      // All GPIO outputs in safe state
    bool watchdogInit;     // Watchdog timer initialized
    bool estopChecked;     // E-stop state read and handled
};

// Check if all safe-state flags are set
bool allSafeStatesVerified(const SafeStateFlags& flags);

// Count how many safe-state checks have passed
int safeStateCount(const SafeStateFlags& flags);

// Total number of safe-state checks
static const int SAFE_STATE_TOTAL = 4;

// NVS recovery context — stored/restored across power cycles
struct RecoveryContext {
    uint32_t magic;             // Magic number to validate stored data (0xART0)
    uint32_t bootCount;         // Number of boots since first install
    uint32_t abnormalBootCount; // Number of abnormal boots
    BootReason lastBootReason;  // Reason for last boot
    uint32_t lastUptimeSeconds; // How long the station ran before last reset
    char lastTestId[32];        // ID of test in progress when power was lost (if any)
    bool testWasRunning;        // Whether a test was active at shutdown
};

static const uint32_t RECOVERY_MAGIC = 0x41525430; // "ART0"

// Validate a recovery context by checking the magic number
bool isValidRecoveryContext(const RecoveryContext& ctx);

// Initialize a fresh recovery context (first boot / corrupted data)
void initRecoveryContext(RecoveryContext& ctx);

// Update recovery context after detecting boot reason
void updateRecoveryContextOnBoot(RecoveryContext& ctx, BootReason reason);

#ifdef ARDUINO

// Detect the boot reason from ESP32 reset reason register
BootReason detectBootReason();

// NVS-based recovery context persistence
class RecoveryStore {
public:
    // Load recovery context from NVS. Returns false if no valid context found.
    bool load(RecoveryContext& ctx);

    // Save recovery context to NVS.
    bool save(const RecoveryContext& ctx);

    // Clear stored recovery context.
    bool clear();

    // Save current test ID (called when test starts)
    bool saveActiveTest(const char* testId);

    // Clear active test (called when test completes normally)
    bool clearActiveTest();
};

// Perform safe-state initialization on boot
// Returns flags indicating which subsystems have been verified safe
SafeStateFlags performSafeStateInit();

#endif

} // namespace arturo
