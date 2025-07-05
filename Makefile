.PHONY: build install clean

BINARY_NAME=wtree
BUILD_DIR=bin
INSTALL_DIR=$(HOME)/.local/bin

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/$(BINARY_NAME)
	@echo "Built $(BUILD_DIR)/$(BINARY_NAME)"

install: build
	@echo "Installing $(BINARY_NAME) as $(BINARY_NAME) to $(INSTALL_DIR)..."
	@mkdir -p $(INSTALL_DIR)
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Installed $(INSTALL_DIR)/$(BINARY_NAME)"

clean:
	@echo "Cleaning build directory..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean complete"

help:
	@echo "Available targets:"
	@echo "  build   - Build the binary to bin/ directory"
	@echo "  install - Build and install as 'wtree' to ~/.local/bin"
	@echo "  clean   - Remove build directory"
	@echo "  help    - Show this help message"