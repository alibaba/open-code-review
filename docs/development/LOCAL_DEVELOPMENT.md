Parent document: `/CLAUDE.md`
Related documents:
- `CONTRIBUTING.md` (primary source — full contributor workflow, CLA, PR process; this doc is the condensed "get running" path)
- `docs/development/TESTING_STRATEGY.md`
- `AGENTS.md`

Read this when:
- You're setting up this repo for the first time to make a change.

Purpose:
- Minimal path from clone to a passing local build/test cycle, plus the pitfalls specific to this repo.

Scope:
- Included: setup order, required tools, common first-run pitfalls.
- Excluded: PR/CLA process (fully covered in `CONTRIBUTING.md`, not repeated here).

---

# Local Development

## Prerequisites

Go 1.25+, Git, Make (`CONTRIBUTING.md`).

## Setup order

```bash
git clone https://github.com/<your-fork>/open-code-review.git
cd open-code-review
git remote add upstream https://github.com/alibaba/open-code-review.git
make build
make test
```

`upstream` is read-only for contributors — pull from it, never push; all work goes to your fork (`origin`) via PR.

## Project structure (see `docs/architecture/REPOSITORY_MAP.md` for the full purpose-map)

```
cmd/opencodereview/   CLI entry point
internal/agent/       review orchestration
internal/scan/        scan orchestration
internal/llmloop/      shared tool-use loop
internal/llm/          provider clients
internal/model/        shared data types
internal/session/      persistence
internal/tool/         built-in tools
internal/telemetry/    OpenTelemetry
internal/viewer/       WebUI session viewer
pages/                 docs site frontend
scripts/                build & install scripts
bin/                    npm wrapper
```

## Line endings — a real gotcha in this repo

LF is enforced via `.gitattributes`; CI fails on CRLF. Configure once:

```bash
git config core.autocrlf input
```

If CI flags a line-ending diff, `git add --renormalize .` and re-commit.

## Testing against a real LLM locally

You need a working provider before `ocr review`/`ocr scan` will do anything: `ocr config provider` interactively, or set `OCR_LLM_URL`/`OCR_LLM_TOKEN`/`OCR_LLM_MODEL` (or reuse existing `ANTHROPIC_*` env vars — OCR picks them up automatically per `configuration.md`). For pure logic changes not touching the LLM path, `retry_fake_llm_test.go`-style fixtures exist in `cmd/opencodereview/` — check for an existing fake-LLM test harness before standing up a real endpoint.

## License headers — required on every new source file

```bash
make license-add    # after creating a new .go/.sh/.js/.mjs/.ts/.tsx file
```

CI (`scripts/verify-license.sh`) rejects PRs with missing headers.

## English-only source

`scripts/verify-english-only.go` flags any non-ASCII letter in scanned files. Two escape hatches: an inline `allow-non-english: <reason>` comment for a handful of lines, or a whole-tree exemption via `allowedPrefixes` in that script (currently only `pages/src/i18n/` and `extensions/vscode/`, the latter temporary). Full rationale: `AGENTS.md`.

## Before every commit (per `AGENTS.md`)

```bash
ocr review --audience agent --background "briefly summarize the background requirements"   # self-review, dogfooding
make check    # license + English-only + fmt + vet
make test     # LC_ALL=C, race detector, 90% coverage gate
```

## Known gaps / uncertainties:
- Whether a fake/mock LLM client fixture is reusable outside `cmd/opencodereview`'s own tests (e.g., for testing `internal/agent` or `internal/scan` in isolation) was not confirmed.
