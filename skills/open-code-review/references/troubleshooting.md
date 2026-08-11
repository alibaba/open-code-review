# Troubleshooting & Performance Tuning

## Performance Tuning

| Symptom | Action |
|---------|--------|
| Rate limit errors | Lower `--concurrency` to 2-4 |
| Frequent 429 / 5xx errors | 429 and all 5xx are already retried by the SDK by default; if still failing, lower `--concurrency` or add custom 4xx retry codes (`ocr config set llm.retry_codes 403,400`) |
| Excessive token cost | Set `--max-tokens-budget 500000` (or lower) |
| Single large file truncated | Set `--max-tokens 200000` or `ocr config set max_tokens 200000` |
| Large file timeouts | Increase `--timeout 20` |
| Excessive agent tool turns | Set `--max-tools 15` |
| Windows git overhead | Lower `--max-git-procs 8` |
| Slow LLM response | Set `OCR_LLM_TIMEOUT=120` (seconds) |

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `ocr: command not found` | Not installed | Run `pnpm add -g @alibaba-group/open-code-review` |
| `ocr llm test` fails | LLM not configured | Run `ocr config provider` or set environment variables |
| Exit code ≠ 0 | Every selected item failed or run-level failure | Inspect the failure JSON on stderr; partial failures (`status: "partial"`) exit 0 — check `warnings` array |
| `status: "partial"` | Partial file failures | Check warnings; run `--resume <id>` to retry failed files |
| Token overflow | File diff too large | Use `--exclude`, set `--max-tokens` or `--max-tokens-budget` |
| Rate limited | Concurrency too high | Lower `--concurrency` (429 is already retried by the SDK; `retry_codes` supports extra 4xx codes only) |
| Wrong comment language | Default English | Run `ocr config set language 中文` |
| `--resume` fails | Review workspace mode | Review resume requires `--from/--to` or `--commit`; or use `scan --resume` |
| `--preview` + `--resume` error | Mutually exclusive | Use one or the other |

## Session Management

Large reviews or full-file scans can be resumed after interruption (`scan` mode and `review` range/commit modes support resume; `review` workspace mode does not).

```bash
# List recent sessions
ocr session list [--limit 10] [--json]

# View session details
ocr session show <session-id> [--json]

# Extract and filter saved comments
ocr session comments <session-id> [--json] [--severity critical,high] [--category security,bug]

# Resume interrupted review or scan
ocr review --audience agent --format json --resume <session-id>
ocr scan --audience agent --format json --resume <session-id>
```

Resume reuses already completed file reviews and re-runs only failed or pending files.

All `session` subcommands support `--repo <path>` to select the repository directory (default: current directory).

## Session Viewer

Launch Web UI to browse and replay review session history (supports comment tag filters & repository filters):

```bash
ocr viewer --addr localhost:5483
```
