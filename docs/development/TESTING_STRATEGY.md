Parent document: `/CLAUDE.md`
Related documents:
- `docs/development/LOCAL_DEVELOPMENT.md`
- `AGENTS.md`
- `docs/architecture/CHANGE_BLAST_RADIUS.md`

Read this when:
- You're deciding what test coverage a change needs, or interpreting a CI test failure.

Purpose:
- What kinds of tests exist, what the CI gate requires, and what the test suite does/doesn't tell you about expected behavior.

Scope:
- Included: unit/e2e test patterns observed, coverage gate, CI test matrix.
- Excluded: how to run tests locally (`LOCAL_DEVELOPMENT.md`), prompt-quality testing (explicitly noted as absent — see below).

---

# Testing Strategy

## The gate

`make test` (not raw `go test` — it sets `LC_ALL=C` so git output stays English-parseable) runs with `-race` and a **90% coverage threshold enforced via `make coverage`**. CI (`ci.yml`) runs this on a `self-hosted` Linux runner inside `golang:1.26.6`, plus a separate `windows` job **without** `-race` or the coverage gate — deliberately, since Windows-specific build tags legitimately reduce coverage on that platform.

## What the test suite reveals about expected behavior

- **`cmd/opencodereview/` is heavily test-covered per-concern**, not just end-to-end: `provider_tui_*_test.go` (10+ files) covers the interactive TUI's individual states (cpinput, customform, deleteconfirm, editsave, funcs, manualenter, modeltui, persist, rollback, savefail) — implying the TUI's state machine is complex enough to warrant per-transition tests, a signal for anyone touching `provider_tui.go`.
- **`retry_fake_llm_test.go`** confirms a fake/mock LLM client fixture exists for deterministic testing of the retry/request path without a real network call — the pattern to follow for any new LLM-path test.
- **`manual_e2e_retry_test.go`, `progress_stream_e2e_test.go`, `retry_report_e2e_test.go`** — naming implies genuine end-to-end tests exist for the retry-reporting feature specifically, consistent with the recent "clarify LLM request failures by review stage" change.
- **`bedrock_config_test.go`, `apply_provider_field_test.go`** — provider-specific config tests exist per provider family, not just generically.
- **`sarif_test.go`** exists despite SARIF having zero production integration usage (`docs/integrations/EXTERNAL_INTEGRATIONS.md`) — the format is tested even though it's unused downstream.
- **`zero_args_test.go`, `arg_errors_test.go`, `flag_suggest_test.go`** — CLI ergonomics (typo suggestions, missing-arg errors) are explicitly tested, not incidental.

## What's explicitly NOT tested (a real gap, not an oversight to paper over)

**Prompt output quality has no automated regression test.** The test suite validates loop mechanics (exit conditions, retry classification, tool dispatch, session schema) exhaustively, but nothing pins "does this prompt wording still produce good review comments." This is consistent with `docs/architecture/CHANGE_BLAST_RADIUS.md`'s ranking of prompt-template changes as the highest silent-risk category — the tests will not catch a quality regression from a prompt edit.

## CI checks beyond `go test`

`go vet`, `govulncheck` (known-vulnerability scan), `go mod tidy` cleanliness check, `gofmt -s` check, LF line-ending check, license-header check, English-only check, GH-Action-pin verification (SHA-pinned actions, not floating tags) — all part of `make check` or the CI workflow directly, not part of `make test`'s coverage-gated run.

## VS Code extension has its own CI

`vscode-ext.yml` lints/compiles/tests `extensions/vscode/` independently (Yarn-based), triggered only on PRs touching that path — not part of the Go coverage gate.

## Known gaps / uncertainties:
- No prompt-quality/regression test suite was found — confirmed absence, not just unread code, but worth a direct question to maintainers before treating this as certain.
- Whether `internal/session/testing.go` (seen but not read in the scan/session research pass) provides shared test fixtures for session-schema tests across packages was not confirmed.
