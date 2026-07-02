# Codex-Owned Review Phase 2-5 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the no-LLM Codex review path with comment validation, reporting, target-aware context, scan manifests, run records, viewer compatibility, and the final Skill cutover while preserving every native OCR command.

**Architecture:** Extend the agent-neutral `internal/reviewbundle` protocol with strict loaders, deterministic validation/reporting, target-bound context, scan manifests, and agent run records. Reuse native diff, scan provider, filtering, rules, Git runner, and viewer storage primitives; keep `ocr review` and `ocr scan` unchanged. The CLI remains read-only except for explicit `--output` and `--session` persistence.

**Tech Stack:** Go 1.25, standard-library JSON/Markdown, existing OCR diff/scan/tool/session/viewer packages, JSON Schema, and Go tests.

---

### Task 1: Validate Codex comments against immutable bundle evidence

**Files:**
- Create: `internal/reviewbundle/load.go`
- Create: `internal/reviewbundle/validate.go`
- Create: `internal/reviewbundle/validate_test.go`
- Modify: `cmd/opencodereview/codex_cmd.go`
- Modify: `cmd/opencodereview/codex_cmd_test.go`

- [x] **Step 1: Write failing validation tests**

Create real temporary repositories and assert strict JSON decoding, schema versions, bundle ID equality, workspace staleness, range/commit ref stability, safe repository-relative paths, known files, valid one-based line ranges, changed-hunk membership, exact `existing_code` matching, ambiguous matches, and valid suggestions. Assert stable error codes including `invalid_schema`, `bundle_id_mismatch`, `stale_bundle`, `unknown_path`, `path_escape`, `invalid_line_range`, `outside_changed_hunk`, `existing_code_mismatch`, and `ambiguous_existing_code`.

- [x] **Step 2: Verify RED**

Run `rtk go test ./internal/reviewbundle -run 'TestValidate|TestLoad'`.
Expected: FAIL because `LoadBundle`, `LoadComments`, and `ValidateComments` do not exist.

- [x] **Step 3: Implement strict loading and deterministic validation**

Add:

```go
func LoadBundle(reader io.Reader) (*Bundle, error)
func LoadComments(reader io.Reader) (*Comments, error)
func ValidateComments(ctx context.Context, bundle *Bundle, comments *Comments, repoDir string, runner *gitcmd.Runner) ValidationResult
```

Use `json.Decoder.DisallowUnknownFields`, recompute target state with `ResolveTarget`, compare immutable identities, validate paths without following escaping symlinks, and return structured notices without mutating input.

- [x] **Step 4: Expose the CLI and verify GREEN**

Add `ocr agent validate-comments --bundle FILE --comments FILE [--repo PATH] [--output FILE]`. Default output is JSON stdout; invalid comments produce a valid result with `valid:false`, not malformed output. Run `rtk go test ./internal/reviewbundle ./cmd/opencodereview -run 'TestCodex|TestValidate|TestLoad'`.

### Task 2: Produce stable JSON, text, and Markdown reports

**Files:**
- Create: `internal/reviewbundle/report.go`
- Create: `internal/reviewbundle/report_test.go`
- Modify: `cmd/opencodereview/codex_cmd.go`
- Modify: `cmd/opencodereview/codex_cmd_test.go`

- [x] **Step 1: Write failing report tests**

Assert deterministic priority/path/line ordering, Markdown escaping, JSON round-trip preservation, no-findings output, validation-error rendering, warning/partial-failure rendering, and no source writes.

- [x] **Step 2: Verify RED**

Run `rtk go test ./internal/reviewbundle -run TestReport`.
Expected: FAIL because `RenderReport` does not exist.

- [x] **Step 3: Implement reports and CLI**

Add:

```go
type ReportOptions struct { Format string; Validation *ValidationResult }
func RenderReport(bundle *Bundle, comments *Comments, options ReportOptions) ([]byte, error)
```

Expose `ocr agent report --bundle FILE --comments FILE --format markdown|text|json [--validation FILE] [--output FILE]`. Report generation never relocates or edits findings.

- [x] **Step 4: Verify GREEN**

Run `rtk go test ./internal/reviewbundle ./cmd/opencodereview -run 'TestCodex|TestReport'`.

### Task 3: Add bundle-bound read-only context commands

**Files:**
- Create: `internal/reviewbundle/context.go`
- Create: `internal/reviewbundle/context_test.go`
- Modify: `cmd/opencodereview/codex_cmd.go`
- Modify: `cmd/opencodereview/codex_cmd_test.go`

- [x] **Step 1: Write failing context tests**

Cover workspace stale rejection, range/commit reads from `head_sha`, bounded line reads, exact diff lookup, file finding, literal/regex search, result limits, path traversal, symlink escape, invalid refs, and non-Git scan bundles.

- [x] **Step 2: Verify RED**

Run `rtk go test ./internal/reviewbundle -run TestContext`.
Expected: FAIL because context APIs do not exist.

- [x] **Step 3: Implement context services**

Add a `ContextService` that validates bundle freshness before each call, constructs `tool.FileReader` with `Ref=bundle.Target.HeadSHA` for range/commit, uses the bundle patch for diff, and reuses `file_read`, `file_find`, and `code_search` providers. Return structured JSON envelopes with `bundle_id`, operation, and result.

- [x] **Step 4: Expose and verify CLI**

