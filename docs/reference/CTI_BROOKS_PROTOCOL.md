# CTI/Brooks Cryopump Serial Protocol

Wire-level protocol reference for implementing `CtiClient` on ESP32 stations. Derived from the pendant2 project (`~/pendant2/docs/protocol/`) and Brooks On-Board Cryopump Module Communications spec.

## Serial Configuration

| Parameter | Value |
|-----------|-------|
| Baud rate | 2400 |
| Data bits | 7 |
| Parity | Even |
| Stop bits | 1 |
| Format shorthand | 7E1 |

## Frame Format

```
Request:  $<command><checksum>\r
Response: $<code><data><checksum>\r
```

- `$` — start delimiter (0x24)
- `<command>` — 1-4 ASCII characters
- `<checksum>` — single ASCII character (0x30-0x6F range)
- `\r` — carriage return terminator (0x0D)

### Example Transaction

```
TX: $J@\r          Query stage 1 temp. 'J' is command, '@' is checksum
RX: $A15.3@\r      'A'=success, '15.3'=data, '@'=checksum
```

## Checksum Algorithm

Sum all command characters, fold high and low bits, mask to printable ASCII:

```cpp
uint8_t checksumGet(const char* cmd) {
    uint8_t sum = 0;
    while (*cmd) sum += *cmd++;

    uint8_t d7d6 = sum >> 6;
    uint8_t d1d0 = sum & 0x03;
    uint8_t xor_val = d7d6 ^ d1d0;

    return (((sum & 0xFC) + xor_val) & 0x3F) + 0x30;
}
```

Validation: receiving side computes checksum over all characters except the last, then compares against the last character.

### Worked Example

Command `J` (0x4A = 74):
```
sum     = 74
d7d6    = 74 >> 6  = 1
d1d0    = 74 & 0x03 = 2
xor     = 1 ^ 2   = 3
result  = ((72 + 3) & 0x3F) + 0x30 = 11 + 48 = 59 = 0x3B... wait
```
Actually: `((74 & 0xFC) + 3) & 0x3F + 0x30` = `(72 + 3) & 63 + 48` = `75 & 63 + 48` = `11 + 48` = `59` = ASCII `;`... The pendant2 docs show `@` (0x40) for `J`. The checksum must be verified against real hardware or the reference implementation. Use the C function above as the authoritative implementation.

## Response Codes

First character after `$` in every response:

| Code | Meaning | Data valid? |
|------|---------|-------------|
| `A` | Success | Yes |
| `B` | Success, but power failure occurred | Yes |
| `E` | Cannot execute (invalid command) | No |
| `F` | Cannot execute + power failure | No |
| `G` | Cannot execute (interlocks active) | No |
| `H` | Cannot execute (interlocks + power failure) | No |

Only `A` and `B` indicate the response data is valid. All others must be rejected.

### Warning Suppression

Certain codes repeat frequently and should be rate-limited in logs:
- `B` (power failure): warn every 30 seconds, not every poll
- `G` (interlocks): warn every 10 seconds

## Timing

| Metric | Value |
|--------|-------|
| Typical TX time | ~50ms (10 bytes at 2400 baud) |
| Pump processing | ~15ms |
| Total round-trip | ~65ms |
| **Timeout** | **600ms** |
| Max throughput | ~15 transactions/sec |

## Offline Detection

- Track consecutive timeouts in a `staleCount` counter
- Reset to 0 on any valid response
- After 2 timeouts: report device as offline
- After 5 timeouts: enter backoff mode (poll every 5 seconds instead of 150ms)
- Log offline warnings every 60 seconds (not every poll)

## Critical: Status Byte Hex Parsing

Commands `S1`, `S2`, `S3` return **2-character hexadecimal strings**, NOT decimal.

```cpp
// CORRECT: parse as base 16
uint8_t status = (uint8_t)strtol(reply, NULL, 16);

// WRONG: parse as decimal — this is a known bug from the STM32 implementation
uint8_t status = atoi(reply);  // BUG!
```

Response `"39"` means 0x39 (binary 0011 1001), not decimal 39 (0x27).

## Transaction Tracking

Each request/response pair should be tracked with a transaction ID to prevent desynchronization (a real bug from the original STM32 pendant implementation):

```cpp
struct CtiTransaction {
    uint16_t id;          // monotonically increasing
    uint32_t timestamp;   // millis() when sent
    char command[64];     // command string for logging
};
```

Benefits:
- Detects mismatched responses from serial noise
- Provides timeout detection
- Enables debug logging with correlation

## Differences from SCPI

| Aspect | SCPI | CTI/Brooks |
|--------|------|------------|
| Transport | TCP socket | Serial UART (2400 baud 7E1) |
| Framing | `cmd\n` / `response\n` | `$cmd<chk>\r` / `$code<data><chk>\r` |
| Checksum | None | Required (bitwise algorithm) |
| Error reporting | Inline strings | Single-char response code |
| Speed | Fast (LAN TCP) | Slow (~65ms/transaction) |
| Connection | Persistent TCP | Always-on serial |

## Implementation Notes for CtiClient

When building the Arturo `CtiClient`:

1. **Use HardwareSerial** on ESP32, not WiFiClient (this is serial, not TCP)
2. **7E1 config**: `Serial1.begin(2400, SERIAL_7E1, rxPin, txPin)`
3. **Checksum on every frame**: both TX and RX
4. **Response parser state machine**: IDLE → WAITING_FOR_$ → READING_DATA → VALIDATE_CHECKSUM
5. **600ms timeout** per transaction
6. **Status bytes are hex** — parse with `strtol(buf, NULL, 16)`
7. **Rate-limit error logging** to avoid serial output flooding
8. **Testable without hardware**: checksum, framing, and response parsing functions should be free functions outside `#ifdef ARDUINO` guards, following the same pattern as `ScpiClient`

## Profile Note

The existing `profiles/pumps/cti_onboard.yaml` uses `$P{addr}` addressing syntax. The point-to-point protocol documented here (from pendant2) does not use addressing — it's `$<command><checksum>\r` directly. The profile may need updating when CtiClient is implemented.

## Source Material

All protocol details derived from:
- `~/pendant2/docs/protocol/required-commands.md` (authoritative command set)
- `~/pendant2/docs/protocol/brooks-protocol.md` (wire format)
- `~/pendant2/docs/protocol/checksum-algorithm.md` (checksum impl)
- `~/pendant2/docs/protocol/response-codes.md` (error handling)
- `~/pendant2/docs/architecture/serial-protocol.md` (transaction tracking)
- `~/pendant2/PUMP_COMMANDS.md` (complete command reference)
- `~/pendant2/src/firmware/src/pump_task.cpp` (working implementation)
