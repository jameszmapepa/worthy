# repo-health — common developer tasks. Run `make` (or `make help`) for the list.

BINARY := repohealth
PKG    := ./cmd/repohealth

# Override the target repository with: make run REPO=owner/repo
REPO ?= charmbracelet/bubbletea

.DEFAULT_GOAL := help

.PHONY: help build run test test-race cover vet fmt fmt-check tidy check clean

help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-11s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary into ./bin
	go build -o bin/$(BINARY) $(PKG)

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

vet: ## Run go vet
	go vet ./...

fmt: ## Format all Go files in place
	gofmt -w .

fmt-check: ## Fail if any Go file is unformatted
	@test -z "$$(gofmt -l .)" || { echo "unformatted:"; gofmt -l .; exit 1; }

tidy: ## Tidy go.mod / go.sum
	go mod tidy

check: fmt-check vet test-race ## Pre-commit gate: fmt-check + vet + race tests

clean: ## Remove build and coverage artifacts
	rm -rf bin coverage.out coverage.html
