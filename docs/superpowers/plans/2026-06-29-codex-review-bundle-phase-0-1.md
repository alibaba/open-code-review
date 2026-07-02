# Codex Review Bundle Phase 0/1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a read-only, no-LLM `ocr agent prepare` command that emits a versioned review bundle for workspace, range, and commit targets without changing the native OCR review path.

**Architecture:** `internal/reviewbundle` owns the agent-neutral protocol, target resolution, stable hashing, rule deduplication, hunk metadata, and bundle-size enforcement. The CLI loads only deterministic repository/rule state, calls the preparer, and writes JSON to stdout unless `--output` is explicit. Existing diff providers remain the source of truth, while review filtering is extracted into an agent-neutral package shared by native review and bundle generation.

**Tech Stack:** Go 1.25, standard library JSON/SHA-256/embed packages, existing `internal/diff`, `internal/config/rules`, `internal/gitcmd`, and Go tests.

---

### Task 1: Extract the native review filter into a shared deterministic package

**Files:**
- Create: `internal/reviewfilter/filter.go`
- Create: `internal/reviewfilter/filter_test.go`
- Modify: `internal/agent/preview.go`

- [x] **Step 1: Write failing filter classification tests**

Cover binary files, explicit excludes, explicit includes overriding the extension allowlist, unsupported extensions, default excluded paths, deleted files, effective paths, and rename status. Tests instantiate `reviewfilter.Filter` with real `rules.FileFilter` values and assert exact `model.ExcludeReason` and status values.

- [x] **Step 2: Run the focused test and verify RED**

Run:

```bash
rtk go test ./internal/reviewfilter
```

Expected: FAIL because `internal/reviewfilter` does not exist.

- [x] **Step 3: Implement the shared filter**

Create:

```go
type Filter struct {
	FileFilter *rules.FileFilter
}

func (f Filter) ExcludeReason(change model.Diff) model.ExcludeReason
func EffectivePath(change model.Diff) string
func Status(change model.Diff) string
```

The implementation must preserve the exact ordering currently used by `agent.whyExcluded`: binary, user exclude, user include override, extension allowlist, and default excluded path. Deleted-file classification remains a caller decision so native review behavior is unchanged.

- [x] **Step 4: Run tests and refactor the native agent**

Replace the algorithm in `internal/agent/preview.go` with calls to `reviewfilter.Filter`, `reviewfilter.EffectivePath`, and `reviewfilter.Status`, retaining compatibility wrappers used by existing tests.

Run:

```bash
rtk go test ./internal/reviewfilter ./internal/agent
```

Expected: PASS.

### Task 2: Define and embed the Phase 0 protocols

**Files:**
- Create: `internal/reviewbundle/bundle.go`
- Create: `internal/reviewbundle/schema.go`
- Create: `internal/reviewbundle/schema_test.go`
- Create: `internal/reviewbundle/schemas/codex-review-bundle-v1.json`
- Create: `internal/reviewbundle/schemas/codex-review-comments-v1.json`
- Create: `docs/CODEX_REVIEW_BUNDLE_SCHEMA.md`

- [x] **Step 1: Write failing protocol tests**

Tests assert:

```go
BundleSchemaVersion == "codex-review-bundle/v1"
CommentsSchemaVersion == "codex-review-comments/v1"
```

They must decode both embedded schemas as JSON, assert their `$id`, require `additionalProperties: false` at protocol object boundaries, and marshal a complete bundle fixture using the documented snake_case names.

- [x] **Step 2: Run the focused test and verify RED**

Run:

```bash
rtk go test ./internal/reviewbundle -run 'TestEmbedded|TestBundleJSON'
```

Expected: FAIL because the package and embedded schemas do not exist.

- [x] **Step 3: Implement protocol types and schemas**

Define separate bundle and comment protocol types. The bundle contains:

```text
schema_version, bundle_id, target, workspace_state, summary,
rules, files, contract, warnings
```

Each file contains status, reviewability, exclusion reason, hashes, deduplicated rule ID, raw patch, and hunk ranges. The comments schema includes bundle identity, summary, exact line range, priority, category, evidence fields, optional suggestion, and confidence.

- [x] **Step 4: Document canonical hashing and error semantics**

`docs/CODEX_REVIEW_BUNDLE_SCHEMA.md` must define length-prefixed SHA-256 inputs, `sha256:` formatting, target resolution rules, workspace state components, bundle ID exclusion of timestamps and itself, 1-based new-file line numbers, size-limit failure, and the reserved validation error codes.

- [x] **Step 5: Run protocol tests**

Run:

```bash
rtk go test ./internal/reviewbundle
```

Expected: PASS.

### Task 3: Prepare deterministic bundles for all diff targets

**Files:**
- Create: `internal/reviewbundle/target.go`
- Create: `internal/reviewbundle/target_test.go`
- Create: `internal/reviewbundle/prepare.go`
- Create: `internal/reviewbundle/prepare_test.go`

- [x] **Step 1: Write failing target-resolution tests**

Use temporary real Git repositories to assert:

