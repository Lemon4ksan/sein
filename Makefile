# Discover library packages, excluding examples, scripts, cmd, and vendor
PKG       := $(shell go list ./... 2>/dev/null | grep -v /examples | grep -v /scripts | grep -v /cmd/ | grep -v /vendor/)
COVER_PKG := $(shell go list ./... 2>/dev/null | grep -v /examples | grep -v /scripts | grep -v /vendor/)
BIN_DIR   ?= bin
TMP_DIR   ?= .tmp
COVER_OUT ?= $(TMP_DIR)/coverage.out

# Colors for console output
CYAN  := \033[0;36m
RESET := \033[0m

.PHONY: test race bench fuzz cover cover-clean cover-html lint format clean help

test: ## Run quick unit tests
	@printf "$(CYAN)Running unit tests...$(RESET)\n"
	go test -timeout 30s $(PKG)

race: ## Run unit tests with race detector enabled
	@printf "$(CYAN)Running tests with race detector...$(RESET)\n"
	go test -race -timeout 60s $(PKG)

bench: ## Run silicon hardware inspection and microsecond benchmark suite
	@printf "$(CYAN)Running sein silicon benchmark...$(RESET)\n"
	go test -bench=. -benchmem ./...

fuzz: ## Run continuous automated fuzz testing across all wire parsers
	@printf "$(CYAN)Running security fuzzing suite across all wire parsers...$(RESET)\n"
	go test -fuzz=^Fuzz -fuzztime=2s ./...

cover: ## Calculate and print exact core library coverage report
	@printf "$(CYAN)Generating exact coverage report...$(RESET)\n"
	@mkdir -p $(TMP_DIR)
	go test -coverpkg=$(COVER_PKG) -coverprofile=$(COVER_OUT) ./...
	go tool cover -func=$(COVER_OUT)

cover-clean: ## Generate clean coverage report and run deduplicated coverage analysis tool
	@printf "$(CYAN)Generating clean coverage report...$(RESET)\n"
	@mkdir -p $(TMP_DIR)
	go test -coverpkg=$(COVER_PKG) -coverprofile=$(COVER_OUT) ./...
	go tool cover -func=$(COVER_OUT)

cover-html: cover ## Generate coverage report and open interactive HTML in browser
	@printf "$(CYAN)Opening coverage report in browser...$(RESET)\n"
	go tool cover -html=$(COVER_OUT)

lint: ## Run golangci-lint check
	@printf "$(CYAN)Running linter...$(RESET)\n"
	golangci-lint run ./...

format: ## Format code and auto-fix linter suggestions
	@printf "$(CYAN)Formatting Go code...$(RESET)\n"
	go fmt ./...
	golangci-lint run --fix

clean: ## Delete temporary files, binaries, and coverage profiles
	@printf "$(CYAN)Cleaning up temporary artifacts...$(RESET)\n"
	rm -rf $(BIN_DIR)/ $(TMP_DIR)/
	rm -f $(COVER_OUT) coverage.out profile.cov *.out *.test *.exe

help: ## Show this help message
	@printf "Usage: make [target]\n\nTargets:\n"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'
