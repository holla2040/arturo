PORT     ?= /dev/ttyACM1
BAUD     ?= 115200
ENV      ?= esp32s3
LOG      := /tmp/arturo-serial.log
LOGGER   := python3 $(CURDIR)/../../tools/serial-monitor/serial_logger.py $(PORT)

# Default: compile only
bin: build

# Build only
build:
	pio run -e $(ENV)

# Build, kill logger, flash, restart logger
flash: build
	@-pkill -f "python3.*serial_logger" 2>/dev/null; sleep 0.5
	pio run -e $(ENV) -t upload --upload-port $(PORT)
	@sleep 2
	@$(LOGGER) > /dev/null 2>&1 &
	@echo "--- logger restarted on $(PORT) ---"

# Build and flash over WiFi (no serial connection needed)
ota: build
	pio run -e ota -t upload

# Start the serial logger (foreground, Ctrl-C to stop)
monitor:
	$(LOGGER)

# Start the serial logger in background
monitor-bg:
	@-pkill -f "python3.*serial_logger" 2>/dev/null; sleep 0.3
	@$(LOGGER) > /dev/null 2>&1 &
	@echo "--- logger running in background, log at $(LOG) ---"

# Listen to UDP broadcast (survives logger restarts)
mon:
	@echo "Listening for UDP broadcasts on port 8888..."
	@socat -u UDP4-RECV:8888,broadcast,reuseaddr -

# Tail the log file
log:
	tail -f $(LOG)

# Show last 50 lines of log
log50:
	tail -50 $(LOG)

# Kill the background logger
monitor-stop:
	@-pkill -f "python3.*serial_logger" 2>/dev/null
	@echo "--- logger stopped ---"

# Run native unit tests
test:
	pio test -e native

# Clean build artifacts
clean:
	pio run -e $(ENV) -t clean

.PHONY: bin build flash ota monitor monitor-bg mon monitor-stop log log50 test clean
