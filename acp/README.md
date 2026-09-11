# ocr-acp: ACP Adapter for OpenCodeReview

Independent ACP (Agent Client Protocol) adapter for OpenCodeReview CLI.

## Status

**Phase 3 complete**: CLI contract and mock OCR implemented and tested.

- ✓ Phase 1: CLI investigation and contract definition
- ✓ Phase 2: Overall design and architecture
- ✓ Phase 3: CLI Contract and mock OCR
- ⏳ Phase 4: Command parsing and intent mapping
- ⏳ Phase 5: OCR process orchestration
- ⏳ Phase 6: ACP protocol implementation
- ⏳ Phase 7: Testing and quality verification
- ⏳ Phase 8: Client integration and release
- ⏳ Phase 9: Delivery review

## Project Structure

```
acp/
├── internal/
│   └── contract/         # CLI contract and intent structures (phase 3)
├── testdata/
│   └── mock-ocr/         # Test double for OCR CLI (phase 3)
├── Makefile              # Build, test, and quality checks
└── go.mod                # Independent Go module
```

Empty placeholder directories exist for `cmd/ocr-acp/` (phase 4),
`internal/intent/` (phase 4), `internal/orchestrator/` (phase 5) and
`internal/adapter/` (phase 6). Git does not track empty directories, so they
are not part of this commit.

## Quick Start

### Test

```bash
make test
make coverage
```

`make coverage` fails if total statement coverage drops below `COVERAGE_MIN`
(default 90%). Override per-run with `make coverage COVERAGE_MIN=95`.

### Quality Checks

```bash
make check
make english-check
```

`make check` fails on unformatted files, on `go vet` findings, and on
`staticcheck` findings. It looks for `staticcheck` on `PATH` first and then in
`$(go env GOPATH)/bin`, where `go install` puts it, so a local install is
found without editing `PATH`. When the binary is genuinely absent it prints a
notice and skips it — it never reports a pass for a check it did not run.

A present-but-useless `staticcheck` is also a failure, not a pass: if it
reports `matched no packages` it analyzed nothing, which usually happens when
the binary is newer than the active Go toolchain. `staticcheck` 0.8.x needs a
Go >= 1.26 toolchain while this module declares `go 1.23`, so CI pins a
`golang:1.26.6` container for exactly that reason.

### CI

`acp/` is a nested Go module, and the root `ci.yml` resolves its package list
with `go list ./...`, which does not descend into a nested module. Nothing in
this directory is built or tested by the workflows at the repository root.

`.github/workflows/acp-ci.yml` covers it instead — the same arrangement
`pages-ci.yml` uses for `pages/`. It runs the license, formatting, vet,
staticcheck, coverage and govulncheck gates in a `golang:1.26.6` container.
Its `pull_request` trigger deliberately has no `branches:` filter, so it also
runs for pull requests into an integration branch rather than only `main`.

Because `make check` skips a missing staticcheck, the workflow asserts
`staticcheck -version` succeeds before calling it, and `make check` itself now
fails if the analyser reports `matched no packages`.

### Build

Not available yet. The `build` target is commented out until `cmd/ocr-acp`
exists in phase 4.

## CLI Contract

`internal/contract` builds OCR CLI argument vectors and models the JSON the
CLI writes back.

### Intent to arguments

`BuildReviewArgs` and `BuildScanArgs` turn an intent into an argv slice. They
always append `--format json --audience human --color never` last, so the
integration contract holds regardless of what a caller passes in `Extra`.

### Extra flag whitelist

`Extra` is not a free-form passthrough. Every element is validated against a
per-command whitelist derived from the OCR CLI's own flag registrations:

| Command | Forwardable via `Extra` |
| --- | --- |
| `review` | `--effort`, `--no-filter`, `--background`, `--background-file` |
| `scan` | `--batch`, `--no-plan`, `--no-dedup`, `--no-summary`, `--background` |

The lists are asymmetric because the CLI registers different flags on each
command — `--effort` is rejected on `scan`, `--batch` on `review`. Enum flags
(`--effort`, `--batch`) also have their values checked, because the CLI
silently falls back to a default for unknown values rather than erroring.

