.PHONY: test test-schemas test-firmware test-server test-integration test-all \
        logs-controller logs-terminal \
        restart-controller restart-terminal restart \
        kill-controller kill-terminal \
        start stop status \
        enable disable

test-schemas:          ## Schema & contract tests (no deps)
	cd tests && python -m pytest schemas/ -v

test-firmware:         ## Firmware unit tests on host (needs PlatformIO)
	cd subsystems/station && pio test -e native

test-server:           ## Go unit tests
	@if find server -name '*.go' 2>/dev/null | grep -q .; then \
		cd server && go test ./...; \
	else \
		echo "No Go source files yet, skipping"; \
	fi

test-integration:      ## Redis integration tests (needs Redis at localhost:6379)
	cd tests && python -m pytest integration/ -v

test: test-schemas test-server  ## Quick tests (no hardware, no Redis)

test-all: test test-firmware test-integration  ## Everything that can run without hardware

# ---- systemd (arturo-controller.service, arturo-terminal.service) ----

logs-controller:       ## Follow controller logs
	journalctl -u arturo-controller -f

logs-terminal:         ## Follow terminal logs
	journalctl -u arturo-terminal -f

restart-controller:    ## Restart controller (use after rebuild)
	sudo systemctl restart arturo-controller

restart-terminal:      ## Restart terminal (use after rebuild)
	sudo systemctl restart arturo-terminal

restart: restart-controller restart-terminal  ## Restart both

kill-controller:       ## SIGTERM controller (tests auto-restart, ~2s)
	kill $$(systemctl show -p MainPID --value arturo-controller)

kill-terminal:         ## SIGTERM terminal (tests auto-restart, ~2s)
	kill $$(systemctl show -p MainPID --value arturo-terminal)

start:                 ## Start both services
	sudo systemctl start arturo-controller arturo-terminal

stop:                  ## Stop both services
	sudo systemctl stop arturo-controller arturo-terminal

status:                ## Status of both services
	systemctl status arturo-controller arturo-terminal --no-pager

enable:                ## Enable both services at boot
	sudo systemctl enable arturo-controller arturo-terminal

disable:               ## Disable both services at boot
	sudo systemctl disable arturo-controller arturo-terminal
