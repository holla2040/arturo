PORT       ?= /dev/ttyACM0
BAUD       ?= 115200
FLASH_BAUD ?= 460800
ENV        ?= esp32s3
LOG        := /tmp/arturo-serial.log
LOGGER     := python3 $(CURDIR)/../../tools/serial-monitor/serial_logger.py $(PORT)
BUILD_DIR  := .pio/build/$(ENV)
ESPTOOL    := $(HOME)/.platformio/penv/bin/esptool
BOOT_APP0  := $(HOME)/.platformio/packages/framework-arduinoespressif32/tools/partitions/boot_app0.bin

# Default: compile only
bin: build

# Build only
build:
	pio run -e $(ENV)

# Kill logger, flash prebuilt binary via esptool directly (no pio/SCons overhead — run `make build` first)
flash:
	@-pkill -f "python3.*serial_logger" 2>/dev/null; sleep 0.5
	$(ESPTOOL) --chip esp32s3 --port $(PORT) --baud $(FLASH_BAUD) \
	  --before default-reset --after hard-reset write-flash -z \
	  --flash-mode dio --flash-freq 80m --flash-size detect \
	  0x0000  $(BUILD_DIR)/bootloader.bin \
	  0x8000  $(BUILD_DIR)/partitions.bin \
	  0xe000  $(BOOT_APP0) \
	  0x10000 $(BUILD_DIR)/firmware.bin
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
