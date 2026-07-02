.PHONY: build test clean run help fmt vet check coverage \
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

LD_FLAGS    := \
	-X main.Version=$(VERSION) \
	-X main.GitCommit=$(GIT_COMMIT) \
	-X main.BuildDate=$(BUILD_DATE)

RELEASE_LD_FLAGS := -s -w $(LD_FLAGS)

define BUILD_PLATFORM
	GOOS=$(1) GOARCH=$(2) CGO_ENABLED=0 $(GO) build -ldflags "$(RELEASE_LD_FLAGS)" \
		-o $(DIST_DIR)/$(BINARY_NAME)-$(1)-$(2)$(3) \
		./cmd/opencodereview
endef

# ── Development targets ──────────────────────────────────────────────────────
build:
	$(GO) build -ldflags "$(LD_FLAGS)" -o $(DIST_DIR)/$(BINARY_NAME) ./cmd/opencodereview

PACKAGES := $(shell $(GO) list ./... | grep -v /extensions/)

test:
	LC_ALL=C $(GO) test -v -race -count=1 $(PACKAGES)

COVERAGE_THRESHOLD := 80

coverage:
	LC_ALL=C $(GO) test -count=1 -coverprofile=coverage.out $(PACKAGES)
	$(GO) tool cover -func=coverage.out | grep total:
	@COVERAGE=$$($(GO) tool cover -func=coverage.out | grep total: | awk '{print $$3}' | sed 's/%//'); \
	if awk "BEGIN {exit !($$COVERAGE < $(COVERAGE_THRESHOLD))}"; then \
		echo "FAIL: Coverage $${COVERAGE}% is below $(COVERAGE_THRESHOLD)% threshold"; \
		exit 1; \
	fi; \
	echo "PASS: Coverage $${COVERAGE}% meets $(COVERAGE_THRESHOLD)% threshold"

clean:
	rm -rf $(DIST_DIR) coverage.out

run: build
	$(DIST_DIR)/$(BINARY_NAME) --staged

help: build
	$(DIST_DIR)/$(BINARY_NAME) -h

fmt:
	$(GO) fmt $(PACKAGES)

vet:
	LC_ALL=C $(GO) vet $(PACKAGES)

check:
	$(GO) mod tidy
	$(GO) fmt $(PACKAGES)
	LC_ALL=C $(GO) vet $(PACKAGES)
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
