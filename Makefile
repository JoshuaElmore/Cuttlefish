# Variables
CARGO = cargo
BUILD_DIR = target/release
INDEXER_DIR = fs_indexer
AGGREGATOR_DIR = fs_aggregator

.PHONY: all clean build-indexer build-aggregator run-indexer run-aggregator

# Default target: build both
all: build-indexer build-aggregator

build-indexer:
	@echo "Building fs_indexer..."
	cd $(INDEXER_DIR) && $(CARGO) build --release

build-aggregator:
	@echo "Building fs_aggregator..."
	cd $(AGGREGATOR_DIR) && $(CARGO) build --release

# Convenience targets to run the apps
run-indexer: build-indexer
	./$(INDEXER_DIR)/$(BUILD_DIR)/fs_indexer

run-aggregator: build-aggregator
	./$(AGGREGATOR_DIR)/$(BUILD_DIR)/fs_aggregator

clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(INDEXER_DIR)/target $(AGGREGATOR_DIR)/target
	rm -f *.csv
