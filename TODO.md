# TODO

## Discussion: Move regen state description into engine/HAL

`onboard_regen.art` has a 50-line `regen_state_name()` function that maps CTI O-command letters to human-readable state names. This is device protocol knowledge that arguably belongs in the engine or HAL layer, not in user scripts.

A possible approach: add a `get_regen_step_description` command that returns the human-readable name directly, so scripts just query it instead of carrying the mapping themselves.

**Complications to consider:**

- On-Board and Standard pumps have different regen state sets. The O-command response letters and their meanings differ between the two pump types.
- Standard pumps don't respond to `get_regen_step` at all, so the abstraction needs to account for pumps that lack this capability entirely.
- Where does the mapping live? Options include: the device profile (YAML), the engine runtime, or the firmware/mock layer. Each has different trade-offs for maintainability and extensibility.
- Should the engine provide a generic "command response translation" mechanism, or is this pump-specific enough to stay in the profile layer?
