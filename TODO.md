# TODO

## Discussion: Redundant per-field queries during regen test (get_regen_status, stage temps)

**Observation.** During `onboard_regen.art`, the test-event log shows a line per script-issued `QUERY` — e.g. `query emp-001 get_regen_status -> N` every ~5 s. The script actually fires three per iteration (`get_regen_status`, `get_temp_1st_stage`, `get_temp_2nd_stage`); only one was visible in the user's excerpt.

**Two layers of redundancy.**

1. *Station side.* All three are on the station's cache-served list (`subsystems/station/src/pump_telemetry.cpp:13-23`). They don't hit the CTI UART — they're served from the station's `_pumpTelemetry` snapshot that the firmware's background poll task maintains.
2. *Controller side.* The station poller (`subsystems/internal/poller/poller.go:104`) is already fetching `get_telemetry` every tick and has `regen_char`, `stage1_temp_k`, `stage2_temp_k` in its own in-process snapshot (`poller.go:83-95`), broadcasting on the WS hub and writing to `pump_status_log` / `temperature_log`. The script is re-fetching data the poller next to it just fetched.

The individual per-field command names exist in the HAL because they read more naturally in scripts than parsing JSON out of `get_telemetry`. But every one of them maps 1:1 to a field the controller already holds.

**Design direction agreed during the discussion (before stopping):**

- **Transparent dispatch, no new script syntax.** Script authors keep writing `QUERY "get_regen_status"`. The controller's script executor internally tries sources in order of latency: (1) controller poller snapshot in-process, (2) station-side RAM cache via Redis, (3) CTI over RS-232. Same return value, only latency differs. The previously-considered `CACHED` modifier was rejected — users should not know or care about caching.
- **Staleness.** If the controller snapshot is stale (poller failing), demote to the next source rather than error. Offline detection is unchanged: the station-side cache-stale path or the heartbeat monitor still catches a dead station.
- **Scope.** Pump (CTI on-board) only in the first cut. Other device types join when they acquire a controller-side poller.

A first-draft architecture section (§5.4) and HAL note were written and then reverted during the discussion — they are recoverable from git history if the direction is confirmed (see this conversation's message thread).

**Open question — not yet decided.** What the test-event log should actually show during a sampling loop. Two candidate answers, pick one before coding:

1. *Script keeps its sample loop, uses the combo packet.* Replace the three per-iteration queries with one `QUERY "get_telemetry" tel` and pull fields out of the JSON. Needs a JSON-field accessor in the engine. One test-event line per iteration with the combo.
2. *Script stops sampling entirely; poller is the sole source.* The script queries only `regen_char` for loop-exit control. The regen-curve CSV is built from `temperature_log` + `pump_status_log` over the test's time window (which the comment at `scripts/onboard_regen.art:70-72` already says is the intent). Script's `timestamps` / `temps_1st` / `temps_2nd` / `regen_letters` / `regen_states` arrays are deleted. One test-event line per iteration — the `get_regen_status` control-flow check — and no temp queries at all.

Recommendation was (2) — matches the existing comment and eliminates duplicate bookkeeping — but user had not yet confirmed when the discussion was parked.

**If (2) is chosen, additional consideration:** the test-event log's role during sampling. Options were: a new `SAMPLE t1 t2 regen_letter state_name` primitive that emits one combined event; reuse `LOG INFO`; or suppress cache-served `query` events and emit a composite event on a timer. Not decided.

**Files touched during the discussion (for re-entry):**

- `docs/SCRIPTING_HAL.md` — Pump section intro, near the existing cache paragraph
- `docs/architecture/ARCHITECTURE.md` — §5.3/§5.4 (new section spot, after Script Executor Adaptation)
- `subsystems/internal/poller/poller.go:83-95` — `telemetrySnapshot` struct; controller-side field set
- `subsystems/station/src/pump_telemetry.cpp:13-23` — `kCachedCommands`; station-side cache-served set
- `subsystems/internal/script/executor/executor.go:624` — where `emit("query", …)` records each test event
- `scripts/onboard_regen.art:64-170` — the sampling loops (pre-regen, poll, post-regen)
- `subsystems/internal/artifact/csv.go:15-17` — comment confirming telemetry CSV is poller-owned, not script-owned

## Discussion: Move regen state description into engine/HAL

`onboard_regen.art` has a 50-line `regen_state_name()` function that maps CTI O-command letters to human-readable state names. This is device protocol knowledge that arguably belongs in the engine or HAL layer, not in user scripts.

A possible approach: add a `get_regen_step_description` command that returns the human-readable name directly, so scripts just query it instead of carrying the mapping themselves.

**Complications to consider:**

- On-Board and Standard pumps have different regen state sets. The O-command response letters and their meanings differ between the two pump types.
- Standard pumps don't respond to `get_regen_step` at all, so the abstraction needs to account for pumps that lack this capability entirely.
- Where does the mapping live? Options include: the device profile (YAML), the engine runtime, or the firmware/mock layer. Each has different trade-offs for maintainability and extensibility.
- Should the engine provide a generic "command response translation" mechanism, or is this pump-specific enough to stay in the profile layer?

## Bug: Poller "no waiter for correlation_id" messages

The controller response listener logs `no waiter for correlation_id=...` when a station response arrives after the poller has already timed out and deregistered its waiter.

**Root cause:** The poller (`internal/poller/poller.go`) sends ~9 sequential queries per pump device (status bytes, valve states, regen status, temperatures) with a 5-second per-query timeout. With 4 stations each having 1 pump, that's ~36 serial queries per poll cycle. Late responses from stations arrive after the poller has moved on, and the dispatcher (`internal/api/dispatcher.go:Dispatch`) finds no matching waiter.

**Observed timing:** Poller sends at :28, "no waiter" responses arrive at :30 and :32 (2-4 seconds after timeout).

**Impact:** Low severity. The responses are still broadcast to WebSocket clients (`hub.BroadcastEvent` in `main.go:368` runs regardless of dispatch), but the poller does not incorporate the late data into its log output. The log noise is the main annoyance.

**Possible fixes:**
- Parallelize per-station polls (each station's queries run concurrently instead of all stations serial)
- Increase the per-query timeout
- Batch multiple queries into fewer round-trips if firmware supports it
- Reduce the number of queries per poll cycle

