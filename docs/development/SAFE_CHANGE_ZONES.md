Parent document: `/CLAUDE.md`
Related documents:
- `docs/architecture/CHANGE_BLAST_RADIUS.md`
- `docs/development/TESTING_STRATEGY.md`
- `AGENTS.md`

Read this when:
- You're about to make a change and want to know how much scrutiny it needs before you start.

Purpose:
- Concrete safe-vs-risky classification for common change types, and the mandatory checks for the risky ones.

Scope:
- Included: change-type classification with the specific check required.
- Excluded: the impact analysis itself (see `CHANGE_BLAST_RADIUS.md` — this doc is the actionable checklist derived from it).

---

# Safe Change Zones

## Safe — additive, low blast radius

- New CI integration example under `examples/` (doesn't touch runtime code).
- New rule doc under `internal/config/rules/rule_docs/*.md` for a currently-uncovered language (additive to the resolution chain, doesn't change existing matches).
- New provider preset entry in `internal/llm/providers.go` (pure addition — see `CHANGE_BLAST_RADIUS.md` #7; do not *edit* an existing preset's `BaseURL`/`Protocol`/`EnvVar` under this same "safe" umbrella).
- VS Code extension webview/UI changes that don't touch `CliService.ts`'s subprocess-invocation contract.
- Documentation changes (this tree included).
- New MCP tool exposure via config (`mcp_servers.*`) — no code change at all.

## Requires a checklist, not a rewrite

- Anything in `cmd/opencodereview/` that adds a new flag or subcommand — follow the existing per-file test pattern (see `docs/development/TESTING_STRATEGY.md`); the TUI files in particular have dense per-state test coverage to match.
- New tool implementation in `internal/tool/` — must wire into `internal/tool/definitions.go` *and* `tools.json`'s schema *and* declare `plan_task`/`main_task` gating correctly; a tool declared in JSON without Go-side wiring silently returns `tool.NotAvailableMsg` rather than failing to build.

## High risk — mandatory manual verification beyond `make test`

| Change | Mandatory check |
|---|---|
| `internal/config/template/*.json` or prompt `.md` files | `ocr review --preview` first (no cost), then a real `ocr review`/`ocr scan` against a representative diff, inspected via `ocr viewer` — no automated test catches a quality regression here (`docs/development/TESTING_STRATEGY.md`) |
| `internal/config/toolsconfig/tools.json` | Validate the JSON schema is well-formed *and* every declared tool name resolves in `internal/tool`'s registry; a mismatch breaks tool-calling for every provider simultaneously |
| `internal/llm/resolver.go` (precedence order) | This is a **silent behavior change** class — both old and new resolution can produce a "complete" triple with no error. Manually verify against every one of the 4 strategies (config/OCR-env/Claude-Code-env/rc-file), not just the one you're testing |
| `internal/session/persist.go` / `manifest.go` (record shapes) | Bump the relevant `schema_version` (`ocr.run-manifest/v1`, `ocr.resume-lineage/v1`); verify `ocr session list`, `ocr viewer`, and resume all still parse both old and new records |
| `internal/llmloop/loop.go` | Changes here ship to **both** `review` and `scan` simultaneously — verify both commands, not just the one you're working on |
| `internal/pathutil/path.go` (`WithinBase`) | This is the sole path-traversal guard for every file-touching tool — treat any change here as security-sensitive; verify symlink-resolution behavior explicitly, both before and after resolution |
| `internal/gitcmd/` (git invocation) | Must remain shell-free with hardcoded subcommands — any change introducing string-built commands or shell invocation reopens the command-injection mitigation `ASSURANCE_CASE.md` T1 documents |
| `internal/viewer/hostguard.go` / `securityheaders.go` | The only network-facing access control in the system — verify against both loopback and a wildcard-bind scenario before merging |

## Before any PR (per `AGENTS.md`)

```bash
make check   # license headers, English-only, fmt, vet
make test    # race detector, 90% coverage gate
```

## Known gaps / uncertainties:
- No repo-documented "who reviews security-sensitive changes" policy (e.g., required reviewer for `internal/pathutil`) was found — `GOVERNANCE.md` may cover this at a project-governance level but wasn't cross-checked in this pass.
