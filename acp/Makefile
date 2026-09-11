.PHONY: build test check coverage license-check english-check clean

# Minimum total statement coverage enforced by `make coverage`.
# Mirrors COVERAGE_THRESHOLD in the repository root Makefile.
COVERAGE_MIN ?= 90

# build target disabled until cmd/ocr-acp is implemented in phase 4
# build:
# 	go build -o ../dist/ocr-acp ./cmd/ocr-acp

test:
	LC_ALL=C go test -v -race -count=1 ./...

# Unlike the root `check`, which formats in place with `gofmt -s -w .`, this
# one fails on unformatted files instead of fixing them, so it can run as a
# gate. `gofmt -s` matches the flag CI uses.
check: license-check english-check
	@unformatted=$$(gofmt -s -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt: the following files need formatting (run 'gofmt -s -w .'):"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	LC_ALL=C go vet ./...
	@sc=$$(command -v staticcheck || echo "$$(go env GOPATH)/bin/staticcheck"); \
	if [ ! -x "$$sc" ]; then \
		echo "staticcheck: not installed, SKIPPED (CI installs it; see .github/workflows/acp-ci.yml)"; \
		exit 0; \
	fi; \
	out=$$("$$sc" ./... 2>&1); \
	status=$$?; \
	printf '%s\n' "$$out"; \
	if printf '%s' "$$out" | grep -q 'matched no packages'; then \
		echo "staticcheck: analyzed no packages; refusing to report a pass."; \
		echo "staticcheck 0.8.x requires a Go >= 1.26 toolchain, but this module declares go 1.23."; \
		exit 1; \
	fi; \
	exit $$status

coverage:
	LC_ALL=C go test -race -count=1 -coverprofile=coverage.out ./...
	@actual=$$(go tool cover -func=coverage.out | awk '/^total:/ {gsub(/%/, "", $$3); print $$3}'); \
	awk -v actual="$$actual" -v minimum="$(COVERAGE_MIN)" 'BEGIN { \
		if (actual == "") { \
			print "FAIL: could not parse total coverage from coverage.out"; \
			exit 1; \
		} \
		printf "total coverage: %s%% (minimum: %s%%)\n", actual, minimum; \
		if (actual + 0 < minimum + 0) { \
			print "FAIL: coverage is below the minimum"; \
			exit 1; \
		} \
	}'

# verify-license.sh takes no path argument and always scans the whole repository
# via `git ls-files`, so this checks more than just the acp module.
license-check:
	@bash ../scripts/verify-license.sh

# Passed "." so the check stays scoped to this module. A missing script is a
# hard failure: this module lives inside the OCR repository, so the script is
# always expected to be there, and silently skipping would let `make check`
# exit 0 without having verified anything.
english-check:
	@if [ ! -f ../scripts/verify-english-only.go ]; then \
		echo "english-check: ../scripts/verify-english-only.go is missing"; \
		exit 1; \
	fi
	go run ../scripts/verify-english-only.go .

clean:
	rm -f coverage.out
	rm -f testdata/mock-ocr/mock-ocr
	rm -f ../dist/ocr-acp
