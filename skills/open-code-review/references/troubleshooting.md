# Troubleshooting & Performance Tuning

## Performance Tuning

| Symptom | Action |
|---------|--------|
| Rate limit errors | Lower `--concurrency` to 2-4 |
| Frequent 429 / 5xx errors | Configure `ocr config set llm.retry_codes 429,500,502,503` |
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
| Exit code ≠ 0, stderr warnings | Partial file failures | Inspect `warnings` array in JSON output |
| `status: "partial"` | Partial file failures | Check warnings; run `--resume <id>` to retry failed files |
| Token overflow | File diff too large | Use `--exclude`, set `--max-tokens` or `--max-tokens-budget` |
| Rate limited | Concurrency too high | Lower `--concurrency` or set `llm.retry_codes` |
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
ocr session comments <session-id> [--format json] [--severity critical,high] [--category security,bug]

# Resume interrupted review or scan
ocr review --audience agent --format json --resume <session-id>
ocr scan --audience agent --format json --resume <session-id>
```

Resume reuses already completed file reviews and re-runs only failed or pending files.

## Session Viewer

Launch Web UI to browse and replay review session history (supports comment tag filters & repository filters):

```bash
ocr viewer --addr localhost:5483
```