Everything else is rejected, including the integration flags the adapter owns
(`--format`, `--audience`, `--color`, `--output`, `--repo`, `--rule`,
`--tools`, `--resume`), the runtime overrides (`--model`, `--provider`),
`--preview`, and any bare word that is not a flag.

### Result types

`ReviewResult`, `ScanResult`, `Summary`, `Comment` and `Manifest` mirror the
real `ocr --format json` envelope (`jsonOutput` plus `model.LlmComment`):
`summary` is an object, a finding carries `content`/`start_line`/`end_line`,
and `start_line == end_line == 0` means a file-level comment. Per-comment
`thinking` is deliberately not modeled, so reasoning cannot leak by accident.
`Comment.Severity` and the `status` string are open sets: the observed values
are declared as `Status*` constants, but consumers must tolerate unknown ones.

## Mock OCR

The `testdata/mock-ocr` program simulates OCR CLI behavior for testing without
real LLM calls.

### Build Mock

```bash
go build -o testdata/mock-ocr/mock-ocr ./testdata/mock-ocr/
```

### Scenarios

`-scenario` defaults to `success-review`, so running the binary bare works.

All scenarios emit the real CLI JSON envelope (`status`, `message`, `summary`
object, `comments` with `content`/`start_line`/`end_line`, `manifest`), not the
adapter's DTO.

- `success-review`: Normal review with findings (default)
- `success-scan`: Normal scan with findings (status `success`, no manifest)
- `partial`: Partial results with a failed coverage entry (timeout simulation)
- `empty-comments`: No findings; emits `"comments":[]`
- `stderr-pollution`: Noise and a second JSON document on stderr alongside a
  valid stdout document
- `invalid-json`: Malformed JSON output
- `non-zero-exit`: `status: failed`, `"comments":null`, a failed manifest and
  exit code 1
- `block-for-cancel`: Prints `READY` to stderr, blocks, then on SIGINT/SIGTERM
  emits a failed document whose `manifest.run_failure.classification` and
  `coverage.failed[].classification` are `cancelled`, and exits 1 to mirror the
  real CLI (which has no dedicated cancellation exit code)
- `spawn-child`: **Placeholder.** It does not actually fork a child process;
  the name overstates what it does and it is not yet usable for testing
  signal propagation to grandchildren

`-delay` pauses before producing output. If a signal arrives during the delay
the mock exits 130 without writing a result.

### Example

```bash
# Normal review
./testdata/mock-ocr/mock-ocr -scenario success-review

# With delay
./testdata/mock-ocr/mock-ocr -scenario success-review -delay 500ms

# Test cancellation
./testdata/mock-ocr/mock-ocr -scenario block-for-cancel
```

### Contract tests

`internal/contract/mock_contract_test.go` builds the mock and decodes its real
stdout into the contract types, asserting the decoded field values (content,
line numbers, summary, manifest classifications) rather than just counts.

`internal/contract/real_cli_test.go` adds two more layers:

- deterministic fixtures of the real CLI shape (summary object, unknown
  fields, `thinking`) that fail if the DTO drifts from the documented output;
- `TestRealOCRBinaryScanMultiplePaths`, an opt-in integration test. Set
  `OCR_BINARY` to run the adapter's own argv against a real binary:
  `OCR_BINARY=../dist/opencodereview go test ./internal/contract/`. It pins the
  `--path` comma-joining rule end to end (a repeated `--path` would silently
  scan only the last path).

## Design Documents

All phase documents are under `temp/archive/` (git-ignored):

- `OpenCodeReview ACP 阶段一调研文档.md`
- `OpenCodeReview ACP 阶段二总体设计.md`
- `OpenCodeReview ACP 阶段三验证文档.md`
- `OpenCodeReview ACP CLI Contract.md`
- `OpenCodeReview ACP 服务端分阶段开发文档.md`

## Dependencies

- Go 1.23+ to build this module (its `go` directive), but the `staticcheck`
  version CI pins (0.8.x) needs a Go >= 1.26 toolchain, so run `make check` on
  Go 1.26+ or expect its staticcheck step to fail fast rather than pass
  vacuously. The parent OCR module declares `go 1.25.5`; aligning the three is
  still pending.
- OCR CLI (external dependency, not imported as library)
- ACP Go SDK v0.13.5 (to be added in phase 6)

## License

Apache-2.0 (same as parent OCR project)
