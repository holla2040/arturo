# TODO

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

## Bug: `onboard_acceptance.art` terminated early with "station went offline"

Running `onboard_acceptance.art` against real station-01 (CTI on-board pump at T1 ~65 K, T2 ~10 K) terminates ~18 s into Stage 1 with `Status: terminated`, `Summary: "station went offline"`. The script itself is healthy — no ASSERT fails, no script error. The controller's health checker kills the session because the station's heartbeat stops arriving for ≥ 10 s.

**Reproduction (observed twice, runs `ae987a2e-…` and `3d50e6a3-…` in DB):**
1. Start controller + terminal + Redis, station-01 online with pump at base.
2. Create RMA, start `onboard_acceptance` script on station-01 via terminal UI.
3. ~18 s later the test row appears as `terminated` with summary "station went offline".

**Evidence from test_events (`/test-runs/{id}/events`):**
- 03:22:21 — test started
- 03:22:22 — `QUERY get_regen_status -> P` OK
- 03:22:24 – 03:22:38 — three successful baseline samples (T1 ~65 K, T2 ~10 K, all asserts passed). Each T1 + T2 query pair took 0.8 s at the start, growing to **2.2 s by the third sample**.
- 03:22:39.98 — `EventType: terminated, EmployeeID: system, Reason: "station went offline"` — emitted by the controller, not the script.

**Evidence from controller stdout (around the same wall-clock window):**
- At test start, flood of `response listener: no waiter for correlation_id=<uuid>` — responses arriving for commands the caller already timed out and deregistered. 13+ in a row over ~15 s.
- Poller ticks visibly slow down; the combined `S1=a J=… K=…` summary line goes from cadence 5 s to 9 s during the flood.
- Underlying "no waiter" mechanic is the already-logged bug above, but here it escalates into a session termination.

**Call path that terminates the session:**
- `subsystems/cmd/controller/main.go:386-415` — `runHealthChecker` ticks every 2 s; calls `reg.RunHealthCheck(now)`.
- `subsystems/internal/registry/registry.go:171-195` — marks station `offline` if `now - LastHeartbeat >= OfflineThreshold` (10 s, line 20).
- `subsystems/cmd/controller/main.go:408-412` — when a station transitions to offline, calls `testMgr.HandleOffline(s.Instance)`.
- `subsystems/internal/testmanager/manager.go:243-255` — if the station has an active session, calls `session.Terminate("system", "station went offline")`. That is the `terminated` event above.

**Root-cause hypothesis:** the poller (`internal/poller/poller.go`, 5 s tick, ~6-9 sequential CTI queries per tick) and the script (baseline loop, 2 queries per 5 s) both share the station's single CTI RS-232 bus. When their queries interleave, the station's serial round-trips queue up and the station firmware's heartbeat publisher — on the same main loop — can't emit heartbeats fast enough to beat the 10 s offline threshold. The pump is still physically alive and still responding (proven by the 2.2 s T2 response), but the control-plane heartbeat starvation looks like "offline" to the controller.

This is an infrastructure issue, not a script issue. `onboard_acceptance.art` is the trigger because it's the first script that issues `QUERY` commands on top of the already-busy poller; it is not the bug.

**Fix options (in order of structural soundness):**

1. **Pause the poller while a test session is active on that station.** The script owns the bus during a run; poller resumes when the session ends. The "right" fix. Poller already sits next to `testmanager`; add a check in the poll loop (`internal/poller/poller.go`) that asks the TestManager whether the station has an active session, and skips that station's tick if so. Requires threading a TestManager reference (or a lightweight "is busy" callback) into the poller.

2. **Station firmware: move heartbeat publish off the CTI command loop.** Put the Redis heartbeat on its own FreeRTOS task / timer so serial bus congestion cannot starve it. The most architecturally correct fix, out of scope for Go-only changes. See `subsystems/station/` (C++/Arduino).

3. **Raise `OfflineThreshold`** in `subsystems/internal/registry/registry.go:20` from 10 s to ~30 s. One-line change. Masks the symptom, lets test runs survive brief bus congestion, but hides genuine disconnects for longer. Reasonable short-term mitigation if the firmware/poller fix is delayed.

4. **Increase `SAMPLE_INTERVAL` in scripts.** Reduces script-side contention but doesn't fix root cause — the poller alone could still starve heartbeats on a slower station or bigger command set. Not recommended as the primary fix.

**Recommendation:** option 1 (poller pause on active session) is the smallest, most surgical Go change and properly respects the single-source-of-truth rule — the script owns the serial bus during a test run, the poller takes over when idle. Option 3 is worth doing alongside it as defense in depth.

**Files/paths pre-identified:**
- `scripts/onboard_acceptance.art` — the script being run (unchanged; it is the trigger, not the bug)
- `subsystems/internal/poller/poller.go` — where the pause logic would live for option 1
- `subsystems/internal/testmanager/manager.go` — `HasActiveSession(stationInstance)` already exists (line ~268), so the poller can query it directly
- `subsystems/internal/registry/registry.go:20` — `OfflineThreshold` constant for option 3
- `subsystems/cmd/controller/main.go:386-415` — the health checker loop that triggers the termination
- DB at `/home/cryo/arturo/arturo.db`, test runs `ae987a2e-f752-4082-b011-a0ce8c03d804` and `3d50e6a3-9eb4-4fde-9851-8d063f85e1e6` contain the failing-run events for reference.
