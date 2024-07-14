# Project variables
BINARY_NAME=go-igate
BUILD_DIR=bin
SERVICE_FILE=go-igate.service

# Go related variables
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/$(BUILD_DIR)
GOPATH=$(shell go env GOPATH)

.PHONY: all build clean run test install-service

all: build

build:
	@echo "Building $(BINARY_NAME) to $(GOBIN)"
	@GOBIN=$(GOBIN) go build -o $(GOBIN)/$(BINARY_NAME) .

clean:
	@echo "Cleaning"
	@GOBIN=$(GOBIN) go clean
	@rm -rf $(GOBIN)/$(BINARY_NAME)

run: build
	@echo "Running $(BINARY_NAME)"
	@$(GOBIN)/$(BINARY_NAME)

test:
	@echo "Running tests"
	@go test ./...

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