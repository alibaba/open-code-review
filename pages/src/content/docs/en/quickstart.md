---
title: QuickStart
sidebar:
  order: 3
---

Install OCR, point it at any LLM that speaks the Anthropic Messages API or
the OpenAI Chat Completions API, and run your first code review.

## Prerequisites

- A working **Git** install — OCR drives Git as a subprocess to read diffs.
- An **API key** for an Anthropic-compatible or OpenAI-compatible LLM.
- One of:
  - **Node.js ≥ 18** (recommended; minimum supported: Node 14 — installs via NPM).
  - Or just `curl` + `chmod` to drop the static binary into `$PATH`.
  - Or **Go ≥ 1.25** if you prefer to build from source.

## Step 1 — Install the CLI

### Option A: NPM (recommended)

```bash
npm install -g @alibaba-group/open-code-review
```

The NPM package installs a small wrapper that downloads the right binary for
your OS / architecture on install (via a postinstall hook). If the binary is
missing at run time, the wrapper errors out rather than downloading. After
install, you have a global `ocr` command:

```bash
ocr --version
```

### Option B: GitHub Release binary

Pick the binary for your platform from the
[releases page](https://github.com/alibaba/open-code-review/releases) and
drop it into your `$PATH`:

```bash
# macOS (Apple Silicon)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# macOS (Intel)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux x86_64
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux ARM64
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Windows (AMD64)
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-amd64.exe

# Windows (ARM64)
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-arm64.exe
```

### Option C: Build from source

```bash
git clone https://github.com/alibaba/open-code-review.git
cd open-code-review
make build
sudo cp dist/opencodereview /usr/local/bin/ocr
```

> See the [Installation](../installation/) page for details on each option,
> including how the NPM wrapper resolves the platform binary.

## Step 2 — Configure an LLM

OCR will refuse to run a review until it can resolve a complete LLM
endpoint (URL + token + model). It searches four sources in priority order:

1. `~/.opencodereview/config.json`
2. OCR-specific environment variables (`OCR_LLM_*`)
3. Claude Code environment variables (`ANTHROPIC_*`)
4. `export ANTHROPIC_*` lines parsed out of your shell rc files
   (`~/.zshrc`, `~/.bashrc`, `~/.bash_profile`, `~/.profile`)

### Quickest path: `ocr config set`

```bash
ocr config set llm.url           https://api.anthropic.com/v1/messages
ocr config set llm.auth_token    sk-ant-xxxxxxxxxx
ocr config set llm.model         claude-opus-4-6
ocr config set llm.use_anthropic true
```

These values are persisted to `~/.opencodereview/config.json`.

### Alternative: environment variables

Highest priority — useful in CI / containers where you don't want a config
file on disk:

```bash
export OCR_LLM_URL=https://api.anthropic.com/v1/messages
export OCR_LLM_TOKEN=sk-ant-xxxxxxxxxx
export OCR_LLM_MODEL=claude-opus-4-6
export OCR_USE_ANTHROPIC=true   # default true; set false for OpenAI protocol
```

### Already using Claude Code?

OCR transparently picks up the same vars Claude Code uses, so no extra setup:

```bash
export ANTHROPIC_BASE_URL=https://api.anthropic.com
export ANTHROPIC_AUTH_TOKEN=sk-ant-xxxxxxxxxx
export ANTHROPIC_MODEL=claude-opus-4-6
```

If `ANTHROPIC_BASE_URL` lacks a versioned path, OCR appends `/v1/messages`
automatically.

### Using an OpenAI-compatible endpoint?

Set `llm.use_anthropic` to `false` (or `OCR_USE_ANTHROPIC=false`):

```bash
ocr config set llm.url           https://api.openai.com/v1/chat/completions
ocr config set llm.auth_token    sk-xxxxxxxxxx
ocr config set llm.model         gpt-4o
ocr config set llm.use_anthropic false
```

> See [Configuration](../configuration/) for the full key reference,
> including `llm.extra_body` for vendor-specific request fields and
> `language` for switching review-comment language.

## Step 3 — Test connectivity

```bash
ocr llm test
```

Expected output (model name varies):

```
Source: OCR config file
URL:    https://api.anthropic.com/v1/messages
Model:  claude-opus-4-6
Hello! …
```

If you instead get an error like `no valid LLM endpoint configured`, recheck
the config keys above. A 401 / 403 means the token is wrong or expired.

## Step 4 — Run your first review

Move into any Git repository and run:

```bash
cd path/to/your-repo

# Workspace mode — reviews staged + unstaged + untracked changes (default)
ocr review

# Branch range — reviews `main..feature-branch`
ocr review --from main --to feature-branch

# Single commit — reviews the diff that commit introduced
ocr review --commit abc123
```

You should see a stream of progress lines, finishing with one or more review
comments per file.

> Workspace mode includes **untracked** files. If you only want to review
> what you've staged, `git add` selectively beforehand.

> The three modes above are the basics. See [CLI Reference](../cli-reference/)
> for the complete list of `ocr review` flags — concurrency tuning, output
> format, audience mode, background context, and more — plus every other
> sub-command (`config`, `rules`, `llm test`, `viewer`).

### Want to see what *would* be reviewed first?

```bash
ocr review --preview         # workspace
ocr review -c abc123 -p      # commit
```

`--preview` runs every filter step but never calls the LLM, so it's free.
It prints the file list with each file's status (`added` / `modified` /
`deleted` / `renamed` / `binary`) and, for excluded files, the reason
(`binary`, `unsupported_ext`, `default_path`, `user_exclude`, `deleted`).

### JSON output for tooling

```bash
ocr review --format json --audience agent > review.json
```

- `--format json` emits a machine-readable array of comments, each with
  `path`, `content`, `start_line`, `end_line`, `existing_code`,
  `suggestion_code` and optional `thinking`.
- `--audience agent` suppresses the human-friendly progress UI so the only
  thing on stdout is the JSON / final summary — exactly what an upstream
  agent or CI script wants.

## Step 5 — Review the results

Each comment includes:

| Field | Meaning |
|---|---|
| `path` | File that the comment is about. |
| `content` | The review comment itself, in the configured `language`. |
| `start_line` / `end_line` | Line range in the **new** version of the file. Both `0` means OCR couldn't precisely position the comment — the issue is real but you'll need to find the exact spot yourself. |
| `existing_code` | The snippet from the diff the comment refers to. Used internally for line resolution; useful when `start_line` is `0`. |
| `suggestion_code` | Optional fix snippet. |
| `thinking` | Optional model reasoning. Only present for some models. |

## Step 6 — Inspect a past session

Every review is persisted to `~/.opencodereview/sessions/...` as a
JSONL transcript. Browse them in a local web UI:

```bash
ocr viewer            # http://localhost:5483
ocr viewer --addr :3000
```

> See [Session Viewer](../viewer/) for the full UI tour.

## See Also

- [CLI Reference](../cli-reference/) — every sub-command, flag, and output mode.
- [Review Rules](../review-rules/) — customize what gets reviewed.
- [Integrations](../integrations/) — embed OCR in Claude Code, an Agent skill, or CI.
- [Telemetry](../telemetry/) — ship traces and metrics over OTLP.
- [FAQ](../faq/) — known errors and remedies.
