.PHONY: build test vet lint fmt clean tidy run help

VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
  -X github.com/NoobCoder1209/k8s-pod-doctor/internal/version.Version=$(VERSION) \
  -X github.com/NoobCoder1209/k8s-pod-doctor/internal/version.Commit=$(COMMIT) \
  -X github.com/NoobCoder1209/k8s-pod-doctor/internal/version.Date=$(DATE)

build: ## Build the binary into ./pod-doctor
	go build -trimpath -ldflags '$(LDFLAGS)' -o pod-doctor ./cmd/pod-doctor

test: ## Run unit tests with race detector
	go test -race -count=1 ./...

vet: ## go vet
	go vet ./...

lint: ## Run golangci-lint (must be installed)
	golangci-lint run

fmt: ## go fmt
	go fmt ./...

tidy: ## go mod tidy
	go mod tidy

clean: ## Remove built binaries and dist artifacts
	rm -rf pod-doctor dist/

run: build ## Build and run with --help
	./pod-doctor --help

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
