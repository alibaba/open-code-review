Parent document: `/CLAUDE.md`
Related documents:
- `docs/operations/OPERATIONAL_FAILURE_GRAPH.md`
- `docs/ai/AI_PIPELINE_MAP.md`
- `docs/architecture/RUNTIME_DEPENDENCY_TREE.md`

Read this when:
- Something failed and you need symptom → probable cause → containment → recovery, fast.

Purpose:
- The failure-scenario reference for diagnosing a bad run.

Scope:
- Included: failures observed/inferable directly from source (resolver, loop, session, viewer, MCP).
- Excluded: propagation between failures (see `OPERATIONAL_FAILURE_GRAPH.md`).

---

# Failure Modes

| Symptom | Probable cause | Containment | Recovery |
|---|---|---|---|
| Process exits non-zero, no LLM call made | Endpoint resolution failed — no complete triple across config/env/rc | N/A — fails before any cost is incurred | Run `ocr llm test`; check the exact 4-strategy precedence in `docs/ai/LLM_ARCHITECTURE.md` |
| One file silently skipped, warning in output | Token-threshold pre-check: prompt would exceed 80% of `MAX_TOKENS` | Only that file is affected; run continues | Raise `max_tokens`/`--max-tokens`, or split the diff |
| File has zero comments | Ambiguous by design — check whether `task_done` was called cleanly (intentional clean review) vs. the lane ending in an error card (real failure) | — | Use `ocr viewer` to inspect the file's `main_task` lane, per `viewer.md` |
| A `code_comment` shows no line number (`start_line=0`) | Sliding-window match failed **and** `RE_LOCATION_TASK` also failed to re-anchor | Comment is still delivered, not dropped | Manual location; consider whether the diff/rule is unusually hard to anchor against |
| Review completes but includes comments that look clearly wrong | `REVIEW_FILTER_TASK` itself errored — errors here are logged and ignored, so unfiltered comments ship | Non-fatal by design | Check logs for `review_filter.execute`/`review_filter.completed` span/event; this is a quality regression, not a crash |
| `--resume` re-reviews everything instead of skipping completed files (review) | Resume rejected: `RepositorySHA256`/`SourceArtifactSHA256`/`RuleConfigSHA256` mismatch, or provider/model changed without an explicit override flag | Whole resume rejected atomically — no partial/inconsistent reuse | Confirm repo state, rule config, and provider/model match the original run |
| `--resume` re-reviews a specific file even though nothing changed (scan) | Content fingerprint (`sha256("full_scan\0"+path+"\0"+content)`) mismatch — any byte change forces re-review | Per-file only, not a whole-run failure | Expected behavior, not a bug — scan resume has no whole-run identity gate |
| Session transcript has gaps but resume still works correctly | Crash before the periodic flush of buffered `llm_request`/`llm_response`/`tool_call` records; `review_item_*` checkpoints force-flush independently | Checkpoint integrity preserved; only human-debugging detail lost | Re-run for a clean transcript if the detail is needed |
| `ocr session list` shows a session as `Aborted: true` | `session_end` record missing — process was killed/crashed mid-run | — | Informational; `--resume` still works off whatever checkpoints were flushed |
| MCP server's tools don't appear | `setup` command failed (non-zero exit, 5-min timeout) or connect/`ListTools` didn't complete within the init timeout | That server is skipped; review/scan proceeds without its tools | Check stderr for `[ocr]`-prefixed diagnostics per `mcp.md` |
| MCP tool silently missing despite being in `tools` allowlist | Name typo — `tools` allowlist entries not offered by the server are skipped with a warning, not an error | — | Check stderr for "allowed tool ... not found in server's tool list" |
| Viewer returns `403 forbidden host` on every route | Host header didn't match the loopback allowlist or a concrete bind host; wildcard binds (`:3000`, `0.0.0.0`) require `OCR_VIEWER_ALLOWED_HOSTS` | Entire viewer inaccessible from that origin, including static assets | Set `OCR_VIEWER_ALLOWED_HOSTS` explicitly, or bind to loopback |
| Viewer won't start | Port already in use — no port-retry logic exists | Command fails outright | Pick a different `--addr` |
| Telemetry shows nothing | `OCR_ENABLE_TELEMETRY`/`telemetry.enabled` unset (default off); or exporter created lazily so a wrong endpoint fails silently at export time, not startup | Review itself is unaffected — telemetry export is async, non-blocking | Check collector access logs for the expected OTLP path; see `docs/operations/OBSERVABILITY.md` |
| `api_key_cmd` hangs for ~5 extra seconds every run | The command's background daemon (e.g. `gpg-agent`, first-use `op` daemon) holds the stdout pipe open | Cosmetic delay, not a failure | Redirect the daemon's own output (`>/dev/null 2>&1`) |
| `api_key_cmd`/`auth_token_cmd` hard-fails the run | Non-zero exit, empty/multi-line/oversized (>64KiB) output, or >60s — no silent fallback by design | Run aborts before any LLM call | Fix the command; this is intentionally not degradable |

## Known gaps / uncertainties:
- Whether a `panic` inside a single subtask (the `subtask.panic` span name implies this is handled) is recovered per-file or crashes the whole run was not directly confirmed by reading the recover/defer logic.
