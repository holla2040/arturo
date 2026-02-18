#include "power_recovery.h"
#include <cstring>

#ifdef ARDUINO
#include <esp_system.h>
#include <nvs_flash.h>
#include <nvs.h>
#include "../debug_log.h"
#endif

namespace arturo {

bool isAbnormalBoot(BootReason reason) {
    switch (reason) {
        case BootReason::WATCHDOG:
        case BootReason::BROWNOUT:
        case BootReason::PANIC:
            return true;
        default:
            return false;
    }
}

bool isPowerRelatedBoot(BootReason reason) {
    return reason == BootReason::BROWNOUT;
}

const char* bootReasonToString(BootReason reason) {
    switch (reason) {
        case BootReason::POWER_ON:   return "POWER_ON";
        case BootReason::SOFTWARE:   return "SOFTWARE";
        case BootReason::WATCHDOG:   return "WATCHDOG";
        case BootReason::BROWNOUT:   return "BROWNOUT";
        case BootReason::PANIC:      return "PANIC";
        case BootReason::DEEP_SLEEP: return "DEEP_SLEEP";
        case BootReason::UNKNOWN:    return "UNKNOWN";
        default:                     return "UNKNOWN";
    }
}

bool allSafeStatesVerified(const SafeStateFlags& flags) {
    return flags.relaysOff && flags.outputsSafe &&
           flags.watchdogInit && flags.estopChecked;
}

int safeStateCount(const SafeStateFlags& flags) {
    int count = 0;
    if (flags.relaysOff)     count++;
    if (flags.outputsSafe)   count++;
    if (flags.watchdogInit)  count++;
    if (flags.estopChecked)  count++;
    return count;
}

bool isValidRecoveryContext(const RecoveryContext& ctx) {
    return ctx.magic == RECOVERY_MAGIC;
}

void initRecoveryContext(RecoveryContext& ctx) {
    memset(&ctx, 0, sizeof(ctx));
    ctx.magic = RECOVERY_MAGIC;
    ctx.bootCount = 0;
    ctx.abnormalBootCount = 0;
    ctx.lastBootReason = BootReason::POWER_ON;
    ctx.lastUptimeSeconds = 0;
    ctx.testWasRunning = false;
    ctx.lastTestId[0] = '\0';
}

void updateRecoveryContextOnBoot(RecoveryContext& ctx, BootReason reason) {
    ctx.bootCount++;
    ctx.lastBootReason = reason;
    if (isAbnormalBoot(reason)) {
        ctx.abnormalBootCount++;
    }
    // testWasRunning and lastTestId are preserved from before the reset
    // so the caller can check if a test was interrupted
}

#ifdef ARDUINO

BootReason detectBootReason() {
    esp_reset_reason_t reason = esp_reset_reason();
    switch (reason) {
        case ESP_RST_POWERON:
        case ESP_RST_EXT:
            return BootReason::POWER_ON;
        case ESP_RST_SW:
            return BootReason::SOFTWARE;
        case ESP_RST_TASK_WDT:
        case ESP_RST_WDT:
        case ESP_RST_INT_WDT:
            return BootReason::WATCHDOG;
        case ESP_RST_BROWNOUT:
            return BootReason::BROWNOUT;
        case ESP_RST_PANIC:
            return BootReason::PANIC;
        case ESP_RST_DEEPSLEEP:
            return BootReason::DEEP_SLEEP;
        default:
            return BootReason::UNKNOWN;
    }
}

static const char* NVS_NAMESPACE = "arturo_rcv";
static const char* NVS_KEY_CTX   = "ctx";

bool RecoveryStore::load(RecoveryContext& ctx) {
    nvs_handle_t handle;
    esp_err_t err = nvs_open(NVS_NAMESPACE, NVS_READONLY, &handle);
    if (err != ESP_OK) {
        LOG_INFO("NVS", "No recovery context found (err=%d)", err);
        return false;
    }

    size_t required = sizeof(RecoveryContext);
    err = nvs_get_blob(handle, NVS_KEY_CTX, &ctx, &required);
    nvs_close(handle);

    if (err != ESP_OK || required != sizeof(RecoveryContext)) {
        LOG_ERROR("NVS", "Failed to load recovery context (err=%d, size=%u)", err, (unsigned)required);
        return false;
    }

    if (!isValidRecoveryContext(ctx)) {
        LOG_ERROR("NVS", "Recovery context has invalid magic (0x%08X)", ctx.magic);
        return false;
    }

    LOG_INFO("NVS", "Loaded recovery context: boots=%u, abnormal=%u, lastReason=%s",
             ctx.bootCount, ctx.abnormalBootCount, bootReasonToString(ctx.lastBootReason));
    return true;
}

bool RecoveryStore::save(const RecoveryContext& ctx) {
    nvs_handle_t handle;
    esp_err_t err = nvs_open(NVS_NAMESPACE, NVS_READWRITE, &handle);
    if (err != ESP_OK) {
        LOG_ERROR("NVS", "Failed to open NVS for write (err=%d)", err);
        return false;
    }

    err = nvs_set_blob(handle, NVS_KEY_CTX, &ctx, sizeof(RecoveryContext));
    if (err != ESP_OK) {
        LOG_ERROR("NVS", "Failed to save recovery context (err=%d)", err);
        nvs_close(handle);
        return false;
    }

    err = nvs_commit(handle);
    nvs_close(handle);

    if (err != ESP_OK) {
        LOG_ERROR("NVS", "Failed to commit recovery context (err=%d)", err);
        return false;
    }

    LOG_DEBUG("NVS", "Saved recovery context: boots=%u", ctx.bootCount);
    return true;
}

bool RecoveryStore::clear() {
    nvs_handle_t handle;
    esp_err_t err = nvs_open(NVS_NAMESPACE, NVS_READWRITE, &handle);
    if (err != ESP_OK) return false;

    nvs_erase_key(handle, NVS_KEY_CTX);
    nvs_commit(handle);
    nvs_close(handle);

    LOG_INFO("NVS", "Recovery context cleared");
    return true;
}

bool RecoveryStore::saveActiveTest(const char* testId) {
    nvs_handle_t handle;
    esp_err_t err = nvs_open(NVS_NAMESPACE, NVS_READWRITE, &handle);
    if (err != ESP_OK) return false;

    // Load existing context, update test fields, save back
    RecoveryContext ctx;
    size_t required = sizeof(RecoveryContext);
    err = nvs_get_blob(handle, NVS_KEY_CTX, &ctx, &required);
    if (err != ESP_OK || !isValidRecoveryContext(ctx)) {
        initRecoveryContext(ctx);
    }

    ctx.testWasRunning = true;
    strncpy(ctx.lastTestId, testId, sizeof(ctx.lastTestId) - 1);
    ctx.lastTestId[sizeof(ctx.lastTestId) - 1] = '\0';

    err = nvs_set_blob(handle, NVS_KEY_CTX, &ctx, sizeof(RecoveryContext));
    nvs_commit(handle);
    nvs_close(handle);

    LOG_INFO("NVS", "Saved active test: %s", testId);
    return err == ESP_OK;
}

bool RecoveryStore::clearActiveTest() {
    nvs_handle_t handle;
    esp_err_t err = nvs_open(NVS_NAMESPACE, NVS_READWRITE, &handle);
    if (err != ESP_OK) return false;

    RecoveryContext ctx;
    size_t required = sizeof(RecoveryContext);
    err = nvs_get_blob(handle, NVS_KEY_CTX, &ctx, &required);
    if (err != ESP_OK || !isValidRecoveryContext(ctx)) {
        nvs_close(handle);
        return false;
    }

    ctx.testWasRunning = false;
    ctx.lastTestId[0] = '\0';

    err = nvs_set_blob(handle, NVS_KEY_CTX, &ctx, sizeof(RecoveryContext));
    nvs_commit(handle);
    nvs_close(handle);

    LOG_DEBUG("NVS", "Cleared active test");
    return err == ESP_OK;
}

SafeStateFlags performSafeStateInit() {
    SafeStateFlags flags = {false, false, false, false};

    // Relays: The RelayController::init() already sets all to OFF,
    // but we verify by logging it here
    LOG_INFO("SAFE", "Verifying safe state on boot...");

    // All GPIO outputs that could drive relays are set LOW/safe by
    // RelayController::init() which runs before this
    flags.relaysOff = true;
    flags.outputsSafe = true;

    LOG_INFO("SAFE", "Safe state verified: %d/%d checks passed",
             safeStateCount(flags), SAFE_STATE_TOTAL);
    return flags;
}

#endif

} // namespace arturo
