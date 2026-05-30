# Variables
CARGO = cargo
GO = go
NPM = npm
BUILD_DIR = target/release
INDEXER_DIR = fs_indexer
AGGREGATOR_DIR = fs_aggregator
API_DIR = fs_api
UI_DIR = fs_ui

.PHONY: all clean build-indexer build-aggregator build-ui build-api run-indexer run-aggregator run-api swagger-api

# Default target: build everything
all: swagger-api build-indexer build-aggregator build-ui build-api

build-indexer:
	@echo "Building fs_indexer..."
	cd $(INDEXER_DIR) && $(CARGO) build --release

build-aggregator:
	@echo "Building fs_aggregator..."
	cd $(AGGREGATOR_DIR) && $(CARGO) build --release

build-ui:
	@echo "Building fs_ui..."
	cd $(UI_DIR) && $(NPM) ci && $(NPM) run build
	@echo "Moving UI build to API directory..."
	mkdir -p $(API_DIR)/ui
	cp -r $(UI_DIR)/build $(API_DIR)/ui/

build-api:
	@echo "Building fs_api..."
	cd $(API_DIR) && $(GO) build -o fs_api .

# Convenience targets to run the apps
run-indexer: build-indexer
	./$(INDEXER_DIR)/$(BUILD_DIR)/fs_indexer

run-aggregator: build-aggregator
	./$(AGGREGATOR_DIR)/$(BUILD_DIR)/fs_aggregator

run-api: swagger-api build-api build-ui
	./$(API_DIR)/fs_api

clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(INDEXER_DIR)/target $(AGGREGATOR_DIR)/target $(API_DIR)/fs_api $(API_DIR)/ui $(UI_DIR)/build $(UI_DIR)/node_modules
	
swagger-api:
	@echo "Generating Swagger documentation for API..."
	cd $(API_DIR) && go install github.com/swaggo/swag/cmd/swag@v1.16.6 && $(shell go env GOPATH)/bin/swag init -g main.go -o docs
