---
title: Contributing
sidebar:
  order: 13
---

OCR is open source under the Apache-2.0 license. Bug reports, doc fixes,
and code contributions are all welcome. This page is a quick reference;
the canonical version lives in
[`CONTRIBUTING.md`](https://github.com/alibaba/open-code-review/blob/main/CONTRIBUTING.md).

## Ways to contribute

You don't have to write Go to be useful:

- **Bug reports** — open a [GitHub issue](https://github.com/alibaba/open-code-review/issues/new/choose)
  with reproduction steps.
- **Feature requests** — start a thread in
  [Discussions](https://github.com/alibaba/open-code-review/discussions/categories/ideas)
  or open a feature-request issue.
- **Docs** — typo fixes, missing examples, broken links — these PRs
  often merge fastest.
- **Reviewing other PRs** — comments from non-maintainers help reduce
  reviewer load.
- **Code** — bug fixes, performance work, new features.

## Local development setup

### Prerequisites

- [Go ≥ 1.25](https://go.dev/dl/)
- [Git](https://git-scm.com/)
- [Make](https://www.gnu.org/software/make/)

### Getting the source

```bash
# Fork on GitHub, then:
git clone https://github.com/<your-username>/open-code-review.git
cd open-code-review
git remote add upstream https://github.com/alibaba/open-code-review.git

make build       # writes dist/opencodereview
make test        # LC_ALL=C go test -v -race -count=1 ./...
```

> The `upstream` remote is read-only. Push to `origin` (your fork) and
> open PRs from there.

### Running your local build

```bash
./dist/opencodereview review --preview
```

For convenience, drop a symlink at `~/bin/ocr-dev` pointing at
`dist/opencodereview` so you can invoke `ocr-dev` from any repo.

### Make targets

| Target | What it does |
|---|---|
| `make build` | Build for current platform → `dist/opencodereview`. |
| `make build-darwin-amd64` | Cross-compile for macOS Intel. |
| `make build-darwin-arm64` | Cross-compile for macOS Apple Silicon. |
| `make build-linux-amd64` | Cross-compile for Linux x86_64. |
| `make build-linux-arm64` | Cross-compile for Linux ARM64. |
| `make build-windows-amd64` | Cross-compile for Windows x86_64. |
| `make build-windows-arm64` | Cross-compile for Windows ARM64. |
| `make build-all` | All six cross-compiled binaries (linux/darwin/windows × amd64/arm64). |
| `make sha256sum` | Generate `sha256sum.txt` for build artifacts. |
| `make dist` | `clean → build-all → sha256sum`. What CI runs. |
| `make test` | Run tests with race detector. |
| `make clean` | Remove `dist/`. |

## Branching and commit conventions

### Branch prefixes

| Prefix | Purpose |
|---|---|
| `feat/` | New feature |
| `fix/` | Bug fix |
| `docs/` | Documentation only |
| `refactor/` | Refactor with no behaviour change |
| `test/` | Test-only changes |
| `chore/` | Build / CI / tooling |

```bash
git checkout main
git pull upstream main
git checkout -b feat/anthropic-streaming
```

### Commit messages

[Conventional Commits](https://www.conventionalcommits.org/) format:

```
<type>(<scope>): <short summary>

[optional body explaining the why]
```

Examples:

```
feat(agent): add support for custom tool definitions
fix(llm): handle timeout errors in Anthropic API calls
docs(readme): clarify endpoint resolution priority
refactor(viewer): extract task-card rendering into helper
```

The same format is used for **PR titles** so they show up cleanly in the
generated changelog.

## Project layout

```
open-code-review/
├── cmd/opencodereview/        # CLI entry point — flag parsing, dispatch
├── internal/
│   ├── agent/                 # Review agent logic, sub-agent dispatch
│   ├── config/                # Template, rules, allowlist, embedded JSON
│   ├── diff/                  # Git diff parsing, three modes
│   ├── gitcmd/                # Git subprocess runner
│   ├── llm/                   # LLM client (Anthropic & OpenAI), endpoint resolver
│   ├── model/                 # Data structs (LlmComment, Diff, …)
│   ├── pathutil/              # Path utilities
│   ├── release/               # Release-notes generation
│   ├── session/               # JSONL session writer
│   ├── stdout/                # Quiet-able stdout writer
│   ├── suggestdiff/           # Suggestion diff rendering
│   ├── telemetry/             # OpenTelemetry config + helpers
│   ├── tool/                  # Tool registry + provider impls
│   └── viewer/                # Embedded HTTP UI
├── pages/                     # WebUI marketing page (separate React app)
├── plugins/                   # Claude Code slash command
├── extensions/                # Editor extensions (VS Code)
├── examples/                  # CI recipes (GitHub Actions, GitLab CI)
├── skills/                    # Agent SDK skill manifest
├── scripts/                   # NPM postinstall + cross-build scripts
├── npm/                       # Per-platform optional dependency packages
└── bin/                       # NPM wrapper (Node)
```

Most contributions touch `internal/agent/`, `internal/tool/`, or
`internal/llm/`. The CLI surface in `cmd/opencodereview/` is
intentionally thin — flag parsing then dispatch to the agent package.

## Code quality checks

Before opening a PR:

```bash
go fmt ./...
go vet ./...
make test       # race-enabled, runs in CI on every push
make build      # smoke test the binary builds
```

CI runs the same set on every push; nothing surprising.

## Adding new tools

A tool has two parts:

1. **JSON definition** in
   [`internal/config/toolsconfig/tools.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/toolsconfig/tools.json):
   the name, description, and JSON-schema parameters the LLM sees.
2. **Go provider** registered in `internal/tool/definitions.go` with
   the actual implementation.

Both have to be present for a new tool name to work. See [Tools](../tools/)
for the existing six and treat them as templates.

## Adding new rule patterns

Edit `internal/config/rules/system_rules.json` to map a new glob to a
rule doc, and add the corresponding markdown under
`internal/config/rules/rule_docs/`. Rule docs are single-file per
pattern (English). The `language` config only appends a directive to the
system prompt instructing the model to respond in that language; it does
not switch rule-doc files.

## PR process

1. **Open an issue first for big changes.** Aligning on the approach
   beats discovering misalignment in code review.
2. **One logical change per PR.** If you have two unrelated fixes,
   submit two PRs.
3. **Update tests.** Behaviour changes need test coverage — `make test`
   has to pass.
4. **Update docs.** If the change affects flags, config keys, or the
   review pipeline, update both this docs site (in [`docs/`](https://github.com/alibaba/open-code-review))
   and any relevant inline help.
5. **Fill in the PR template.** A maintainer will review, usually
   within a few business days.

## Contributor License Agreement (CLA)

The project requires the Alibaba Open Source CLA. The first time you
open a PR, a bot will post a link — sign electronically (takes a
minute). Subsequent PRs don't require re-signing.

## First contribution?

Look for issues labeled
[`good first issue`](https://github.com/alibaba/open-code-review/labels/good%20first%20issue)
or [`help wanted`](https://github.com/alibaba/open-code-review/labels/help%20wanted).
Most are small, self-contained, and have enough context in the issue
description to get started.

## See Also

- [Architecture](../architecture/) — the mental model you'll need
  before touching `internal/agent/`.
- [Tools](../tools/) — what the existing tools look like.
- Full contributing guide:
  [CONTRIBUTING.md](https://github.com/alibaba/open-code-review/blob/main/CONTRIBUTING.md)
