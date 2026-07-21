<div align="center">
  <a href="https://open-codereview.ai">
    <img src="imgs/logo-core.svg" alt="OpenCodeReview logo" width="180" />
  </a>
  <h1>OpenCodeReview</h1>
</div>

<p align="center">
  <a href="https://trendshift.io/repositories/41087" target="_blank">
    <img src="https://trendshift.io/api/badge/trendshift/repositories/41087/weekly?language=Go" alt="alibaba%2Fopen-code-review | Trendshift" style="width: 320px; height: 70px;" width="320" height="70" />
  </a>
</p>
<p align="center">
  <a href="https://www.npmjs.com/package/@alibaba-group/open-code-review"><img alt="npm" src="https://img.shields.io/npm/v/@alibaba-group/open-code-review?style=flat-square" /></a>
  <a href="https://github.com/alibaba/open-code-review/actions/workflows/release.yml"><img alt="Build status" src="https://img.shields.io/github/actions/workflow/status/alibaba/open-code-review/release.yml?style=flat-square" /></a>
  <a href="https://github.com/alibaba/open-code-review/blob/main/LICENSE"><img alt="License" src="https://img.shields.io/github/license/alibaba/open-code-review?style=flat-square" /></a>
  <a href="https://deepwiki.com/alibaba/open-code-review"><img alt="Ask DeepWiki" src="https://deepwiki.com/badge.svg" /></a>
  <a href="https://www.bestpractices.dev/projects/13328"><img alt="OpenSSF Best Practices" src="https://img.shields.io/badge/OpenSSF-Silver-4C566A?style=flat-square" /></a>
</p>
<p align="center">
  <a href="#supported-platforms"><img alt="Windows" src="https://img.shields.io/badge/Windows-supported-blue.svg" /></a>
  <a href="#supported-platforms"><img alt="macOS" src="https://img.shields.io/badge/macOS-supported-blue.svg" /></a>
  <a href="#supported-platforms"><img alt="Linux" src="https://img.shields.io/badge/Linux-supported-blue.svg" /></a>
  <a href="#supported-agents"><img alt="Claude Code" src="https://img.shields.io/badge/Claude_Code-supported-blueviolet.svg" /></a>
  <a href="#supported-agents"><img alt="Codex" src="https://img.shields.io/badge/Codex-supported-blueviolet.svg" /></a>
  <a href="#supported-agents"><img alt="Cursor" src="https://img.shields.io/badge/Cursor-supported-blueviolet.svg" /></a>
</p>
<p align="center">
  English | <a href="README.zh-CN.md">简体中文</a> | <a href="README.ja-JP.md">日本語</a> | <a href="README.ko-KR.md">한국어</a> | <a href="README.ru-RU.md">Русский</a>
</p>

---

## What is Open Code Review?

Open Code Review is an AI-powered code review CLI tool. It originated as Alibaba Group's internal official AI code review assistant — over the past two years, it has served tens of thousands of developers and identified millions of code defects. After thorough validation at massive scale, we incubated it into an open source project for the community. Simply configure a model endpoint to get started.

It reads Git diffs, sends changed files to a configurable LLM via an agent with tool-use capabilities, and generates structured review comments with line-level precision. The agent can read full file contents, search the codebase, inspect other changed files for context, and produce deep reviews — not just surface-level diff feedback. Beyond diff review, `ocr scan` reviews entire files for auditing unfamiliar codebases or directories that have no meaningful diff.

