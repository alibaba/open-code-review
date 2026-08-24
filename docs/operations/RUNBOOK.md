Parent document: `/CLAUDE.md`
Related documents:
- `docs/operations/FAILURE_MODES.md`
- `docs/operations/OBSERVABILITY.md`
- `pages/src/content/docs/en/cli-reference.md` (full flag reference, user-facing)

Read this when:
- You're operating `ocr` day-to-day (personally or as the maintainer of a CI integration) and need the common task recipes.

Purpose:
- Common maintenance/operating tasks, condensed. Not a flag reference — that's `cli-reference.md`.

Scope:
- Included: provider setup/rotation, session housekeeping, restart/resume, connectivity verification.
- Excluded: full flag syntax (`pages/.../cli-reference.md`), incident diagnosis (`FAILURE_MODES.md`).

---

# Runbook

## First-time provider setup

```bash
ocr config provider   # interactive: pick provider, enter key, choose model, auto-runs `ocr llm test`
ocr config model       # switch models later
```

Non-interactive (CI): `ocr config set provider ... ; ocr config set model ... ; ocr config set providers.<name>.api_key ...` — see `pages/src/content/docs/en/configuration.md`.

## Verify connectivity without spending review tokens

```bash
ocr llm test
```

## Rotate an API key

Update `providers.<name>.api_key` (or switch to `api_key_cmd` to source from a secret manager instead of storing the key in `config.json`). No restart needed — resolution happens fresh on every invocation.

## Preview what a review will do before spending tokens

```bash
ocr review --preview        # shows the 5-gate filter result, no LLM call
ocr rules check <file-path> # shows which rule layer/pattern wins for a file
```

## Resume an interrupted run

```bash
ocr session list
ocr review --from main --to feature-branch --resume <session-id>
```

Review's resume is identity-gated (repo/diff-range/rule-config/provider-model must match the parent run) — if any of those changed, resume is rejected outright rather than silently re-reviewing everything. Scan's resume reuses per-file by content fingerprint — any file with different content is transparently re-reviewed even inside a `--resume` run. See `docs/architecture/DATA_CONTRACTS.md` § Run manifest.

## Inspect a past session

```bash
ocr session list
ocr session show <id>
ocr session comments <id>
ocr viewer                  # browser UI, localhost:5483 by default
```

## Free disk space

Delete session files directly under `~/.opencodereview/sessions/<repo>/` — there's no database, so deleting a `.jsonl` file is a complete, safe removal. The viewer/`ocr session list` regenerate their index from what's on disk on the next read.

## Turn on telemetry for a single run

```bash
OCR_ENABLE_TELEMETRY=1 OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317 ocr review --from main --to HEAD
```

Persistent: `ocr config set telemetry.enabled true` (+ `telemetry.exporter`/`telemetry.otlp_endpoint`). Full recipes: `pages/src/content/docs/en/telemetry.md`.

## Update `ocr`

```bash
npm update -g @alibaba-group/open-code-review
```

or re-run `install.sh`/`install.ps1` (pulls the latest GitHub Release binary + verifies its checksum).

## Restrict a review to specific rules for one PR

```bash
ocr review --rule ./.review-rules-only-for-this-pr.json
```

Bypasses both project and global rule layers for that run only.

## Known gaps / uncertainties:
- No documented procedure for migrating/porting session data between machines — assume `~/.opencodereview/sessions/` is directly copyable (flat JSONL files, self-contained) but this was not explicitly verified.
