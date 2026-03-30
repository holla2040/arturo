#include "interlock.h"
#include <cstring>

#ifdef ARDUINO
#include "../debug_log.h"
#endif

namespace arturo {

InterlockStatus interlockEvaluate(float value, const InterlockThreshold& threshold) {
    // Check fault limits first (most severe)
    if (value <= threshold.faultLow || value >= threshold.faultHigh) {
        return InterlockStatus::FAULT;
    }
    // Check warning limits
    if (value <= threshold.warningLow || value >= threshold.warningHigh) {
        return InterlockStatus::WARNING;
    }
    return InterlockStatus::OK;
}

bool interlockIsActionable(InterlockStatus status) {
    return status == InterlockStatus::WARNING || status == InterlockStatus::FAULT;
}

#ifdef ARDUINO
InterlockManager::InterlockManager() : _numChecks(0) {
    memset(_checks, 0, sizeof(_checks));
    memset(_results, 0, sizeof(_results));
}

int InterlockManager::addCheck(const char* name, InterlockType type,
                               const InterlockThreshold& threshold) {
    if (_numChecks >= INTERLOCK_MAX_CHECKS) {
        LOG_ERROR("INTERLOCK", "Cannot add check '%s': max %d reached",
                  name, INTERLOCK_MAX_CHECKS);
        return -1;
    }

    int idx = _numChecks;
    _checks[idx].name = name;
    _checks[idx].type = type;
    _checks[idx].threshold = threshold;

    _results[idx].name = name;
    _results[idx].type = type;
    _results[idx].status = InterlockStatus::OK;
    _results[idx].value = 0;

    _numChecks++;

    LOG_INFO("INTERLOCK", "Added check '%s': warn=[%.1f,%.1f] fault=[%.1f,%.1f]",
             name, threshold.warningLow, threshold.warningHigh,
             threshold.faultLow, threshold.faultHigh);
    return idx;
}

InterlockResult InterlockManager::evaluate(int index, float value) {
    InterlockResult result = {nullptr, InterlockType::CUSTOM, InterlockStatus::FAULT, 0};

    if (index < 0 || index >= _numChecks) {
        LOG_ERROR("INTERLOCK", "Invalid check index: %d", index);
        return result;
    }

    InterlockStatus status = interlockEvaluate(value, _checks[index].threshold);

    _results[index].status = status;
    _results[index].value = value;

    if (status == InterlockStatus::FAULT) {
        LOG_ERROR("INTERLOCK", "FAULT: %s = %.2f", _checks[index].name, value);
    } else if (status == InterlockStatus::WARNING) {
        LOG_INFO("INTERLOCK", "WARNING: %s = %.2f", _checks[index].name, value);
    }

    return _results[index];
}

bool InterlockManager::hasFault() const {
    for (int i = 0; i < _numChecks; i++) {
        if (_results[i].status == InterlockStatus::FAULT) return true;
    }
    return false;
}

bool InterlockManager::hasWarning() const {
    for (int i = 0; i < _numChecks; i++) {
        if (interlockIsActionable(_results[i].status)) return true;
    }
    return false;
}

InterlockStatus InterlockManager::worstStatus() const {
    InterlockStatus worst = InterlockStatus::OK;
    for (int i = 0; i < _numChecks; i++) {
        if ((uint8_t)_results[i].status > (uint8_t)worst) {
            worst = _results[i].status;
        }
    }
    return worst;
}

int InterlockManager::getResults(InterlockResult* results, int maxResults) const {
    int count = (_numChecks < maxResults) ? _numChecks : maxResults;
    memcpy(results, _results, count * sizeof(InterlockResult));
    return count;
}
#endif

} // namespace arturo
