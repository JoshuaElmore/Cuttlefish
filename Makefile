# Variables
CARGO = cargo
GO = go
NPM = npm
BUILD_DIR = target/release
INDEXER_DIR = fs_indexer
AGGREGATOR_DIR = fs_aggregator
API_DIR = fs_api
UI_DIR = fs_ui

.PHONY: all clean build-indexer build-aggregator build-ui build-api build-api-debug run-indexer run-aggregator run-api swagger-api test-api bench-api

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
# Replaced, not merged: Vite hashes each bundle's filename, so copying over the
# top left every previously built assets/index-*.js in place. They are dead
# weight the server still happily hands out, and the directory grew with every
# build.
	rm -rf $(API_DIR)/ui/build
	mkdir -p $(API_DIR)/ui
	cp -r $(UI_DIR)/build $(API_DIR)/ui/

build-api:
	@echo "Building fs_api..."
	cd $(API_DIR) && $(GO) build -o fs_api .

# Same binary with per-request debug logging compiled in (see debugMode in middleware.go)
build-api-debug:
	@echo "Building fs_api (debug logging)..."
	cd $(API_DIR) && $(GO) build -ldflags "-X main.debugMode=1" -o fs_api .

# Go tests. The database-backed MCP tests skip themselves unless
# CUTTLEFISH_TEST_DSN points at a database they may create a scratch schema in:
#
#   make test-api CUTTLEFISH_TEST_DSN="host=localhost user=postgres password=postgres dbname=fs_index sslmode=disable"
test-api:
	@echo "Testing fs_api..."
	cd $(API_DIR) && $(GO) test ./...

# Tool-call benchmarks. Needs CUTTLEFISH_TEST_DSN; a no-op without it.
bench-api:
	@echo "Benchmarking fs_api MCP tools..."
	cd $(API_DIR) && $(GO) test -run XXX -bench MCPTools -benchtime 100x ./...

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