- workspace resolves current `HEAD` and distinct staged, unstaged, and untracked hashes;
- range resolves `merge-base(from,to)` and the target commit SHA;
- commit resolves `commit^` and the commit SHA;
- option-like or missing refs fail before diff loading.

- [x] **Step 2: Run target tests and verify RED**

Run:

```bash
rtk go test ./internal/reviewbundle -run 'TestResolveTarget'
```

Expected: FAIL because target resolution is missing.

- [x] **Step 3: Implement target resolution**

Add:

```go
type TargetSpec struct {
	From   string
	To     string
	Commit string
}

func ResolveTarget(ctx context.Context, repoDir string, spec TargetSpec, runner *gitcmd.Runner) (Target, *WorkspaceState, error)
```

All Git revisions must use `--end-of-options`; all subprocesses must use the shared runner. Workspace state hashing must be deterministic and must not follow an untracked symlink outside the repository.

- [x] **Step 4: Write failing bundle preparation tests**

Tests cover modified, staged, untracked, renamed, deleted, binary, symlink, and no-final-newline changes. Assertions verify filtering parity, rule source/pattern/content deduplication, hunk ranges, raw patches, per-file content hashes, aggregate counts, stable bundle IDs, and explicit `bundle_too_large` failure.

- [x] **Step 5: Run preparation tests and verify RED**

Run:

```bash
rtk go test ./internal/reviewbundle -run 'TestPrepare'
```

Expected: FAIL because `Prepare` is missing.

- [x] **Step 6: Implement minimal preparation**

Add:

```go
type PrepareOptions struct {
	RepoDir       string
	Target        TargetSpec
	Resolver      rules.Resolver
	FileFilter    *rules.FileFilter
	GitRunner     *gitcmd.Runner
	MaxBundleSize int64
}

func Prepare(ctx context.Context, options PrepareOptions) (*Bundle, []byte, error)
```

Select the existing workspace/range/commit diff provider, classify every returned diff with the shared filter, resolve `rules.DetailResolver` metadata, deduplicate rules by canonical content and metadata, parse hunks with `diff.ParseHunks`, compute stable hashes, marshal canonical JSON, and fail rather than truncate when the limit is exceeded.

- [x] **Step 7: Run package tests**

Run:

```bash
rtk go test ./internal/reviewbundle ./internal/diff ./internal/config/rules ./internal/agent
```

Expected: PASS.

### Task 4: Expose `ocr agent prepare` without loading an OCR LLM runtime

**Files:**
- Create: `cmd/opencodereview/codex_cmd.go`
- Create: `cmd/opencodereview/codex_cmd_test.go`
- Modify: `cmd/opencodereview/main.go`

- [x] **Step 1: Write failing CLI tests**

Tests assert:

- `dispatch` recognizes `codex`;
- missing and conflicting target flags fail consistently with native review;
- `prepare --format json` succeeds with an empty temporary home and no OCR LLM configuration;
- stdout is valid bundle JSON;
- `--output` writes mode `0600` and does not emit the bundle to stdout;
- `--preview` emits only a human-readable manifest and no patch body;
- output exceeding `--max-bundle-bytes` fails explicitly.

- [x] **Step 2: Run CLI tests and verify RED**

Run:

```bash
rtk go test ./cmd/opencodereview -run 'TestCodex'
```

Expected: FAIL because the command is not registered.

- [x] **Step 3: Implement command parsing and output**

Implement `runAgent`, `parseCodexPrepareFlags`, and `executeCodexPrepare`. Reuse `loadCommonContext` only for deterministic template/rules/repository state; never call `loadLLMRuntime`, create an LLM client, read model credentials, modify source files, or commit. Default output is JSON on stdout, and explicit output uses a user-selected path with restrictive permissions.

- [x] **Step 4: Update usage and run focused tests**

Run:

```bash
rtk go test ./cmd/opencodereview -run 'TestCodex|TestDispatch|TestTopLevel'
```

Expected: PASS.

### Task 5: Verify native capability preservation

**Files:**
- Modify only if verification exposes a regression.

- [x] **Step 1: Format and inspect**

Run:

```bash
rtk gofmt -w cmd/opencodereview/codex_cmd.go cmd/opencodereview/codex_cmd_test.go internal/reviewfilter internal/reviewbundle
rtk git diff --check
```

Expected: no formatting or whitespace errors.

- [x] **Step 2: Run the complete test suite**

Run:

```bash
rtk go test ./...
```

Expected: PASS with zero failures.

- [x] **Step 3: Run static analysis**

Run:

```bash
rtk go vet ./...
```

Expected: exit code 0.

- [x] **Step 4: Verify no-LLM and read-only boundaries**

Inspect the Codex command dependency path and test repository status before and after `ocr agent prepare`. Confirm that it does not call `loadLLMRuntime`, write repository files without `--output`, modify source files, or affect the existing `review`/`scan` dispatch paths.

- [x] **Step 5: Record the implementation checkpoint**

Do not commit automatically. Report changed files, exact verification evidence, remaining Phase 2–5 work, and any parity gaps still preventing Skill cutover.
