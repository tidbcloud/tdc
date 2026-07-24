GO ?= go
GORELEASER ?= goreleaser
MODULE := github.com/tidbcloud/tdc
BINARY_NAME := tdc
BIN_DIR := bin
TDC_BIN := $(BIN_DIR)/$(BINARY_NAME)
TELEMETRY_BACKEND_BIN := $(BIN_DIR)/tdc-telemetry-backend
LIVE_E2E_PROFILE ?= live-e2e
LIVE_E2E_RUN = TDC_E2E_BIN="$(abspath $(TDC_BIN))" TDC_LIVE=1 TDC_PROFILE="$(LIVE_E2E_PROFILE)" $(GO) test ./e2e -count=1 -v -timeout 30m

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

LDFLAGS := -s -w \
	-X $(MODULE)/internal/version.version=$(VERSION) \
	-X $(MODULE)/internal/version.commit=$(COMMIT) \
	-X $(MODULE)/internal/version.date=$(DATE) \
	-X $(MODULE)/internal/version.installSource=local \
	-X $(MODULE)/internal/version.releaseChannel=stable

.PHONY: all build build-telemetry-backend test e2e live-e2e live-e2e-configure live-e2e-organization live-e2e-db live-e2e-fs live-e2e-fs-git live-e2e-fs-journal live-e2e-fs-vault release-snapshot clean

all: build

build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(TDC_BIN) ./cmd/tdc

build-telemetry-backend:
	@mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -o $(TELEMETRY_BACKEND_BIN) ./cmd/tdc-telemetry-backend

test:
	$(GO) test ./...

e2e: build
	TDC_E2E_BIN="$(abspath $(TDC_BIN))" $(GO) test ./e2e -count=1 -v

live-e2e: build
	$(LIVE_E2E_RUN) -run '^TestLive'

live-e2e-configure: build
	$(LIVE_E2E_RUN) -run '^TestLive(ProfileConfigured|CLICommandSurface)$$'

live-e2e-organization: build
	$(LIVE_E2E_RUN) -run '^TestLiveOrganization'

live-e2e-db: build
	$(LIVE_E2E_RUN) -run '^TestLiveDB'

live-e2e-fs: build
	$(LIVE_E2E_RUN) -run '^TestLive(FSResourceRegistryLifecycle|FSCommandSurface|FSConfigurationFreeAccess|FSDataPlaneLifecycle|FSMountRuntime|FSWebDAVMountRuntime)$$'

live-e2e-fs-git: build
	$(LIVE_E2E_RUN) -run '^TestLiveFSGit'

live-e2e-fs-journal: build
	$(LIVE_E2E_RUN) -run '^TestLiveFSJournal'

live-e2e-fs-vault: build
	$(LIVE_E2E_RUN) -run '^TestLiveFSVault'

release-snapshot:
	$(GORELEASER) release --snapshot --clean

clean:
	rm -rf $(BIN_DIR) dist
