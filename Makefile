# worthy — common developer tasks. Run `make` (or `make help`) for the list.

BINARY  := worthy
PKG     := ./cmd/worthy
# Release builds strip the symbol table and DWARF debug info (-s -w) and remove
# embedded local paths (-trimpath); ~29% smaller with no runtime behaviour change.
LDFLAGS := -s -w

# Override the target repository with: make run REPO=owner/repo
REPO ?= charmbracelet/bubbletea

.DEFAULT_GOAL := help

# golangci-lint version CI pins; keep in sync with .github/workflows/ci.yml.
GOLANGCI_VERSION := v2.12.2

.PHONY: help build release install run test test-race cover bench vet fmt fmt-check lint tidy check size clean

help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-11s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary into ./bin
	go build -o bin/$(BINARY) $(PKG)

release: ## Stripped release build into ./bin (-s -w -trimpath)
	go build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(BINARY) $(PKG)

install: ## Install the stripped binary to GOPATH/bin
	go install -trimpath -ldflags="$(LDFLAGS)" $(PKG)

run: ## Run the TUI (make run REPO=owner/repo)
	go run $(PKG) $(REPO)

test: ## Run all tests
	go test ./...

test-race: ## Run all tests with the race detector
	go test ./... -race -count=1

cover: ## Run tests and write coverage.out + coverage.html
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@go tool cover -func=coverage.out | tail -1

bench: ## Run benchmarks with allocation stats
	go test -bench=. -benchmem ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Format all Go files in place
	gofmt -w .

fmt-check: ## Fail if any Go file is unformatted
	@test -z "$$(gofmt -l .)" || { echo "unformatted:"; gofmt -l .; exit 1; }

lint: ## Run golangci-lint (see https://golangci-lint.run for install)
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not found — install $(GOLANGCI_VERSION): https://golangci-lint.run/welcome/install/"; \
		exit 1; }
	golangci-lint run ./...

tidy: ## Tidy go.mod / go.sum
	go mod tidy

check: fmt-check vet lint test-race ## Pre-commit gate: fmt-check + vet + lint + race tests

size: ## Compare default vs stripped binary size
	@go build -o bin/$(BINARY)-plain $(PKG)
	@go build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(BINARY) $(PKG)
	@ls -lh bin/$(BINARY)-plain bin/$(BINARY) | awk '{print $$5, $$9}'

clean: ## Remove build and coverage artifacts
	rm -rf bin coverage.out coverage.html
