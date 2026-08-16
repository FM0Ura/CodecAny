# ==============================================================================
# CodecAny Makefile
# ==============================================================================
# This Makefile provides commands to build, run, test, and maintain
# the CodecAny project — the CLI daemon (cmd/cli) and the optional web
# control panel (cmd/server + web/, "Signal Path" — see
# docs/design_painel_controle.md).
#
# Usage:
#   make [target] [variables]
# ==============================================================================

# Default target runs when 'make' is called without arguments
.DEFAULT_GOAL := help

# OS detection
OS := $(shell uname -s 2>/dev/null || echo Windows)

# Go variables
GOCMD       := go
GOBUILD     := $(GOCMD) build
GOCLEAN     := $(GOCMD) clean
GOTEST      := $(GOCMD) test
GOFMT       := $(GOCMD) fmt
GOLINT      := golangci-lint
BINARY_NAME := codecany

# Web control panel (GUI) variables
WEB_DIR         := web
WEBDIST_DIR     := cmd/server/webdist
PNPM            := pnpm
GUI_BINARY_NAME := codecany-server
GUI_ADDR        := 127.0.0.1:8383

# ANSI 24-bit color codes, lifted straight from the "Signal Path" design
# tokens (docs/design_painel_controle.md) — this help menu is styled like a
# terminal-native extension of the panel itself, not a generic Makefile.
# NOTE: no inline "# comment" on the same line as these assignments — GNU
# Make keeps the whitespace before a trailing comment as part of the value,
# which would leak stray padding into every colored string below.
# --accent      #8C6FF2 (roxo, marca/identidade)
# --signal      #3FD6E6 (cyano, ao vivo/agrupamento)
# --info        #8993D1 (periwinkle, variável)
# --text-muted  #A99CC4 (descrição)
# --text-faint  #6D6184 (nota discreta)
# --success     #5AD394
# --danger      #F16B63
COLOR_RESET   := \033[0m
COLOR_TITLE   := \033[1;38;2;140;111;242m
COLOR_SECTION := \033[1;38;2;63;214;230m
COLOR_TARGET  := \033[38;2;140;111;242m
COLOR_ARG     := \033[38;2;137;147;209m
COLOR_DESC    := \033[38;2;169;156;196m
COLOR_FAINT   := \033[38;2;109;97;132m
COLOR_SIGNAL  := \033[38;2;63;214;230m
COLOR_SUCCESS := \033[38;2;90;211;148m
COLOR_DANGER  := \033[38;2;241;107;99m

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
	@printf "$(COLOR_SIGNAL)●$(COLOR_RESET) $(COLOR_FAINT)CLI daemon + painel de controle web (\"Signal Path\") — build & dev orchestrator.$(COLOR_RESET)\n\n"
	@printf "Usage:\n  make $(COLOR_TARGET)<target>$(COLOR_RESET) [$(COLOR_ARG)VARIABLE$(COLOR_RESET)=value]\n"
	@awk 'BEGIN {FS = ":.*##"; printf ""} \
		/^[a-zA-Z_-]+:.*?##/ { printf "  $(COLOR_TARGET)%-18s$(COLOR_RESET) $(COLOR_DESC)%s$(COLOR_RESET)\n", $$1, $$2 } \
		/^##@/ { printf "\n$(COLOR_SECTION)◆ %s$(COLOR_RESET)\n", substr($$0, 5) }' $(MAKEFILE_LIST)
	@printf "\n"

##@ Build & Execution (CLI)
.PHONY: build
build: ## Compile the CLI binary
	@printf "$(COLOR_SIGNAL)▸$(COLOR_RESET) Building CLI binary...\n"
	$(GOBUILD) -o $(BINARY_NAME) ./cmd/cli
	@printf "$(COLOR_SUCCESS)✓$(COLOR_RESET) Build complete: ./$(BINARY_NAME)\n"

.PHONY: clean
clean: ## Clean build files and standard Go caches
	@printf "$(COLOR_SIGNAL)▸$(COLOR_RESET) Cleaning up...\n"
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	@printf "$(COLOR_SUCCESS)✓$(COLOR_RESET) Clean complete!\n"

.PHONY: run
run: ## Run the CLI tool (e.g. make run ARGS="-dir test_dir")
	@printf "$(COLOR_SIGNAL)▸$(COLOR_RESET) Running CodecAny CLI...\n"
	$(GOCMD) run ./cmd/cli/main.go $(ARGS)

##@ Web Control Panel (GUI)
.PHONY: gui-deps
gui-deps: ## Install frontend dependencies (pnpm only, see web/package.json)
	@if ! command -v $(PNPM) >/dev/null 2>&1; then \
		printf "$(COLOR_DANGER)✗$(COLOR_RESET) pnpm not found. Install it first: https://pnpm.io/installation\n"; \
		exit 1; \
	fi
	@printf "$(COLOR_SIGNAL)▸$(COLOR_RESET) Installing frontend dependencies (pnpm)...\n"
	cd $(WEB_DIR) && $(PNPM) install
	@printf "$(COLOR_SUCCESS)✓$(COLOR_RESET) Frontend dependencies installed.\n"

.PHONY: gui-build-web
gui-build-web: gui-deps ## Build only the frontend SPA (outputs to cmd/server/webdist)
	@printf "$(COLOR_SIGNAL)▸$(COLOR_RESET) Building frontend (React + TS + Vite)...\n"
	cd $(WEB_DIR) && $(PNPM) build
	@printf "$(COLOR_SUCCESS)✓$(COLOR_RESET) Frontend built: ./$(WEBDIST_DIR)\n"

.PHONY: gui-build
gui-build: gui-build-web ## Build the full GUI binary (frontend embedded via go:embed)
	@printf "$(COLOR_SIGNAL)▸$(COLOR_RESET) Building GUI server binary...\n"
	$(GOBUILD) -o $(GUI_BINARY_NAME) ./cmd/server
	@printf "$(COLOR_SUCCESS)✓$(COLOR_RESET) Build complete: ./$(GUI_BINARY_NAME) (frontend embedded)\n"

.PHONY: gui-run
gui-run: ## Run the GUI server (e.g. make gui-run ARGS="-dir test_dir")
	@printf "$(COLOR_SIGNAL)▸$(COLOR_RESET) Running CodecAny control panel...\n"
	@printf "$(COLOR_FAINT)  Sem autenticação — bind padrão em $(GUI_ADDR) (rede confiável).$(COLOR_RESET)\n"
	$(GOCMD) run ./cmd/server $(ARGS)

.PHONY: gui-dev
gui-dev: gui-deps ## Run the frontend in dev mode (Vite, hot-reload; proxies /api to gui-run)
	@printf "$(COLOR_SIGNAL)▸$(COLOR_RESET) Starting Vite dev server...\n"
	@printf "$(COLOR_FAINT)  Precisa do backend rodando em paralelo — noutro terminal: make gui-run$(COLOR_RESET)\n"
	cd $(WEB_DIR) && $(PNPM) dev

.PHONY: gui-clean
gui-clean: ## Remove the GUI binary (embedded frontend assets in webdist/ stay tracked in git)
	@printf "$(COLOR_SIGNAL)▸$(COLOR_RESET) Cleaning up GUI binary...\n"
	rm -f $(GUI_BINARY_NAME)
	@printf "$(COLOR_SUCCESS)✓$(COLOR_RESET) Clean complete!\n"

##@ Everything
.PHONY: build-all
build-all: build gui-build ## Build both the CLI and the GUI binaries

.PHONY: clean-all
clean-all: clean gui-clean ## Clean both the CLI and the GUI binaries

##@ Testing & Quality
.PHONY: test
test: ## Run package unit tests
	@printf "$(COLOR_SIGNAL)▸$(COLOR_RESET) Running tests...\n"
	$(GOTEST) -v ./...

.PHONY: lint
lint: ## Run golangci-lint static analysis checks
	@printf "$(COLOR_SIGNAL)▸$(COLOR_RESET) Linting codebase...\n"
	@if command -v $(GOLINT) >/dev/null 2>&1; then \
		$(GOLINT) run; \
	else \
		printf "$(COLOR_DANGER)✗$(COLOR_RESET) golangci-lint is not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest\n"; \
	fi

.PHONY: fmt
fmt: ## Auto-format Go code using standard gofmt
	@printf "$(COLOR_SIGNAL)▸$(COLOR_RESET) Formatting code...\n"
	$(GOFMT) ./...

##@ Knowledge Graph (Graphify)
.PHONY: graph-update
graph-update: ## Update codebase graphify knowledge graph
	@printf "$(COLOR_SIGNAL)▸$(COLOR_RESET) Updating knowledge graph...\n"
	graphify update .

.PHONY: graph-query
graph-query: ## Query knowledge graph (e.g. make graph-query QUERY="What does main do?")
	@if [ -z "$(QUERY)" ]; then \
		printf "$(COLOR_DANGER)✗$(COLOR_RESET) QUERY variable is required. Example: make graph-query QUERY=\"What is main?\"\n"; \
		exit 1; \
	fi
	graphify query "$(QUERY)"
