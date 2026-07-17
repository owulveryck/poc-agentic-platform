# PPG — Platform Planning Gateway
#
# Common workflow:
#   make quickstart    # build + one-minute guided demo on the examples/ corpus
#   make install       # build + install binaries into ~/.local/bin
#   make test          # run all tests
#
# Override the install location:
#   make install BINDIR=/usr/local/bin

BINDIR ?= $(HOME)/.local/bin
GO     ?= go

.PHONY: help build quickstart install uninstall setup-claude-code remove-claude-code \
        setup-github-copilot remove-github-copilot test tidy clean

## help: Show this help.
help:
	@awk 'BEGIN{FS=":.*##"; printf "Targets:\n"} \
	  /^##/ {sub(/^## */,""); printf "  %s\n", $$0}' $(MAKEFILE_LIST)

## build: Build all binaries into ./bin/
build:
	@mkdir -p bin
	$(GO) build -o bin/ppg               ./cmd/ppg
	$(GO) build -o bin/ppg-mcp-server    ./adapters/claudecode/mcpserver
	$(GO) build -o bin/ppg-guard         ./adapters/claudecode/guard
	$(GO) build -o bin/ppg-copilot-guard ./adapters/copilot/guard
	$(GO) build -o bin/ppg-preflight     ./adapters/preflight
	$(GO) build -o bin/ppg-verify        ./cmd/ppg-verify
	$(GO) build -o bin/svc-mock          ./cmd/svc-mock
	@echo "Built into ./bin/"

## quickstart: Build, start a throwaway gateway on the examples/ demo corpus, and run a guided /enrich + /lock_in_plan + /discover_service tour.
quickstart: build
	@bash scripts/quickstart.sh

## install: Install binaries into $(BINDIR) (default ~/.local/bin).
install:
	@mkdir -p $(BINDIR)
	$(GO) build -o $(BINDIR)/ppg               ./cmd/ppg
	$(GO) build -o $(BINDIR)/ppg-mcp-server    ./adapters/claudecode/mcpserver
	$(GO) build -o $(BINDIR)/ppg-guard         ./adapters/claudecode/guard
	$(GO) build -o $(BINDIR)/ppg-copilot-guard ./adapters/copilot/guard
	$(GO) build -o $(BINDIR)/ppg-preflight     ./adapters/preflight
	$(GO) build -o $(BINDIR)/ppg-verify        ./cmd/ppg-verify
	$(GO) build -o $(BINDIR)/svc-mock          ./cmd/svc-mock
	@echo "Installed to $(BINDIR)"

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

## tidy: Run go mod tidy.
tidy:
	$(GO) mod tidy

## clean: Remove ./bin/
clean:
	rm -rf bin
