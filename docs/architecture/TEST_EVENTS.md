# Test Events Log

The **Test Events** log is the operator-facing record of what happened during a single test run. It backs the `test_events` SQLite table and the station-detail "Test Events" panel in the terminal UI.

## Scope

The log records **lifecycle transitions and notable state changes** — not script-level command traffic.

- Raw per-command SEND/QUERY history lives in the `measurements` table (`RecordCommand`), not here.
- Continuous telemetry lives in `temperature_samples` and `pump_status_log`.
- `test_events` is the signal layer on top of those: events an operator actually wants to see.

## Allowed `event_type` values

| event_type | Written by | When | `reason` contents |
|---|---|---|---|
| `started` | `testmanager/session.go` | Test session begins | Script + RMA description |
| `paused` | `testmanager/session.go` | Operator pauses the run | Employee ID in `employee_id` column; reason blank |
| `resumed` | `testmanager/session.go` | Operator resumes the run | Employee ID; reason blank |
| `terminated` | `testmanager/session.go` | Operator stops the run | Stop reason text |
| `aborted` | `testmanager/session.go` | Run aborted programmatically (e-stop, script error) | Abort reason text |
| `completed` | `testmanager/session.go` | Run finishes normally | Summary: `N tests, M passed, K failed` |
| `regen_state` | `testmanager/temp_monitor.go` | Regeneration state character changes | `regen=<char> (<phase_name>) • 1st=<NN.N>K • 2nd=<NN.N>K • elapsed=<duration>` |

## `regen_state` details

The temperature monitor polls `get_telemetry` every 5 s during a test run and tracks the `regen_char` field. On each change (including the first reading), it writes one `regen_state` row.

- **Phase names** come from `subsystems/internal/mockpump/pump.go` `RegenPhase.String()`, which is documented there as matching real CTI hardware CSV output. The canonical char → name mapping:

  | char | phase name |
  |---|---|
  | `A` | `off` (pump off — poller convention; see `poller.go:121`) |
  | `^` | `warmup 1` |
  | `C` | `warmup 2` |
  | `]` | `warmup 3` |
  | `E` | `warmup 4` |
  | `J` | `rough 1` |
  | `T` | `rough 2` |
  | `L` | `rate of rise` |
  | `N` | `cooldown` |
  | `[` | `zero tc` |
  | `P` | `complete` |
  | `V` | `aborted` |
  | anything else | `unknown` |

- **Elapsed format** matches the terminal's detail-page elapsed display: `m:ss` under one hour, `h:mm:ss` above.

## Non-goals

The following are **explicitly not** recorded in `test_events`:

- `query` — per-statement QUERY traces. Use `measurements` if you need them.
- `send` — per-statement SEND traces. Use `measurements`.
- Per-tick polling rows. State-change only.