Visit the [official website](https://open-codereview.ai) for more details.

![Highlights](imgs/highlights-en.png)

## Benchmark

> Compared to general-purpose agents (Claude Code), Open Code Review achieves significantly higher **Precision** and **F1** with the same underlying model, while consuming only **~1/9 of the tokens** and completing reviews faster. Note that its Recall is lower than general-purpose agents — a deliberate trade-off favoring precision over noise.

A real-world code review benchmark built from **50** popular open-source repositories, **200** real Pull Requests, and **10** programming languages — cross-validated by 80+ senior engineers (**1,505** annotated ground-truth issues).

| Metric | What it measures | Why it matters |
|--------|-----------------|----------------|
| **F1** | Harmonic mean of precision and recall | Best single number for overall review quality |
| **Precision** | Proportion of reported issues that are real defects | Higher = fewer false alarms to triage |
| **Recall** | Proportion of real defects that are found | Higher = fewer issues slip through review |
| **Avg Time** | Wall-clock time per review | Matters for CI pipeline latency |
| **Avg Token** | Total tokens consumed per review | Directly impacts API cost |

![Benchmark](imgs/benchmark-en.png)

## Why Open Code Review?

### The Problem with General-Purpose Agents

If you've used general-purpose agents like Claude Code with Skills for code review, you've likely encountered these pain points:

- **Incomplete coverage** — On larger changesets, agents tend to "cut corners," selectively reviewing only some files and missing others.
- **Position drift** — Reported issues frequently don't match the actual code location, with line numbers or file references drifting off target.
- **Unstable quality** — Natural-language-driven Skills are hard to debug, and review quality fluctuates significantly with minor prompt variations.

The root cause: a purely language-driven architecture lacks hard constraints on the review process.

### Core Design: Deterministic Engineering × Agent Hybrid

Open Code Review's core philosophy is to combine deterministic engineering with an agent, each handling what it does best.

**Deterministic Engineering — Hard Constraints**

For review steps that *must not go wrong*, engineering logic — not the language model — guarantees correctness:

- **Precise file selection** — Determines exactly which files need review and which should be filtered, ensuring no important change is missed.
- **Smart file bundling** — Groups related files into a single review unit (e.g., `message_en.properties` and `message_zh.properties` are bundled together). Each bundle runs as a sub-agent with isolated context — a divide-and-conquer strategy that stays stable on very large changesets and naturally supports concurrent review.
- **Fine-grained rule matching** — Matches review rules to each file's characteristics, keeping the model's attention sharply focused and eliminating information noise at the source. Compared to purely language-driven rule guidance, template-engine-based rule matching is more stable and predictable.
- **External positioning and reflection modules** — Independent comment-positioning and comment-reflection modules systematically improve both the location accuracy and content accuracy of AI feedback.

**Agent — Dynamic Decision-Making**

The agent's strengths are concentrated where they matter most — dynamic decisions and dynamic context retrieval:

- **Scenario-tuned prompts** — Prompt templates deeply optimized for code review, improving effectiveness while reducing token consumption.
- **Scenario-tuned toolset** — Distilled from deep analysis of tool-call traces in large-scale production data — including call frequency distributions, per-tool repetition rates, and the impact of new tools on the overall call chain — resulting in a purpose-built toolset that is more stable and predictable for code review than a generic agent toolkit.

## How to Use

### Prerequisites

- **Git >= 2.41** — Open Code Review relies on Git for diff generation, code search, and repository operations.

### CLI

#### Install

**Via NPM (Recommended)**

```bash
npm install -g @alibaba-group/open-code-review
```

After installation, the `ocr` command is available globally.

**Update**

If you installed via NPM, update manually to the latest version:

```bash
npm install -g @alibaba-group/open-code-review@latest
```

NPM installations also check for newer versions in the background by default and upgrade automatically. To disable auto-updates, set `OCR_NO_UPDATE=1`.

If you installed with the install script or a manually downloaded binary, rerun the same install/download command to replace the local binary with the latest release. Use `OCR_VERSION` when you need to pin a specific release tag.

**From GitHub Release**

Install the latest binary for your OS/architecture with one command (macOS / Linux):

```bash
curl -fsSL https://raw.githubusercontent.com/alibaba/open-code-review/main/install.sh | sh
```

The script picks the right release binary, verifies its SHA-256 checksum, and installs it as `ocr` in `/usr/local/bin`. Override the target with `OCR_INSTALL_DIR` or pin a release with `OCR_VERSION`:

```bash
OCR_INSTALL_DIR="$HOME/.local/bin" OCR_VERSION=v1.3.13 \
  sh -c "$(curl -fsSL https://raw.githubusercontent.com/alibaba/open-code-review/main/install.sh)"
```

On Windows (PowerShell 5.1+):

```powershell
irm https://raw.githubusercontent.com/alibaba/open-code-review/main/install.ps1 | iex
```

The script picks the right Windows release binary, verifies its SHA-256 checksum, and installs it as `ocr.exe` in `%LOCALAPPDATA%\Programs\ocr`. Override the target with `OCR_INSTALL_DIR` or pin a release with `OCR_VERSION`:

```powershell
$env:OCR_INSTALL_DIR = "$env:USERPROFILE\bin"
$env:OCR_VERSION = "v1.3.13"
irm https://raw.githubusercontent.com/alibaba/open-code-review/main/install.ps1 | iex
```

Piping a remote script into a shell executes code from the internet. Prefer downloading and inspecting first:

```bash
curl -fsSL https://raw.githubusercontent.com/alibaba/open-code-review/main/install.sh -o install.sh
less install.sh && sh install.sh
```

```powershell
irm https://raw.githubusercontent.com/alibaba/open-code-review/main/install.ps1 -OutFile install.ps1
notepad install.ps1   # review, then: .\install.ps1
```

<details>
<summary>Manual download (all platforms, including Windows)</summary>

Download the binary for your platform from [GitHub Releases](https://github.com/alibaba/open-code-review/releases):

```bash
# macOS (Apple Silicon)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# macOS (Intel)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux (x86_64)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux (ARM64)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Windows (x86_64) — move ocr.exe to a directory in your PATH
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-amd64.exe

# Windows (ARM64) — move ocr.exe to a directory in your PATH
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-arm64.exe
```

</details>

**From Source**

```bash
git clone https://github.com/alibaba/open-code-review.git
cd open-code-review
make build
sudo cp dist/opencodereview /usr/local/bin/ocr
```

#### Quick Start

**1. Configure LLM**

**You must configure an LLM before reviewing code.**

OCR manages LLM configuration through a unified **Provider** system. It ships with many popular built-in providers and also supports adding custom providers to connect to private deployments or other compatible endpoints. Config is stored in `~/.opencodereview/config.json`.

**Option A: Interactive setup (Recommended)**

```bash
ocr config provider          # Select a built-in provider or add a custom one
ocr config model             # Pick a model for the active provider
```

![Provider setup](imgs/providers.jpg)

The interactive UI guides you through provider selection, API key entry, and model configuration, then automatically tests connectivity.

Run `ocr llm providers` to see all built-in providers. Built-in providers come with preset API URLs and protocols — just supply an API key to get started. If the corresponding environment variable is already set (e.g. `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`), the API key is picked up automatically.

**Custom providers** can also be added through the interactive UI — you'll need to provide a name, API URL, protocol type (`anthropic` or `openai`), and API key.

**Option B: CLI setup (for CI/CD and non-interactive environments)**

Use `ocr config set` to write provider configuration directly, suitable for scripts and automation.

Using a built-in provider:

```bash
ocr config set provider anthropic
ocr config set providers.anthropic.api_key your-api-key-here
ocr config set providers.anthropic.model claude-sonnet-4-6
```

Using a custom provider (private gateway or other compatible endpoint):

```bash
ocr config set provider my-gateway
ocr config set custom_providers.my-gateway.url https://my-llm-gateway.internal/v1
ocr config set custom_providers.my-gateway.protocol openai
ocr config set custom_providers.my-gateway.api_key your-api-key-here
ocr config set custom_providers.my-gateway.model gpt-4o
```

> `url` and `protocol` are required for custom providers. Supported protocols: `anthropic`, `openai`, `openai-responses`.

Optional settings:

| Key | Description |
|-----|-------------|
| `providers.<name>.auth_header` | Auth header: `x-api-key` or `authorization` (default: `authorization`) |
| `providers.<name>.extra_body` | Custom JSON fields merged into the request body |
| `providers.<name>.extra_headers` | Comma-separated `key=value` pairs of custom HTTP headers added to every request |
| `providers.<name>.models` | Model list for interactive selection |

**`extra_headers` (optional):** Adds custom HTTP headers to every LLM API request. Useful for proxies, gateways, or enterprise endpoints that require additional headers (e.g. organization IDs, tracing IDs). Format is comma-separated `key=value` pairs. Double-quote values that contain commas:

```bash
ocr config set llm.extra_headers "X-Org-ID=org-123,X-Forwarded-For=\"1.2.3.4,5.6.7.8\""
```

You can also set extra headers per-provider:

```bash
ocr config set providers.anthropic.extra_headers "X-Org-ID=org-123"
```

**Environment variables (highest priority)**

Environment variables override config file settings, useful in CI/CD where writing config files is inconvenient:

```bash
export OCR_LLM_URL=https://api.anthropic.com/v1/messages
export OCR_LLM_TOKEN=your-api-key-here
export OCR_LLM_MODEL=claude-opus-4-6
export OCR_USE_ANTHROPIC=true
```

To use the OpenAI Responses API (GPT-5.x / o-series), set `OCR_LLM_PROTOCOL` instead of `OCR_USE_ANTHROPIC`:

```bash
export OCR_LLM_URL=https://api.openai.com/v1
export OCR_LLM_TOKEN=your-openai-key
export OCR_LLM_MODEL=gpt-5.4
export OCR_LLM_PROTOCOL=openai-responses
```

`OCR_LLM_PROTOCOL` accepts `anthropic`, `openai`, `openai-responses`, and takes priority over `OCR_USE_ANTHROPIC` when both are set.

Also compatible with Claude Code environment variables (`ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_MODEL`) and parses `~/.zshrc` / `~/.bashrc` for those exports.

> **Note for CC-Switch Users**: If you are using [CC-Switch](https://github.com/farion1231/cc-switch) with [routing service](https://www.ccswitch.io/en/docs?section=proxy&item=service) enabled, you can point the provider's `url` to the CC-Switch proxy address without additional configuration:
> - For **Claude** provider: set `providers.anthropic.url` to `http://127.0.0.1:15721`
> - For **Codex** provider: set the corresponding provider's `url` to `http://127.0.0.1:15721/v1`
> - `api_key` can be any value; `extra_body` settings still apply

**2. Test Connectivity**

```bash
ocr llm test
```

**3. Review**

```bash
cd your-project

# Workspace mode — review all staged, unstaged, and untracked changes
ocr review

# Branch range — compare two refs
ocr review --from main --to feature-branch

# Single commit
ocr review --commit abc123

# Resume an interrupted range or commit review
ocr session list
ocr review --from main --to feature-branch --resume <session-id>

# Full-file scan — review whole files instead of a diff (no git history needed)
ocr scan                          # scan the entire repository
ocr scan --path internal/agent    # scan a directory or specific files

# Delegation mode — let your AI coding agent perform the review itself
# OCR handles file selection and rule resolution; no LLM configuration needed
ocr delegate preview
ocr delegate rule src/main.go src/handler.go
```

### Integrate with Coding Agents

OCR can be seamlessly integrated into AI coding agents as a slash command, enabling code review directly within your agent workflow.

#### Option 1: Install as a Skill

Use `npx` to install the OCR skill into your project:

```bash
npx skills add alibaba/open-code-review --skill open-code-review
```

This installs the `open-code-review` skill from the [skills registry](skills/open-code-review/SKILL.md), which teaches your coding agent how to invoke `ocr` for code review, classify issues by priority, and optionally apply fixes.

**Delegation mode** — if you want your coding agent to perform the review itself (using OCR only for file selection and rule resolution, no LLM configuration needed on the OCR side):

```bash
npx skills add alibaba/open-code-review --skill open-code-review-delegate
```

See [skills/open-code-review-delegate/SKILL.md](skills/open-code-review-delegate/SKILL.md) for details.

#### Option 2: Install as a Claude Code Plugin

For [Claude Code](https://docs.anthropic.com/en/docs/claude-code), install the command plugin through the following command in Claude Code:

```bash
/plugin marketplace add alibaba/open-code-review
/plugin install open-code-review@open-code-review
```

This registers the `/open-code-review:review` slash command, which runs OCR and automatically filters and fixes issues. It also provides `/open-code-review:delegate-review` for delegation mode (the agent reviews using its own capabilities while OCR handles file selection and rules).

#### Option 3: Install as a Codex Plugin

For local Codex, install the Open Code Review plugin from this repository:

```bash
codex plugin marketplace add alibaba/open-code-review
codex
/plugins
```

For a local checkout or fork:

```bash
codex plugin marketplace add .
codex
/plugins
```

Install and enable `Open Code Review`, then start a new Codex thread and invoke it explicitly:

```text
@Open Code Review review my current changes
@Open Code Review review this branch against main
@Open Code Review review and fix high-confidence issues
```

This registers a Codex skill that runs the local OCR CLI:

```bash
ocr review --audience agent
```

This integration does not change OCR's internal LLM backend and does not require configuring an OpenAI Responses API endpoint for Codex. OCR itself still requires the `ocr` CLI to be installed and configured as described in the CLI setup section.

Korean guide: [`plugins/open-code-review/CODEX.ko-KR.md`](plugins/open-code-review/CODEX.ko-KR.md)

#### Option 4: Install as a Cursor Plugin

For [Cursor](https://www.cursor.com/), install the Open Code Review plugin from this repository:

```
cursor-plugin marketplace add alibaba/open-code-review
```

Or add the marketplace manually. In Cursor, open `/plugins`, search for `Open Code Review`, and install it.

For a local checkout or fork:

```
cursor-plugin marketplace add .
```

After installation, invoke it in Cursor:

```text
@Open Code Review review my current changes
@Open Code Review review this branch against main
@Open Code Review review and fix high-confidence issues
```

This registers a Cursor skill that runs the local OCR CLI:

```bash
ocr review --audience agent
```

This integration does not change OCR's internal LLM backend. OCR itself still requires the `ocr` CLI to be installed and configured as described in the CLI setup section.

#### Option 5: Copy the Command File Directly

For a quick setup without any package manager, simply copy the command file to use the `/open-code-review` slash command in Claude Code.

**Project-level** (shared with team via git):

```bash
mkdir -p .claude/commands
curl -o .claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/review.md
```

**User-level** (personal global use across all projects):

```bash
mkdir -p ~/.claude/commands
curl -o ~/.claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/review.md
```

For delegation mode (no LLM configuration needed on OCR side):

```bash
# Project-level
mkdir -p .claude/commands
curl -o .claude/commands/open-code-review-delegate.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/delegate-review.md

# User-level
mkdir -p ~/.claude/commands
curl -o ~/.claude/commands/open-code-review-delegate.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/delegate-review.md
```

> **Prerequisite**: All integration methods require the `ocr` CLI to be installed. Standard mode additionally requires an LLM configured — see [Install](#install) and [Configure LLM](#1-configure-llm) above. Delegation mode does **not** require LLM configuration on the OCR side.

### CI/CD Integration

OCR can be integrated into CI/CD pipelines to automate code review on Merge Requests / Pull Requests.

The core command for CI integration:

```bash
ocr review \
  --from "origin/main" \
  --to "<commit_sha>" \
  --format json
```

The `--from` flag accepts a branch ref (e.g., `origin/main`) or commit SHA as the base, while `--to` accepts a commit SHA or branch ref as the head. In CI environments, using commit SHA for `--to` is recommended to correctly handle fork PRs/MRs where the source branch doesn't exist on the origin remote.

The `--format json` flag outputs machine-readable results suitable for parsing in CI scripts.

Each finding carries two structured fields so CI integrations can sort, group, filter, or gate builds without re-parsing comment text:

| Field | Allowed values | Notes |
|-------|----------------|-------|
| `category` | `bug`, `security`, `performance`, `maintainability`, `test`, `style`, `documentation`, `other` | The category the issue belongs to. |
| `severity` | `critical`, `high`, `medium`, `low` | The importance of the issue. |

In JSON output the two fields appear as siblings alongside `content`, `start_line`, etc. In the terminal, they render as an inline `[category · severity]` badge before the comment, colored by severity.

See the [`examples/`](./examples/) directory for integration examples:

- [`github_actions/`](./examples/github_actions/) — GitHub Actions integration example
- [`gitlab_ci/`](./examples/gitlab_ci/) — GitLab CI integration example
- [`gitflic_ci/`](./examples/gitflic_ci/) — GitFlic CI integration example
- [`gerrit_ci/`](./examples/gerrit_ci/) — Gerrit (Jenkins / Gerrit Trigger) integration example

#### GitHub Action

For GitHub, this repository also ships a ready-to-use composite Action at the repo root ([`action.yml`](./action.yml)). Instead of scripting `ocr review` yourself, reference it directly and it handles the full pipeline — checkout, OCR install, running the review, posting inline and summary comments, uploading artifacts, and retry/idempotency:

```yaml
- uses: alibaba/open-code-review@main
  with:
    llm_url: ${{ secrets.OCR_LLM_URL }}
    llm_auth_token: ${{ secrets.OCR_LLM_AUTH_TOKEN }}
    llm_model: ${{ vars.OCR_LLM_MODEL }}
    llm_use_anthropic: ${{ vars.OCR_LLM_USE_ANTHROPIC }}
```

Pin to a version tag or commit SHA for reproducibility. See the [`examples/github_actions/`](./examples/github_actions/) directory for a complete workflow demo and the full list of inputs, outputs, and comment-posting modes (sticky summary, incremental non-destructive posting).

## Documentation

Full documentation lives at **[open-codereview.ai/docs](https://open-codereview.ai/docs)**:

- [Quickstart](https://open-codereview.ai/docs/quickstart) — install and run your first review
- [Installation](https://open-codereview.ai/docs/installation) — all platforms and package managers
- [CLI Reference](https://open-codereview.ai/docs/cli-reference) — every command and flag
- [Review Rules](https://open-codereview.ai/docs/review-rules) — rule priority chain, file format, and path filtering
- [Configuration](https://open-codereview.ai/docs/configuration) — config keys and environment variables
- [MCP Server](https://open-codereview.ai/docs/mcp) — extend the review agent with external tools
- [Coding Agent Integrations](https://open-codereview.ai/docs/claude-code) — Claude Code, Agent Skill, and delegation mode
- [CI/CD Integration](https://open-codereview.ai/docs/cicd) — run reviews in your pipeline
- [Architecture](https://open-codereview.ai/docs/architecture) · [Tools](https://open-codereview.ai/docs/tools) · [Session Viewer](https://open-codereview.ai/docs/viewer) · [Telemetry](https://open-codereview.ai/docs/telemetry) · [FAQ](https://open-codereview.ai/docs/faq)

## Commands

OCR provides `review`, `scan`, `delegate`, `config`, `llm`, `session`, and `viewer` commands. For the complete command list and every flag — including resumable reviews and the full `ocr scan` / `ocr delegate` options — see the **[CLI Reference](https://open-codereview.ai/docs/cli-reference)**.

## Examples

```bash
# Interactive provider and model setup
ocr config provider
ocr config model
ocr llm providers

# Delete a custom provider
ocr config unset custom_providers.my-gateway

# Preview which files will be reviewed (no LLM calls)
ocr review --preview
ocr review -c abc123 -p

# Review workspace changes with default settings
ocr review

# Review branch diff with higher concurrency
ocr review --from main --to my-feature --concurrency 4

# Review a specific commit with verbose JSON output
ocr review --commit abc123 --format json --audience agent

# Resume an interrupted range or commit review
ocr session list
ocr session show <session-id>
ocr review --from main --to my-feature --resume <session-id>
ocr review --commit abc123 --resume <session-id>

# Select or override model for this review
ocr review --model claude-opus-4-6
ocr review --commit abc123 --model claude-sonnet-4-6

# Provide requirement context for more targeted review
ocr review --background "Adding rate limiting to the login API"

# Provide requirement context from a Markdown file
ocr review --background-file ./docs/my_business_context.md

# Combine inline context with a local context file (both are used)
ocr review --background "Focus on auth" --background-file ./docs/my_business_context.md

# Use custom review rules
ocr review --rule /path/to/my-rules.json

# Preview which rule applies to a file
ocr rules check src/main/java/com/example/Foo.java
ocr rules check --rule custom.json src/main/resources/mapper/UserMapper.xml

# Full-file scan: preview the file list first (no LLM calls)
ocr scan --preview

# Scan the whole repo, cap spend at ~500k tokens
ocr scan --max-tokens-budget 500000

# Scan a subdirectory, skipping generated/test files
ocr scan --path internal --exclude '**/*_test.go,**/generated/**'

# Scan a non-git directory with JSON output (includes project_summary)
ocr scan --repo /path/to/plain/dir --format json

# Fastest scan: skip planning, dedup, and the project summary
ocr scan --no-plan --no-dedup --no-summary

# Delegation mode — let your AI agent drive the review (no LLM config needed)
ocr delegate preview
ocr delegate preview --from main --to feature-branch
ocr delegate preview --commit abc123
ocr delegate rule internal/handler.go internal/service.go cmd/main.go

# View review session history in browser
ocr viewer
ocr viewer --addr :3000
```

### Viewer security

The viewer serves session JSONL contents (LLM request messages and responses) over HTTP. It enforces a Host-header allowlist on every request: loopback names (`localhost`, `127.0.0.0/8`, `::1`) and the concrete bind host are always allowed. Wildcard binds (`--addr :3000`, `--addr 0.0.0.0:3000`) and other non-loopback Hostnames must be added via the `OCR_VIEWER_ALLOWED_HOSTS` environment variable (comma-separated):

```bash
OCR_VIEWER_ALLOWED_HOSTS=review.internal,ocr.lan ocr viewer --addr :3000
```

This blocks DNS-rebinding attacks against the local viewer.

## Review Rules

OCR resolves review rules through a four-layer priority chain (`--rule` flag > project config > global config > built-in defaults), and supports inline or file-based rules, `**` glob matching, and `include` / `exclude` path filtering. For the full rule file format and filtering semantics, see **[Review Rules](https://open-codereview.ai/docs/review-rules)**.

## Configuration Reference

Configuration lives in `~/.opencodereview/config.json` and can be overridden by environment variables. It covers providers, models, MCP servers, language, and telemetry. For the complete key reference, environment variables, and MCP server setup, see **[Configuration](https://open-codereview.ai/docs/configuration)** and **[MCP Server](https://open-codereview.ai/docs/mcp)**.

## Telemetry

OpenTelemetry integration for observability (spans, metrics). Disabled by default.

```bash
ocr config set telemetry.enabled true
ocr config set telemetry.exporter otlp
ocr config set telemetry.otlp_endpoint localhost:4317
```

Set `telemetry.content_logging` to include LLM prompts and responses in exported data.

**Protocol selection:** Set the environment variable `OTEL_EXPORTER_OTLP_PROTOCOL` to choose the export protocol:

| Value | Transport | Notes |
|---|---|---|
| `grpc` (default) | gRPC | Default port 4317 |
| `http/protobuf` | HTTP | Default port 4318 |

**Endpoint format:** `telemetry.otlp_endpoint` expects a base URL in `host:port` or `http://host:port` format, without a path component. The SDK appends the signal path (e.g. `/v1/traces`) automatically per the [OTLP specification](https://opentelemetry.io/docs/specs/otlp/#otlphttp-request).


## Contributing

This project exists thanks to all the people who contribute. See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, coding guidelines, and how to submit pull requests.

<a href="https://github.com/alibaba/open-code-review/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=alibaba/open-code-review" />
</a>

## License

[Apache-2.0](LICENSE) — Copyright 2026 Alibaba
