# open-code-review for Pi

A [Pi](https://pi.dev) extension that runs [open-code-review](https://open-codereview.ai)
(`ocr`) from your Pi session. It registers two LLM-callable tools and
two slash commands:

| Resource | Name | What it does |
| --- | --- | --- |
| Tool | `ocr_review` | Reviews a diff with the LLM configured for `ocr`. |
| Tool | `ocr_delegate` | Selects files and resolves rules without calling an OCR LLM, so the session reviews them itself. |
| Command | `/ocr-review` | Explicit review, or a hand-off of free text to the agent. |
| Command | `/ocr-review-delegate` | Explicit delegation-mode run, or a hand-off of free text to the agent. |

## Requirements

- Pi, with the `ocr` CLI 1.9.0 or newer on `PATH` and Git 2.41 or newer:

  ```bash
  npm install -g @alibaba-group/open-code-review
  ```

- An OCR LLM provider for `ocr_review`:

  ```bash
  ocr config provider
  ocr config model
  ocr llm test
  ```

  Delegation mode (`ocr_delegate`) needs no provider.

## Install

```bash
git clone https://github.com/alibaba/open-code-review
pi install ./open-code-review/plugins/open-code-review/pi
```

Start a new Pi session, or restart Pi, to load the extension.

## Tools

### `ocr_review`

Runs `ocr review --audience agent --format json --output <temp file>` in the session's
working directory and returns the findings.

| Parameter | Type | Meaning |
| --- | --- | --- |
| `scope` | `workspace` (default), `commit`, `range` | Which flag set to use. |
| `commit` | string | Commit or tag to review against its parent. Requires `scope: "commit"`, rejects `from`/`to`. |
| `from` | string | Base ref. Requires `scope: "range"`. |
| `to` | string | Target ref. Requires `scope: "range"`, defaults to `HEAD`. |
| `background` | string | Requirement or business context for the review. |
| `exclude` | string | Comma-separated gitignore-style patterns, merged with the repository rules. |
| `analysis` | boolean | Behavior, impact, and maintainer reports. Commit or range only. |
| `preview` | boolean | List the files a review would cover without calling the LLM. |
| `timeoutMinutes` | number | Process budget. Defaults to 30, or 2 for a preview. |

The tool returns a compact report: status, files reviewed, and each finding as
`path:line [severity/category] text`. Up to 20 findings are listed. Beyond that the
report stops with a count and the path of the full JSON file, which stays on disk so a
follow-up read can fetch any detail.

### `ocr_delegate`

Runs `ocr delegate <action> --format json` and returns the JSON that OCR produced. No
OCR LLM call happens: `ocr` selects what to review, and this session reviews it.

| Parameter | Type | Meaning |
| --- | --- | --- |
| `action` | `preview` or `rule`, required | `preview` lists the reviewable files; `rule` resolves the rules for `paths`. |
| `commit`, `from`, `to` | string | Same selection rules as `ocr_review`, without `scope`: `commit` alone, or `from` with an optional `to`, or neither for the workspace. |
| `background` | string | Requirement or business context for the selection. |
| `exclude` | string | Comma-separated gitignore-style patterns. |
| `paths` | string[] | Files to resolve rules for. Required by `rule`, rejected by `preview`. |
| `timeoutMinutes` | number | Process budget. Defaults to 2. |

## Delegation mode

Delegation is for sessions that review with their own model, or where no OCR provider is
configured: `ocr_delegate` returns what to review and the rules that apply, and the
session reviews the diff itself. [The repository skill](https://github.com/alibaba/open-code-review/tree/main/plugins/open-code-review/skills/open-code-review-delegate)
spells out that workflow, and the commands below route free text to it.

## Commands

`/ocr-review` and `/ocr-review-delegate` read their arguments in two ways.

Explicit flags run the corresponding tool directly, with no model turn:

```text
/ocr-review
/ocr-review --from main --to feature
/ocr-review --commit abc123 --background "rate limiting for the login API"
/ocr-review --preview
/ocr-review-delegate preview --from main
/ocr-review-delegate rule internal/agent/agent.go internal/llm/client.go
```

Anything else is handed to the agent together with the repository's
`open-code-review` skill, which is where free-form requests belong.

Flags accepted by `/ocr-review`: `--from`, `--to`, `--commit`/`-c`, `--background`/`-b`,
`--exclude`, `--preview`/`-p`, `--analysis`, `--timeout`. `/ocr-review-delegate` accepts
the same flags after a `preview` or `rule` action, and treats the remaining arguments as
paths.

## Output handling

`ocr review` writes its JSON to a temp file and reads it back. The report inlines up to 20
findings (see `ocr_review` above); past that the file stays on disk, so `Full JSON:` always
points at a file that exists. Output the host truncated is reported as truncated, never
parsed, and long detail and analysis are cut at 4000 characters instead of flooding the
session.

`ocr delegate` writes JSON to stdout. Output the host truncated, or longer than a single
result can carry whole, is reported as truncated, never parsed.

No findings is a valid answer. A non-zero exit is not: the tool then returns the exit
code and the CLI's stderr, and no findings at all.

## Troubleshooting

**`ocr was not found on PATH`.** The extension never installs anything for you. Install
the CLI yourself with `npm install -g @alibaba-group/open-code-review`.

**`unknown flag: --format`.** `ocr delegate --format json` needs `ocr` 1.9.0 or newer.
Upgrade with `npm install -g @alibaba-group/open-code-review@latest`.

**`ocr review` fails with a provider or model error.** Delegation mode works without a
provider: use `ocr_delegate`, or configure one with `ocr config provider`,
`ocr config model`, and `ocr llm test`.

**A review returned no findings but exited non-zero.** Nothing is inferred from a failed
run, so the findings list is empty. The report carries the exit code and stderr.

**The process was killed.** Reviews are long. Raise `timeoutMinutes`, or narrow the scope
with `from`/`to`, `commit`, or `exclude`.

## Development

```bash
npm install
npm run lint        # eslint with neostandard
npm run typecheck   # tsc --noEmit, no build output
npm test            # node --test, with pi.exec mocked
npm run build       # writes dist/package, the publishable tree
npm run release     # builds, then publishes dist/package
```

`npm run check` runs lint, typecheck and tests in sequence.

`src/ocr.ts` holds the host-neutral logic (argument building, validation, output
reading), so the tests exercise it without Pi and without a real `ocr` binary. To load
the extension from a working copy without installing it:

```bash
pi -e ./plugins/open-code-review/pi
```
