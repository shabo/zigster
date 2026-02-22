.PHONY: build start stop restart history stress stress-cpu stress-gpu stress-nvme stress-disk stress-wifi stress-all test clean help
.DEFAULT_GOAL := help

BIN      := sensors
PIDFILE  := /tmp/sensors-monitor.pid
DURATION ?= 60s

help: ## Show available targets
	@echo ""
	@echo "  sensors monitor"
	@echo "  ───────────────"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "  Set duration: make stress-cpu DURATION=30s"
	@echo ""

build: ## Build the binary
	go build -o $(BIN) .

start: build ## Build and run (foreground, live monitoring)
	./$(BIN)

stop: ## Stop a backgrounded instance
	@if [ -f $(PIDFILE) ] && kill -0 $$(cat $(PIDFILE)) 2>/dev/null; then \
		kill $$(cat $(PIDFILE)); \
		rm -f $(PIDFILE); \
		echo "stopped (pid $$(cat $(PIDFILE) 2>/dev/null))"; \
	else \
		echo "not running"; \
		rm -f $(PIDFILE); \
	fi

restart: stop start ## Restart (stop + start)

history: build ## Browse saved historical temperature data
	./$(BIN) --history

stress: build ## Show stress test targets
	./$(BIN) stress

stress-cpu: build ## Stress all CPU cores
	./$(BIN) stress cpu $(DURATION)

stress-gpu: build ## Stress NVIDIA GPU
	./$(BIN) stress gpu $(DURATION)

stress-nvme: build ## Stress NVMe SSD (random I/O)
	./$(BIN) stress nvme $(DURATION)

stress-disk: build ## Stress SATA HDD (sequential I/O)
	./$(BIN) stress disk $(DURATION)

stress-wifi: build ## Stress WiFi / network adapter
	./$(BIN) stress wifi $(DURATION)

stress-all: build ## Stress ALL components at once
	./$(BIN) stress all $(DURATION)

test: ## Run tests
	go test -v ./...

clean: ## Remove build artifacts
	rm -f $(BIN)
