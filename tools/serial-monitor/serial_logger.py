#!/usr/bin/env python3
"""
Persistent serial logger for Arturo station debugging.

Connects to /dev/ttyACM0, logs all output to /tmp/arturo-serial.log,
prints to stdout, and UDP-broadcasts every line on port 8888.

Usage:
    python3 serial_logger.py                    # default /dev/ttyACM0
    python3 serial_logger.py /dev/ttyACM1       # different port

Log file: /tmp/arturo-serial.log (last 5000 lines kept)

Listen to UDP broadcast (survives logger restarts):
    socat -u UDP4-RECV:8888,broadcast,reuseaddr -

Both the operator terminal and Claude can read the log:
    tail -f /tmp/arturo-serial.log              # live follow
    tail -100 /tmp/arturo-serial.log            # last 100 lines
"""

import sys
import os
import time
import signal
import socket
import serial
from datetime import datetime
from collections import deque

PORT = sys.argv[1] if len(sys.argv) > 1 else "/dev/ttyACM0"
BAUD = 115200
LOG_FILE = "/tmp/arturo-serial.log"
MAX_LINES = 5000
RECONNECT_INTERVAL = 1.0  # seconds between reconnect attempts
UDP_PORT = 8888

# Ring buffer for log rotation
log_buffer = deque(maxlen=MAX_LINES)
running = True

# UDP broadcast socket (fire-and-forget, never blocks)
udp_sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
udp_sock.setsockopt(socket.SOL_SOCKET, socket.SO_BROADCAST, 1)


def signal_handler(sig, frame):
    global running
    running = False
    print("\n[serial-logger] Shutting down...")
    sys.exit(0)


signal.signal(signal.SIGINT, signal_handler)
signal.signal(signal.SIGTERM, signal_handler)


def flush_log():
    """Write ring buffer to log file atomically."""
    tmp = LOG_FILE + ".tmp"
    with open(tmp, "w") as f:
        for line in log_buffer:
            f.write(line + "\n")
    os.replace(tmp, LOG_FILE)


def udp_broadcast(entry):
    """Send a log line as UDP broadcast on port 8888.

    Disabled: station firmware broadcasts directly to UDP 8888. Running this
    script alongside the socat UDP listener caused duplicates.
    """
    # try:
    #     udp_sock.sendto((entry + "\n").encode("utf-8", errors="replace"), ("<broadcast>", UDP_PORT))
    # except OSError:
    #     pass  # network down, interface missing — don't block logging
    return


def log_line(text):
    """Log a line to stdout, ring buffer, and UDP broadcast."""
    ts = datetime.now().strftime("%H:%M:%S.%f")[:-3]
    entry = f"[{ts}] {text}"
    print(entry, flush=True)
    log_buffer.append(entry)
    udp_broadcast(entry)


def marker(msg):
    """Write a marker line (not from serial)."""
    ts = datetime.now().strftime("%H:%M:%S.%f")[:-3]
    entry = f"[{ts}] --- {msg} ---"
    print(f"\033[90m{entry}\033[0m", flush=True)  # dim gray on terminal
    log_buffer.append(entry)
    udp_broadcast(entry)


def connect():
    """Try to open the serial port. Returns serial object or None."""
    try:
        s = serial.Serial(PORT, BAUD, timeout=0.5)
        return s
    except (serial.SerialException, OSError):
        return None


def monitor_loop():
    """Main loop: connect, read, reconnect."""
    flush_counter = 0

    marker(f"serial-logger started on {PORT} @ {BAUD}")
    flush_log()

    while running:
        ser = connect()
        if ser is None:
            time.sleep(RECONNECT_INTERVAL)
            continue

        marker(f"connected to {PORT}")
        flush_log()

        try:
            while running:
                try:
                    raw = ser.readline()
                except (serial.SerialException, OSError):
                    break  # port gone (flash in progress?)

                if raw:
                    text = raw.decode("utf-8", errors="replace").rstrip()
                    if text:
                        log_line(text)
                        flush_counter += 1
                        # Flush to disk every 10 lines or on important messages
                        if flush_counter >= 10 or any(
                            k in text for k in ["ERROR", "FAIL", "PANIC", "assert"]
                        ):
                            flush_log()
                            flush_counter = 0
        except KeyboardInterrupt:
            break
        finally:
            try:
                ser.close()
            except Exception:
                pass

        marker(f"disconnected from {PORT}")
        flush_log()

        # Wait for port to reappear
        while running and not os.path.exists(PORT):
            time.sleep(RECONNECT_INTERVAL)

        if running:
            # Small delay for USB enumeration to settle
            time.sleep(1.5)
            marker("reconnecting...")

    flush_log()


if __name__ == "__main__":
    # Load any existing log
    if os.path.exists(LOG_FILE):
        with open(LOG_FILE, "r") as f:
            for line in f:
                log_buffer.append(line.rstrip())

    monitor_loop()
