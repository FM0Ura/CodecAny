# ==============================================================================
# CodecAny Makefile
# ==============================================================================
# This Makefile provides commands to build, run, test, and maintain 
# the CodecAny project.
#
# Usage:
#   make [target] [variables]
# ==============================================================================

# Default target runs when 'make' is called without arguments
.DEFAULT_GOAL := help

# OS detection
OS := $(shell uname -s 2>/dev/null || echo Windows)

# Go variables
GOCMD     := go
GOBUILD   := $(GOCMD) build
GOCLEAN   := $(GOCMD) clean
GOTEST    := $(GOCMD) test
GOFMT     := $(GOCMD) fmt
GOLINT    := golangci-lint
BINARY_NAME := codecany

# ANSI Color Codes for stylized console output
COLOR_RESET   := \033[0m
COLOR_TITLE   := \033[1;36m
COLOR_SECTION := \033[1;33m
COLOR_TARGET  := \033[1;32m
COLOR_ARG     := \033[35m

# Export ASCII banner to environment for clean multi-line print
define BANNER
   ______          __              ___             
  / ____/___  ____/ /__  ________ /   |  ____  __  __
 / /   / __ \/ __  / _ \/ ___/ __  /| | / __ \/ / / / 
/ /___/ /_/ / /_/ /  __/ /__/ /_/ ___ |/ / / / /_/ /  
\____/\____/\__,_/\___/\___/\__,_/_/  |_/_/ /_/\__, /   
                                              /____/    
endef
export BANNER

##@ General Info
.PHONY: help
help: ## Display this styled help menu (default)
	@printf "$(COLOR_TITLE)"
	@echo "$$BANNER"
	@printf "$(COLOR_RESET)"
	@printf "CodecAny CLI build & development orchestrator.\n\n"
	@printf "Usage:\n  make $(COLOR_TARGET)<target>$(COLOR_RESET) [$(COLOR_ARG)VARIABLE$(COLOR_RESET)=value]\n"
	@awk 'BEGIN {FS = ":.*##"; printf ""} \
		/^[a-zA-Z_-]+:.*?##/ { printf "  $(COLOR_TARGET)%-18s$(COLOR_RESET) %s\n", $$1, $$2 } \
		/^##@/ { printf "\n$(COLOR_SECTION)%s$(COLOR_RESET)\n", substr($$0, 5) }' $(MAKEFILE_LIST)
	@printf "\n"

##@ Build & Execution
.PHONY: build
build: ## Compile the CLI binary
	@echo "Building binary..."
	$(GOBUILD) -o $(BINARY_NAME) ./cmd/cli
	@echo "Build complete: ./$(BINARY_NAME)"

.PHONY: clean
clean: ## Clean build files and standard Go caches
	@echo "Cleaning up..."
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	@echo "Clean complete!"

.PHONY: run
run: ## Run the CLI tool (e.g. make run ARGS="-dir test_dir")
	@echo "Running CodecAny..."
	$(GOCMD) run ./cmd/cli/main.go $(ARGS)

##@ Testing & Quality
.PHONY: test
test: ## Run package unit tests
	@echo "Running tests..."
	$(GOTEST) -v ./...

.PHONY: lint
lint: ## Run golangci-lint static analysis checks
	@echo "Linting codebase..."
	@if command -v $(GOLINT) >/dev/null 2>&1; then \
		$(GOLINT) run; \
	else \
		echo "golangci-lint is not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

.PHONY: fmt
fmt: ## Auto-format Go code using standard gofmt
	@echo "Formatting code..."
	$(GOFMT) ./...

##@ Knowledge Graph (Graphify)
.PHONY: graph-update
graph-update: ## Update codebase graphify knowledge graph
	@echo "Updating knowledge graph..."
	graphify update .

.PHONY: graph-query
graph-query: ## Query knowledge graph (e.g. make graph-query QUERY="What does main do?")
	@if [ -z "$(QUERY)" ]; then \
		echo "Error: QUERY variable is required. Example: make graph-query QUERY=\"What is main?\""; \
		exit 1; \
	fi
	graphify query "$(QUERY)"
