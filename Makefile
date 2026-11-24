# Project variables
BINARY_NAME=sparky
BUILD_DIR=bin
SERVICE_FILE=sparky.service
CONFIG_BINARY_NAME=sparky-config
CONFIG_SERVICE_FILE=sparky-config.service
CONFIG_ENV_FILE=/etc/sparky/config-ui.env
CONFIG_SERVICE_USER=$(shell id -un)

# Go related variables
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/$(BUILD_DIR)
GOPATH=$(shell go env GOPATH)

.PHONY: all build clean run test install-service install-config-service reference-audio

all: build

build:
	@echo "Building $(BINARY_NAME) to $(GOBIN)"
	@mkdir -p $(GOBIN)
	@go build -o $(GOBIN)/$(BINARY_NAME) .
	@go build -o $(GOBIN)/$(CONFIG_BINARY_NAME) ./cmd/sparky-config

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

install-service:
	@echo "Installing $(BINARY_NAME) as a service"
	# Replace placeholders in the service file template
	@sed -e 's|{{WORKING_DIRECTORY}}|'$(GOBIN)'/|g' \
	     -e 's|{{EXEC_START}}|'$(GOBIN)'/$(BINARY_NAME)|g' \
	     $(SERVICE_FILE).template > /tmp/$(SERVICE_FILE)
	# Copy the modified service file to the systemd directory
	@sudo cp /tmp/$(SERVICE_FILE) /etc/systemd/system/$(SERVICE_FILE)
	# Reload systemd to recognize the new service file
	@sudo systemctl daemon-reload
	# Enable the service to start on boot
	@sudo systemctl enable $(SERVICE_FILE)
	# Start the service immediately
	@sudo systemctl start $(SERVICE_FILE)
	@echo "$(BINARY_NAME) service installed and started"

# An administrator explicitly opts in to the configuration UI. Do not
# install or enable a network-facing admin service without credentials.
# Run as the non-root owner of the Sparky config and binaries; sudo is used
# only to install the unit and manage systemd.
install-config-service: build
	@test "$(CONFIG_ENV_FILE)" = "/etc/sparky/config-ui.env" || { echo "CONFIG_ENV_FILE must match the fixed systemd unit path"; exit 1; }
	@test "$(CONFIG_SERVICE_USER)" = "$(id -un)" || { echo "CONFIG_SERVICE_USER must match the invoking non-root user"; exit 1; }
	@test "$$(id -u)" -ne 0 || { echo "Run make as a non-root user; sudo is invoked only for installation"; exit 1; }
	@sudo test -f $(CONFIG_ENV_FILE) || { echo "Missing $(CONFIG_ENV_FILE); create it with SPARKY_CONFIG_PASSWORD (16+ characters) first"; exit 1; }
	@test "$(sudo stat -c %a $(CONFIG_ENV_FILE))" = "600" || { echo "$(CONFIG_ENV_FILE) must have permissions 0600"; exit 1; }
	@test "$(sudo stat -c %u $(CONFIG_ENV_FILE))" = "0" || { echo "$(CONFIG_ENV_FILE) must be owned by root"; exit 1; }
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
