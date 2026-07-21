# kuberoutectl — build and release helpers.
# Version metadata is injected into internal/buildinfo via -ldflags.

BINARY  := kuberoutectl
PKG     := github.com/ymedlop/kuberoutectl
CMD     := ./cmd/kuberoutectl

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
# Commit date in UTC (RFC3339 with a literal Z), matching GoReleaser's
# {{ .CommitDate }} so a local build's `version` string is identical to a
# release build's for the same commit — and reproducible, unlike a wall-clock date.
DATE    ?= $(shell TZ=UTC git log -1 --date=format-local:%Y-%m-%dT%H:%M:%SZ --format=%cd 2>/dev/null || echo unknown)

LDFLAGS := -s -w \
	-X $(PKG)/internal/buildinfo.Version=$(VERSION) \
	-X $(PKG)/internal/buildinfo.Commit=$(COMMIT) \
	-X $(PKG)/internal/buildinfo.Date=$(DATE)

.PHONY: help build install run test vet fmt fmt-check tidy check clean dist snapshot demo verify-readme

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  %-12s %s\n", $$1, $$2}'

build: ## Build the CLI into bin/
	go build -ldflags '$(LDFLAGS)' -o bin/$(BINARY) $(CMD)

install: ## Install the CLI into GOBIN
	go install -ldflags '$(LDFLAGS)' $(CMD)

run: build ## Build then run
	./bin/$(BINARY)

test: ## Run all tests
	go test ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Format sources in place
	gofmt -w internal cmd

fmt-check: ## Fail if any source is unformatted
	@out=$$(gofmt -l internal cmd); \
	if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi

tidy: ## Tidy go.mod/go.sum
	go mod tidy

check: fmt-check vet test ## Pre-commit gate: format, vet, test

clean: ## Remove build artifacts
	rm -rf bin dist

demo: ## Regenerate the README demo GIF (assets/demo.gif) from fixtures
	bash scripts/demo.sh

verify-readme: ## Check README + demo commands still exist in the CLI
	bash scripts/verify-readme-commands.sh

# Cross-compile the snapshot deliverables for every shipped OS/arch pair.
# Mirrors the GoReleaser matrix (windows/linux/darwin × amd64/arm64).
dist: ## Cross-compile {windows,linux,darwin} × {amd64,arm64} into ./dist
	GOOS=windows GOARCH=amd64 go build -ldflags '$(LDFLAGS)' -o dist/$(BINARY)_windows_amd64/$(BINARY).exe $(CMD)
	GOOS=windows GOARCH=arm64 go build -ldflags '$(LDFLAGS)' -o dist/$(BINARY)_windows_arm64/$(BINARY).exe $(CMD)
	GOOS=linux   GOARCH=amd64 go build -ldflags '$(LDFLAGS)' -o dist/$(BINARY)_linux_amd64/$(BINARY) $(CMD)
	GOOS=linux   GOARCH=arm64 go build -ldflags '$(LDFLAGS)' -o dist/$(BINARY)_linux_arm64/$(BINARY) $(CMD)
	GOOS=darwin  GOARCH=amd64 go build -ldflags '$(LDFLAGS)' -o dist/$(BINARY)_darwin_amd64/$(BINARY) $(CMD)
	GOOS=darwin  GOARCH=arm64 go build -ldflags '$(LDFLAGS)' -o dist/$(BINARY)_darwin_arm64/$(BINARY) $(CMD)

snapshot: ## Build a local snapshot release with GoReleaser (reproducible)
	SOURCE_DATE_EPOCH=$$(git log -1 --format=%ct) goreleaser release --snapshot --clean
