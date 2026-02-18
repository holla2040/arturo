.PHONY: test test-schemas test-firmware test-server test-integration test-all

test-schemas:          ## Schema & contract tests (no deps)
	cd tests && python -m pytest schemas/ -v

test-firmware:         ## Firmware unit tests on host (needs PlatformIO)
	cd firmware && pio test -e native

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
