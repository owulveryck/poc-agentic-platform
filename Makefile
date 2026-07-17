# PPG — Platform Planning Gateway
#
# Common workflow:
#   make quickstart    # build + one-minute guided demo on the examples/ corpus
#   make install       # build + install binaries into ~/.local/bin
#   make test          # run all tests
#
# Override the install location:
#   make install BINDIR=/usr/local/bin

BINDIR  ?= $(HOME)/.local/bin
GO      ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo devel)
LDFLAGS  = -ldflags "-X github.com/owulveryck/poc-agentic-platform/internal/version.Version=$(VERSION)"

.PHONY: help build quickstart install uninstall setup-claude-code remove-claude-code \
        setup-github-copilot remove-github-copilot test lint tidy clean

## help: Show this help.
help:
	@awk 'BEGIN{FS=":.*##"; printf "Targets:\n"} \
	  /^##/ {sub(/^## */,""); printf "  %s\n", $$0}' $(MAKEFILE_LIST)

## build: Build all binaries into ./bin/
build:
	@mkdir -p bin
	$(GO) build $(LDFLAGS) -o bin/ppg               ./cmd/ppg
	$(GO) build $(LDFLAGS) -o bin/ppg-mcp-server    ./adapters/claudecode/mcpserver
	$(GO) build $(LDFLAGS) -o bin/ppg-guard         ./adapters/claudecode/guard
	$(GO) build $(LDFLAGS) -o bin/ppg-copilot-guard ./adapters/copilot/guard
	$(GO) build $(LDFLAGS) -o bin/ppg-preflight     ./adapters/preflight
	$(GO) build $(LDFLAGS) -o bin/ppg-verify        ./cmd/ppg-verify
	$(GO) build $(LDFLAGS) -o bin/svc-mock          ./cmd/svc-mock
	@echo "Built into ./bin/ ($(VERSION))"

## quickstart: Build, start a throwaway gateway on the examples/ demo corpus, and run a guided /enrich + /lock_in_plan + /discover_service tour.
quickstart: build
	@bash scripts/quickstart.sh

## install: Install binaries into $(BINDIR) (default ~/.local/bin).
install:
	@mkdir -p $(BINDIR)
	$(GO) build $(LDFLAGS) -o $(BINDIR)/ppg               ./cmd/ppg
	$(GO) build $(LDFLAGS) -o $(BINDIR)/ppg-mcp-server    ./adapters/claudecode/mcpserver
	$(GO) build $(LDFLAGS) -o $(BINDIR)/ppg-guard         ./adapters/claudecode/guard
	$(GO) build $(LDFLAGS) -o $(BINDIR)/ppg-copilot-guard ./adapters/copilot/guard
	$(GO) build $(LDFLAGS) -o $(BINDIR)/ppg-preflight     ./adapters/preflight
	$(GO) build $(LDFLAGS) -o $(BINDIR)/ppg-verify        ./cmd/ppg-verify
	$(GO) build $(LDFLAGS) -o $(BINDIR)/svc-mock          ./cmd/svc-mock
	@echo "Installed to $(BINDIR) ($(VERSION))"

## uninstall: Remove installed binaries from $(BINDIR).
uninstall:
	rm -fv $(BINDIR)/ppg \
	       $(BINDIR)/ppg-mcp-server \
	       $(BINDIR)/ppg-guard \
	       $(BINDIR)/ppg-copilot-guard \
	       $(BINDIR)/ppg-preflight \
	       $(BINDIR)/ppg-verify \
	       $(BINDIR)/svc-mock

## setup-claude-code: Register the ppg MCP server + hooks user-scope for Claude Code (DRY_RUN=1 to preview, FORCE=1 to overwrite).
setup-claude-code:
	@scripts/setup-claude-code.sh

## remove-claude-code: Unregister the ppg MCP + strip ppg-guard hooks from Claude Code (DRY_RUN=1 to preview).
remove-claude-code:
	@scripts/remove-claude-code.sh

## setup-github-copilot: Register the ppg MCP server + hooks user-scope for GitHub Copilot (DRY_RUN=1 to preview, FORCE=1 to overwrite).
setup-github-copilot:
	@scripts/setup-github-copilot.sh

## remove-github-copilot: Unregister the ppg MCP + delete the ppg hook file for GitHub Copilot (DRY_RUN=1 to preview).
remove-github-copilot:
	@scripts/remove-github-copilot.sh

## test: Run all tests.
test:
	$(GO) test ./...

## lint: Run go vet and golangci-lint (if installed).
lint:
	$(GO) vet ./...
	@command -v golangci-lint >/dev/null 2>&1 \
	  && golangci-lint run \
	  || echo "golangci-lint not installed; ran go vet only"

## tidy: Run go mod tidy.
tidy:
	$(GO) mod tidy

## clean: Remove ./bin/
clean:
	rm -rf bin
