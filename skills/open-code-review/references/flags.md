# Flag Reference

## Shared Flags (review + scan)

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--audience` | | `human` | `human` (progress UI) or `agent` (summary only). **Always use `agent`.** |
| `--format` | `-f` | `text` | `text` or `json` |
| `--background` | `-b` | `""` | Business context string |
| `--rule` | | `""` | Path to custom rule.json |
| `--repo` | | cwd | Git repository root directory |
| `--exclude` | | `""` | Comma-separated gitignore-style exclude patterns |
| `--concurrency` | | `8` | Maximum concurrent file worker count |
| `--timeout` | | `10` | Per-file timeout in minutes |
| `--max-tools` | | Template default | Maximum tool-call turns per file (review mode: min 10; scan mode: only takes effect when greater than template default) |
| `--max-git-procs` | | `16` | Maximum concurrent git sub-processes |
| `--max-tokens-budget` | | `0` (unlimited) | Token budget cap; outputs partial results gracefully if exceeded |
| `--provider` | | Configured Provider | Per-run override for LLM Provider (e.g. `openai`, `anthropic`) |
| `--model` | | Configured Model | Per-run override for LLM model |
| `--max-tokens` | | `0` | Per-run override for per-file prompt token limit (0 = template/configured default) |
| `--resume` | | `""` | Resumes an interrupted session ID (`scan` supports all; `review` requires `--from/--to` or `--commit`) |
| `--tools` | | Built-in | Path to JSON tools configuration file |
| `--preview` | `-p` | `false` | Dry-run: lists target files without invoking LLM (supports `--format json`) |

## Review-Only Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--from` | | Source ref in Range mode |
| `--to` | | Target ref in Range mode |
| `--commit` | `-c` | Single commit hash (compared against its parent) |
| `--background-file` | `-B` | Markdown file path as business context (max 1 MB) |

## Scan-Only Flags

| Flag | Description |
|------|-------------|
| `--path` | Comma-separated directories/files |
| `--batch` | `none` \| `by-language` \| `by-directory` |
| `--no-plan` | Skip pre-review planning phase |
| `--no-dedup` | Skip cross-file deduplication |
| `--no-summary` | Skip repository-level summary |