Add `ocr agent context read|find|diff|search --bundle FILE` with operation-specific flags. Run `rtk go test ./internal/reviewbundle ./cmd/opencodereview -run 'TestCodex|TestContext'`.

### Task 4: Add scan preparation, budgets, grouping, and manifests

**Files:**
- Create: `internal/reviewbundle/scan.go`
- Create: `internal/reviewbundle/scan_test.go`
- Modify: `internal/scan/batch.go`
- Modify: `internal/scan/estimate.go`
- Modify: `cmd/opencodereview/codex_cmd.go`
- Modify: `cmd/opencodereview/codex_cmd_test.go`
- Modify: `internal/reviewbundle/schemas/codex-review-bundle-v1.json`

- [x] **Step 1: Write failing scan tests**

Cover Git and non-Git roots, path narrowing, include/exclude rules, binary and oversized files, deterministic `none`/`by-language`/`by-directory` groups, batch size, hard byte/token budgets, explicit skipped-file reasons, no duplicates, manifest/bundle linkage, preview estimates, and deterministic IDs.

- [x] **Step 2: Verify RED**

Run `rtk go test ./internal/reviewbundle -run TestPrepareScan`.
Expected: FAIL because scan preparation APIs do not exist.

- [x] **Step 3: Export native deterministic scan helpers**

Expose stable wrappers from `internal/scan`:

```go
func ParseBatchStrategy(string) BatchStrategy
func GroupBatches([]model.ScanItem, BatchStrategy, int) [][]model.ScanItem
func EstimateTokens([]model.ScanItem, bool, bool, bool) Estimate
func EstimateItemTokens(model.ScanItem, bool) int64
```

Keep native callers using the same implementations.

- [x] **Step 4: Implement scan manifest preparation**

Add scan target/protocol fields, full-file evidence hashes, grouping metadata, estimated tokens, skipped entries, partial state, and ordered bundle parts. Enforce budgets before inclusion and never report skipped files as reviewed.

- [x] **Step 5: Expose and verify CLI**

Extend `ocr agent prepare` with `--scan`, repeatable/comma-separated `--path`, `--include`, `--max-file-size-bytes`, `--max-tokens-budget`, `--batch`, and `--batch-size`. Run focused scan and native scan tests.

### Task 5: Persist agent runs and make viewer records compatible

**Files:**
- Create: `internal/session/codex.go`
- Create: `internal/session/codex_test.go`
- Modify: `internal/viewer/store.go`
- Modify: `internal/viewer/store_test.go`
- Modify: `cmd/opencodereview/codex_cmd.go`
- Modify: `cmd/opencodereview/codex_cmd_test.go`

- [x] **Step 1: Write failing session/viewer tests**

Assert opt-in persistence, mode `agent`, bundle/findings/validation/report correlation IDs, file counts, durations, warnings, partial failures, context calls, unavailable token metrics, restrictive permissions, and unchanged parsing of native `ocr-llm` sessions.

- [x] **Step 2: Verify RED**

Run `rtk go test ./internal/session ./internal/viewer -run Codex`.
Expected: FAIL because Codex run records are unsupported.

- [x] **Step 3: Implement append-only records and viewer adapter**

Write the existing JSONL location only when `--session` is explicitly supplied. Add `controlPlane`, `bundleId`, `runId`, and Codex event records without changing native LLM records. Extend viewer summaries/cards to recognize Codex records and represent token usage as unavailable instead of zero usage.

- [x] **Step 4: Verify GREEN**

Run `rtk go test ./internal/session ./internal/viewer ./cmd/opencodereview -run 'Codex|Viewer|Session'`.

### Task 6: Cut over the Codex Skill and document parity

**Files:**
- Modify: `skills/open-code-review/SKILL.md`
- Modify: `plugins/open-code-review/skills/open-code-review/SKILL.md`
- Modify: `plugins/open-code-review/.codex-plugin/plugin.json`
- Modify: `README.md`
- Modify: `plugins/open-code-review/CODEX.ko-KR.md`
- Modify: `docs/CODEX_SKILL_FEASIBILITY.md`
- Create: `docs/CODEX_PARITY_MATRIX.md`

- [x] **Step 1: Write static integration tests**

Assert both Skills use `ocr agent prepare`, validation, report, target-aware context, scan planning/dedup/summary instructions, prompt-injection boundaries, and authorized-edit rules; reject default `ocr review`, `ocr scan`, `ocr llm test`, and provider requirements.

- [x] **Step 2: Verify RED**

Run `rtk go test ./cmd/opencodereview -run TestCodexSkill`.
Expected: FAIL against the current external-LLM Skill.

- [x] **Step 3: Update Skills, plugin metadata, and docs**

Make Codex the sole review decision-maker. Preserve explicit documentation for native `ocr review` and `ocr scan`. Record each parity item with its implementation and automated test; distinguish functional parity from model-quality benchmark work.

- [x] **Step 4: Verify the complete branch**

Run:

```bash
rtk gofmt -w cmd/opencodereview internal/reviewbundle internal/session internal/viewer internal/scan
rtk go test ./...
rtk go vet ./...
rtk go build ./cmd/opencodereview
rtk git diff --check
```

Inspect Codex production paths to confirm they do not call `loadLLMRuntime`, `NewLLMClient`, provider resolution, source-writing functions, commit, or push.
