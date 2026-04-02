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
