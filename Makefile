# Project variables
BINARY_NAME=sparky
BUILD_DIR=bin
SERVICE_FILE=sparky.service
CONFIG_BINARY_NAME=sparky-config
CONFIG_SERVICE_FILE=sparky-config.service
CONFIG_ENV_FILE=/etc/sparky/config-ui.env
CONFIG_SERVICE_USER=$(shell id -un)
GATEWAY_SERVICE_USER=$(shell id -un)

# Go related variables
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/$(BUILD_DIR)
GOPATH=$(shell go env GOPATH)

.PHONY: all build clean run test init-config install-service install-config-service reference-audio

all: build

build:
	@echo "Building $(BINARY_NAME) to $(GOBIN)"
	@mkdir -p $(GOBIN)
	@go build -o $(GOBIN)/$(BINARY_NAME) .
	@go build -o $(GOBIN)/$(CONFIG_BINARY_NAME) ./cmd/sparky-config

# Publish a complete private file to the exact destination, without replacement.
init-config:
	@go run "$(dir $(abspath $(lastword $(MAKEFILE_LIST))))cmd/sparky-init-config/main.go" config.yml.example "$(GOBIN)/config.yml"
	@echo "Created $(GOBIN)/config.yml with iGate/transmit/digipeat disabled."
	@echo "Edit station-callsign and receive settings before starting Sparky."

clean:
	@echo "Cleaning"
	@GOBIN=$(GOBIN) go clean
	@rm -f $(GOBIN)/$(BINARY_NAME) $(GOBIN)/$(CONFIG_BINARY_NAME)

run: build
	@echo "Running $(BINARY_NAME)"
	@$(GOBIN)/$(BINARY_NAME)

test:
	@echo "Running tests"
	@go test ./...

reference-audio:
	@./scripts/gen-reference-audio.sh

# First-time systemd installer only. Existing units require a manual migration
# review to avoid silently changing user, log location, or device permissions.
# Run make as the intended *non-root* gateway account, never under sudo.
install-service:
	@test "$(GATEWAY_SERVICE_USER)" = "$$(id -un)" || { echo "GATEWAY_SERVICE_USER must match the invoking non-root user" >&2; exit 1; }
	@test "$$(id -u)" -ne 0 || { echo "Do not run make as root; sudo is used only to install the service" >&2; exit 1; }
	@test -x "$(GOBIN)/$(BINARY_NAME)" || { echo "Missing gateway binary; run make build first" >&2; exit 1; }
	@test -f "$(GOBIN)/config.yml" && test ! -L "$(GOBIN)/config.yml" && test -r "$(GOBIN)/config.yml" || { echo "Missing, unreadable or symlinked bin/config.yml; initialize and review it first" >&2; exit 1; }
	@test -w "$(GOBIN)" || { echo "Gateway account needs write access to the existing working directory for its cache" >&2; exit 1; }
	@go run ./cmd/sparky-device-check --config "$(GOBIN)/config.yml" || { echo "Device permissions preflight failed: review guidance above before installing" >&2; exit 1; }
	@sudo test ! -e /etc/systemd/system/$(SERVICE_FILE) && sudo test ! -L /etc/systemd/system/$(SERVICE_FILE) || { echo "Existing gateway unit: review and migrate it manually; refusing to overwrite" >&2; exit 1; }
	@! sudo systemctl cat $(SERVICE_FILE) >/dev/null 2>&1 || { echo "Existing gateway systemd unit found in unit search paths; review migration manually" >&2; exit 1; }
	@echo "Installing new gateway service as $(GATEWAY_SERVICE_USER) (journal logs; no root runtime)"
	@sed -e 's|{{WORKING_DIRECTORY}}|$(GOBIN)/|g' \
	     -e 's|{{EXEC_START}}|$(GOBIN)/$(BINARY_NAME)|g' \
	     -e 's|{{SERVICE_USER}}|$(GATEWAY_SERVICE_USER)|g' \
	     $(SERVICE_FILE).template | sudo sh -c 'set -C; umask 022; cat > /etc/systemd/system/$(SERVICE_FILE)'
	@sudo systemctl daemon-reload
	@sudo systemctl enable --now $(SERVICE_FILE)
	@echo "Sparky gateway service started. Inspect: sudo journalctl -u $(SERVICE_FILE)"

# An administrator explicitly opts in to the configuration UI. Do not
# install or enable a network-facing admin service without credentials.
# Run as the non-root owner of the Sparky config and binaries; sudo is used
# only to install the unit and manage systemd.
install-config-service: build
	@test "$(CONFIG_ENV_FILE)" = "/etc/sparky/config-ui.env" || { echo "CONFIG_ENV_FILE must match the fixed systemd unit path"; exit 1; }
	@test "$(CONFIG_SERVICE_USER)" = "$$(id -un)" || { echo "CONFIG_SERVICE_USER must match the invoking non-root user"; exit 1; }
	@test "$$(id -u)" -ne 0 || { echo "Run make as a non-root user; sudo is invoked only for installation"; exit 1; }
	@sudo test -f $(CONFIG_ENV_FILE) || { echo "Missing $(CONFIG_ENV_FILE); create it with SPARKY_CONFIG_PASSWORD (16+ characters) first"; exit 1; }
	@test "$$(sudo stat -c %a $(CONFIG_ENV_FILE))" = "600" || { echo "$(CONFIG_ENV_FILE) must have permissions 0600"; exit 1; }
	@test "$$(sudo stat -c %u $(CONFIG_ENV_FILE))" = "0" || { echo "$(CONFIG_ENV_FILE) must be owned by root"; exit 1; }
	@test -f $(GOBIN)/config.yml && test -r $(GOBIN)/config.yml && test -w $(GOBIN) || { echo "The UI user must be able to read bin/config.yml and write its directory"; exit 1; }
	@sudo grep -Eq '^SPARKY_CONFIG_PASSWORD=.{16,}$$' $(CONFIG_ENV_FILE) || { echo "$(CONFIG_ENV_FILE) needs SPARKY_CONFIG_PASSWORD with 16+ characters"; exit 1; }
	@echo "Installing $(CONFIG_BINARY_NAME) as an independent, opt-in systemd service"
	@sed -e 's|{{WORKING_DIRECTORY}}|$(GOBIN)/|g' \
	     -e 's|{{EXEC_START}}|$(GOBIN)/$(CONFIG_BINARY_NAME)|g' \
	     -e 's|{{SERVICE_USER}}|$(CONFIG_SERVICE_USER)|g' \
	     $(CONFIG_SERVICE_FILE).template | sudo tee /etc/systemd/system/$(CONFIG_SERVICE_FILE) >/dev/null
	@sudo systemctl daemon-reload
	@sudo systemctl enable --now $(CONFIG_SERVICE_FILE)
	@echo "Sparky web configuration UI enabled on 127.0.0.1:8080 (restart remains disabled)"
