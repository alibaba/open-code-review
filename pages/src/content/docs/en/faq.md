---
title: FAQ
sidebar:
  order: 14
---

Common errors, surprises, and "is this supposed to do that?" questions.
If your problem isn't here, open a
[GitHub issue](https://github.com/alibaba/open-code-review/issues) with
the steps you ran and the full output.

## Configuration & startup

### `no valid LLM endpoint configured`

```
no valid LLM endpoint configured; one of OCR_LLM_URL/OCR_LLM_TOKEN/OCR_LLM_MODEL,
~/.opencodereview/config.json, or ANTHROPIC_BASE_URL/ANTHROPIC_AUTH_TOKEN/
ANTHROPIC_MODEL must be set
```

OCR ran the full endpoint-resolution chain ([Configuration](../configuration/#reuse-existing-environment-variables))
and didn't find a complete `(URL, token, model)` triple. Either:

- Run `ocr config set llm.url …` / `llm.auth_token …` / `llm.model …`
  to populate `~/.opencodereview/config.json`, **or**
- Export `OCR_LLM_URL` / `OCR_LLM_TOKEN` / `OCR_LLM_MODEL`, **or**
- Export `ANTHROPIC_BASE_URL` / `ANTHROPIC_AUTH_TOKEN` /
  `ANTHROPIC_MODEL` if you already use Claude Code.

Then `ocr llm test` to verify connectivity before retrying the review.

### `ocr llm test` shows the wrong source

OCR uses the **first** complete triple, not the last. So if your
config file has all three llm.* keys, env vars are ignored. To make
env wins, either delete the config keys (`rm` the file or unset by
hand) or use `ocr config set` to switch to the new values.

### 401 / 403 from `ocr llm test`

The token is missing scope, expired, or wrong vendor. Anthropic and
OpenAI use different auth headers and different URL shapes — make sure
`llm.use_anthropic` matches the URL you're pointing at:

- Anthropic: URL ends `/v1/messages`, `use_anthropic=true`.
- OpenAI / OpenAI-compatible: URL ends `/v1/chat/completions`,
  `use_anthropic=false`.

### `not a git repository`

`ocr review` runs `git diff` (and `git ls-files` for untracked files)
against the current directory. If you're not inside a Git working tree,
it exits early. Either `cd` into a repo, or pass `--repo /path/to/repo`.

## Filtering & rules

### My file isn't being reviewed

Run `ocr review --preview` (no LLM cost). The output lists every
candidate file with the **reason** it was kept or dropped:

```
src/foo.go              modified
src/foo_test.go         modified  (excluded: user_exclude)
node_modules/lib.js     added     (excluded: default_path)
imgs/logo.png           binary    (excluded: unsupported_ext)
```

The five exclusion reasons map to gates in the
[file filter](../review-rules/#how-files-are-filtered):

| Reason | Fix |
|---|---|
| `binary` | Nothing to do — binary files have no reviewable text. |
| `user_exclude` | Remove the pattern from your `exclude` list. |
| `unsupported_ext` | Add the extension to your `include` list to bypass the allowlist gate. |
| `default_path` | Add the file to `include` — that overrides built-in test-file exclude patterns. |
| `deleted` | Nothing to do — there's no new content to review. |

### My custom rule isn't firing

Run `ocr rules check <file-path>`. It prints the **layer** and
**glob pattern** that matched, end-to-end:

```
File: src/api/UserHandler.go
Source: Project (.opencodereview/rule.json)
Pattern: src/api/**/*.go
Rule: …
```

If the layer is wrong (e.g., showing "System built-in" when you expected
your project rule), most likely the **declaration order** matters — the
first matching pattern wins. Move your more-specific rule earlier in the
`rules` array, or fix the glob.

### Brace expansion isn't working

`bmatcuk/doublestar/v4` supports `{ts,tsx}` braces. If they're not
matching, check for stray spaces — `{ts, tsx}` with a space silently
fails to match `tsx`.

## Reviews

### A file shows zero comments — was it actually reviewed?

Open the [Session Viewer](../viewer/) (`ocr viewer`), find the session,
and look at the file's `main_task` lane:

- Tool calls present + ends in `task_done` → reviewed cleanly.
- Tool calls present + ends mid-loop → look for an error card.
- No `main_task` cards at all → the file was filtered out before review;
  see [Filtering & rules](#filtering--rules) above.

### Comments have `start_line: 0` and `end_line: 0`

OCR couldn't anchor the comment to a precise line in the diff. Two
common causes:

- The model paraphrased `existing_code` instead of copying it verbatim
  from the diff. The model is told not to, but it sometimes does.
- The diff had unusual formatting (CRLF, mixed tabs/spaces) that broke
  the sliding-window match.

The comment is still real — it just wasn't placed automatically. Most
agent integrations (the SKILL, the Claude Code plugin) read the
`existing_code` field and locate the spot in the file themselves.

### Token threshold exceeded

```
[ocr] WARNING: prompt tokens (94000) exceed 80% of max_tokens(58888) for src/big.sql
```

The initial prompt for that file (rule + diff + change-files list) was
already past 80 % of `MAX_TOKENS = 58888` before the model could even
respond. OCR skips the file and continues — you'll see this in
`warnings` in JSON mode too.

Mitigations:

- Add the file to your `exclude` list if it's autogenerated.
- Split a large refactor into smaller commits.
- Use `--commit` mode for a series of small commits rather than
  reviewing them all at once via workspace mode.

### Plan phase took forever and the file is small

Run `ocr review --preview` first. If the file's `lines.changed` is
above `PLAN_MODE_LINE_THRESHOLD` (default **50**), the plan phase runs.
That's by design — large diffs benefit from a planning pass. To skip
it for a single review, run with a smaller diff, or temporarily edit
the embedded template (advanced; you'll need to override `--tools`).

### "Max tool requests reached"

```
[ocr] Max tool requests reached for src/foo.go.
```

The model spent 30 (`MAX_TOOL_REQUEST_TIMES`) tool-use rounds without
calling `task_done`. Comments emitted up to that point are still
collected and rendered. If this happens on most files, the issue is
usually one of:

- Model isn't great at following the "call `task_done` when finished"
  instruction. Switch to a stronger model (e.g., Claude Opus).
- A tool keeps erroring and the model keeps retrying. Look at the
  session JSONL — if the same tool result repeats, that's why.
- The file is genuinely large or context-heavy and 30 rounds isn't
  enough. Raise or lower the cap with `--max-tools <n>` (e.g.,
  `--max-tools 40` for more, `--max-tools 15` for fewer). Values 1–9
  are clamped up to 10; `0` (the default) uses the template default of
  30.

### Some sub-agents fail; the run still exits 0

By design. OCR isolates per-file failures so one bad file doesn't kill
a 20-file review. The aggregate exit code is `0` if *anything*
succeeded; only a fully-failed run (zero successful sub-agents) exits
non-zero. Check the `warnings` array in JSON mode or stderr in text
mode to see which files failed.

### CI run is much slower than local

Two usual suspects:

- **Model rate limits** — under throttling, the LLM client backs off
  and retries. Lower `--concurrency` (e.g., to `4`) so you don't hit
  the limit in the first place.
- **Cold cache** — if your provider supports prompt caching, the first
  run after deploy doesn't benefit from it. Subsequent runs in the
  same window are faster.

## Output & integration

### `--audience agent` still has progress lines

Make sure you're not seeing **stderr**. Progress messages occasionally
go to stderr (warnings, errors). The clean stdout that `--audience
agent` guarantees is *parser-friendly* — to suppress everything,
redirect: `ocr review --audience agent 2>/dev/null`.

### JSON output is `{ "files_reviewed": 0, "comments": [] }`

Workspace had no eligible files. This is intentional — the explicit
shape lets callers distinguish "nothing to review" from "no findings
found in the reviewed files". A normal review with zero comments
produces a regular empty array `[]` instead.

### Where do session JSONLs live?

```
~/.opencodereview/sessions/<path-encoded-repo-path>/<session-id>.jsonl
```

The repo path is encoded by replacing `/` and `\` with `-` and `:` with
`_` (e.g. `/Users/foo/my-repo` → `Users-foo-my-repo`). Browse sessions
with `ocr viewer`. Delete the directory to wipe history; OCR regenerates
the encoded path on the next run.

## Performance & cost

### How can I tell what tokens cost what?

Enable telemetry:

```bash
ocr config set telemetry.enabled true
ocr config set telemetry.exporter console
ocr review
```

LLM calls don't get their own spans — they're recorded as metrics
instead. Watch `ocr.llm.tokens_used` (counter, labelled `model` +
`type`), `ocr.llm.requests_total` (counter, labelled `model` +
`status`), and `ocr.llm.request_duration_seconds` (histogram, labelled
`model`). The console exporter prints these aggregates inline. For
dashboards, switch to the OTLP exporter and ship to your metrics
stack — see [Telemetry](../telemetry/).

### Why are my reviews so expensive?

Common levers:

- Plan phase is on for files ≥ 50 lines. It costs an extra LLM call
  per file. Lowering the threshold reduces cost; raising it improves
  small-PR speed.
- `MAX_TOOL_REQUEST_TIMES = 30` is generous. A model that uses every
  round will produce a longer (more tokens) conversation than one that
  finishes in 3 rounds. Stronger models tend to finish faster.
  Conversely, if you raised it with `--max-tools` to fight "max tool
  requests reached", expect cost per file to grow roughly linearly.
- Memory compression itself is an LLM call. Long subtasks pay for
  compression rounds in addition to review rounds.

### How do I reduce LLM calls?

- Add an `include` list so OCR doesn't review files you don't care
  about.
- Set `--concurrency` lower if your account has burst-mode pricing.
- Pass `--background` — better context up-front sometimes lets the
  model finish without `file_read` / `code_search` round-trips.

## Privacy & security

### Does OCR send my code anywhere?

OCR sends your **diffs** (and optional read-tool snippets) to whatever
LLM endpoint you configured. Nothing else leaves your machine —
session JSONLs and rule files are local-only.

If telemetry is enabled, the `content_logging` flag is plumbed through
the config layer but currently gates **no** code path — prompt and
response content is never exported to your collector regardless of the
flag's value. Treat it as reserved. Leave it `false` in production. See
[Telemetry](../telemetry/#content-logging) for details.

### Can I redact secrets before they're sent to the LLM?

Not built-in. The recommended workflow:

1. Don't commit secrets to your repo (the usual rule).
2. Add files known to contain hash material to `exclude`.
3. Use `git diff --no-textconv` filters or pre-commit redaction to keep
   secrets out of diffs.

A "redaction rule" feature is on the roadmap; track
[the issue tracker](https://github.com/alibaba/open-code-review/issues).

## Misc

### Where's the changelog?

[GitHub Releases](https://github.com/alibaba/open-code-review/releases)
— each release has notes generated from Conventional Commits.

### Does OCR support non-Git VCS?

No. The diff providers shell out to `git`. SVN / Mercurial / etc. would
need new providers; an issue for Hg support is open
[here](https://github.com/alibaba/open-code-review/issues).

### Why is the binary called `opencodereview` but the CLI is `ocr`?

The static binary published in releases is named after the project
(`opencodereview`); the NPM wrapper installs it as `ocr` for
ergonomics. If you build from source you get `dist/opencodereview` —
copy it to `ocr` on your `$PATH`.

### How do I uninstall?

```bash
npm uninstall -g @alibaba-group/open-code-review        # NPM install
sudo rm /usr/local/bin/ocr                              # binary install
rm -rf ~/.opencodereview                                # all state
```

OCR doesn't write outside `~/.opencodereview` (apart from the binary
download via NPM), so removing that directory wipes history, config,
and per-user rules.

## See Also

- [Configuration](../configuration/) — LLM endpoint resolution and config keys.
- [Review Rules](../review-rules/) — the file filter and rule resolution chain.
- [Session Viewer](../viewer/) — inspect past review sessions.
- [Telemetry](../telemetry/) — token usage and LLM metrics.
