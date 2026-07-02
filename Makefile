GO ?= go
MODULE := github.com/Icemap/tdc
BINARY_NAME := tdc
BIN_DIR := bin
TDC_BIN := $(BIN_DIR)/$(BINARY_NAME)
LIVE_E2E_PROFILE ?= live-e2e

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

LDFLAGS := -s -w \
	-X $(MODULE)/internal/version.version=$(VERSION) \
	-X $(MODULE)/internal/version.commit=$(COMMIT) \
	-X $(MODULE)/internal/version.date=$(DATE)

.PHONY: all build test e2e live-e2e clean

all: build

build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(TDC_BIN) ./cmd/tdc

test:
	$(GO) test ./...

e2e: build
	TDC_E2E_BIN="$(abspath $(TDC_BIN))" $(GO) test ./e2e -count=1 -v

live-e2e: build
	TDC_E2E_BIN="$(abspath $(TDC_BIN))" TDC_LIVE=1 TDC_PROFILE="$(LIVE_E2E_PROFILE)" $(GO) test ./e2e -count=1 -v -run '^TestLive'

clean:
	rm -rf $(BIN_DIR)
