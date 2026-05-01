BINARY      ?= file-viewer
PKG         := ./...
CMD         := ./cmd/file-viewer
GO          ?= go
GOLANGCI    ?= golangci-lint
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)

.PHONY: all build run fmt vet lint test tidy clean install release-build help

all: fmt vet lint test build

help:
	@echo "make build       - build local binary"
	@echo "make run ARGS=.. - run with args"
	@echo "make fmt         - go fmt"
	@echo "make vet         - go vet"
	@echo "make lint        - golangci-lint run"
	@echo "make test        - go test"
	@echo "make tidy        - go mod tidy"
	@echo "make release-build OS=linux ARCH=amd64"

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(CMD)

run: build
	./bin/$(BINARY) $(ARGS)

fmt:
	$(GO) fmt $(PKG)

vet:
	$(GO) vet $(PKG)

lint:
	@command -v $(GOLANGCI) >/dev/null 2>&1 || { \
	  echo "golangci-lint not found; install from https://golangci-lint.run/usage/install/"; exit 1; }
	$(GOLANGCI) run

test:
	$(GO) test -race -count=1 $(PKG)

tidy:
	$(GO) mod tidy

clean:
	rm -rf bin dist

install:
	$(GO) install -ldflags "$(LDFLAGS)" $(CMD)

# Cross-compile a release artifact. Use OS/ARCH variables.
OS    ?= $(shell go env GOOS)
ARCH  ?= $(shell go env GOARCH)
release-build:
	mkdir -p dist
	GOOS=$(OS) GOARCH=$(ARCH) CGO_ENABLED=0 \
	  $(GO) build -trimpath -ldflags "$(LDFLAGS)" \
	  -o dist/$(BINARY)-$(OS)-$(ARCH)$(if $(filter windows,$(OS)),.exe,) $(CMD)
