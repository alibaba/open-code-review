.PHONY: build test clean run help fmt vet check \
	build-all dist sha256sum version-info \
	build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 \
	build-windows-amd64 build-windows-arm64

BINARY_NAME := opencodereview
GO          := go
DIST_DIR    := ./dist

# Version info — use git tag if available, fallback to short commit hash
GIT_TAG     := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "")
GIT_COMMIT  := $(shell git rev-parse --short HEAD)
BUILD_DATE  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

VERSION     ?= $(if $(GIT_TAG),$(GIT_TAG),v0.0.0-$(GIT_COMMIT))

LD_FLAGS    := -s -w \
	-X main.Version=$(VERSION) \
	-X main.GitCommit=$(GIT_COMMIT) \
	-X main.BuildDate=$(BUILD_DATE)

define BUILD_PLATFORM
	GOOS=$(1) GOARCH=$(2) CGO_ENABLED=0 $(GO) build -ldflags "$(LD_FLAGS)" \
		-o $(DIST_DIR)/$(BINARY_NAME)-$(1)-$(2)$(3) \
		./cmd/opencodereview
endef

# ── Development targets ──────────────────────────────────────────────────────
build:
	$(GO) build -ldflags "$(LD_FLAGS)" -o $(DIST_DIR)/$(BINARY_NAME) ./cmd/opencodereview

test:
	LC_ALL=C $(GO) test -v -race -count=1 ./...

clean:
	rm -rf $(DIST_DIR)

run: build
	$(DIST_DIR)/$(BINARY_NAME) --staged

help: build
	$(DIST_DIR)/$(BINARY_NAME) -h

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

check:
	$(GO) mod tidy
	$(GO) fmt ./...
	$(GO) vet ./...
	@echo "check passed"

# ── Cross-platform targets ───────────────────────────────────────────────────
build-linux-amd64:
	$(call BUILD_PLATFORM,linux,amd64)

build-linux-arm64:
	$(call BUILD_PLATFORM,linux,arm64)

build-darwin-amd64:
	$(call BUILD_PLATFORM,darwin,amd64)

build-darwin-arm64:
	$(call BUILD_PLATFORM,darwin,arm64)

build-windows-amd64:
	$(call BUILD_PLATFORM,windows,amd64,.exe)

build-windows-arm64:
	$(call BUILD_PLATFORM,windows,arm64,.exe)

build-all: build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-windows-amd64 build-windows-arm64

# Generate SHA256 checksums for all release binaries
sha256sum: build-all
	cd $(DIST_DIR) && shasum -a 256 $(BINARY_NAME)-* | sort > sha256sum.txt

# Full release: clean → build all platforms → checksums
dist: clean build-all sha256sum
	@echo $(VERSION) > $(DIST_DIR)/VERSION

version-info:
	@echo "Version:   $(VERSION)"
	@echo "GitCommit: $(GIT_COMMIT)"
	@echo "BuildDate: $(BUILD_DATE)"
	@echo "LD_FLAGS:  $(LD_FLAGS)"
