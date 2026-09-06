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

### "No tool calls parsed" (local models / Ollama)

```
[ocr] No tool calls parsed for src/foo.go, retrying...
[ocr] Max tool requests reached for src/foo.go.
```

If every review loops through `No tool calls parsed` retries and ends
with "Max tool requests reached" and zero comments, the model — not the
config — is the problem. OCR drives the review entirely through tool
calls, so **the model must support native tool calling (function
calling)**. A model that merely *narrates* tool calls in its text
output (or inside `<think>` blocks) can never work with OCR, no matter
how the prompt is tuned — `deepseek-r1` is a common example. Models
with native tool support, such as `qwen3`, work fine. For Ollama, pick
from the models tagged with tools support:
<https://ollama.com/search?c=tools>.

Verify a local model directly, without OCR in the loop:

```bash
curl http://127.0.0.1:11434/v1/chat/completions -H "Content-Type: application/json" -d '{
  "model": "qwen3:32b",
  "messages": [{"role": "user", "content": "The code below has a bug, use the report_bug tool to report it.\n\nfunc add(a, b int) int {\n  return a - b\n}"}],
  "tools": [{"type": "function", "function": {"name": "report_bug", "description": "Report a bug in the code",
    "parameters": {"type": "object", "properties": {"line": {"type": "integer"}, "description": {"type": "string"}}, "required": ["description"]}}}]
}'
```

Pass: the response contains a structured `tool_calls` array naming
`report_bug`. Fail: the "call" appears as text inside `content`.

If the model *does* support tools but responses are slow on local
hardware, raise the LLM timeout instead — see
[Timeouts](../configuration/#timeouts).

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
and look at the `main_task` lane of the group that contains the file
(groups are keyed by their file paths, so a file reviewed on its own
appears under its own path):

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
[ocr] WARNING: prompt tokens (240000) exceed 80% of max_tokens(200000) [round 1] for group "src/big.sql"
```

The initial prompt for that group (rule + diffs + change-files list) was
already past 80 % of `MAX_TOKENS = 200000` before the model could even
respond. OCR skips the group and continues — you'll see this in
`warnings` in JSON mode too.

`MAX_TOKENS` is the **prompt** ceiling only. The model's output is capped
independently by `MAX_COMPLETION_TOKENS` (`16384`), so this warning is
always about input size.

Mitigations:

- Add the file to your `exclude` list if it's autogenerated.
- Split a large refactor into smaller commits.
- Use `--commit` mode for a series of small commits rather than
  reviewing them all at once via workspace mode.

### Plan phase took forever and the file is small

Run `ocr review --preview` first. The plan phase runs when **either**
threshold is crossed:

- The largest file in the group changed at least
  `PLAN_MODE_LINE_THRESHOLD` lines (default **50**), **or**
- The group holds 2+ files and their combined `lines.changed` reaches
  `PLAN_MODE_GROUP_LINE_THRESHOLD` (default **100**).

So a small file can still get a plan pass if it was grouped with other
changes that add up. That's by design — large or wide diffs benefit
from a planning pass. To skip it for a single review, run with a
smaller diff, or temporarily edit the embedded template (advanced;
you'll need to override `--tools`).

### "Max tool requests reached"

```
[ocr] Max tool requests reached for src/foo.go.
```

The model spent all 100 (`MAX_TOOL_REQUEST_TIMES`) tool-use rounds
without calling `task_done`. Comments emitted up to that point are
still collected and rendered. If this happens on most groups, the issue
is usually one of:

- Model isn't great at following the "call `task_done` when finished"
  instruction. Switch to a stronger model (e.g., Claude Opus).
- A tool keeps erroring and the model keeps retrying. Look at the
  session JSONL — if the same tool result repeats, that's why.
- The group is genuinely large or context-heavy and 100 rounds isn't
  enough. Raise the cap with `--max-tools <n>` (e.g., `--max-tools
  150`). The flag only ever *raises* the limit — a value below the
  template default of `100` is ignored, values 1–49 are clamped up to
  `50`, and `0` keeps the template default.
- The model does not support native tool calling at all (common with
  local models) — see
  ["No tool calls parsed" (local models / Ollama)](#no-tool-calls-parsed-local-models-ollama).

### Some sub-agents fail; the run still exits 0

By design. OCR isolates per-group failures so one bad group doesn't kill
a 20-file review. The aggregate exit code is `0` if *anything*
succeeded; only a fully-failed run (zero successful sub-agents) exits
non-zero. Check the `warnings` array in JSON mode or stderr in text
mode to see which groups failed.

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

Nothing was eligible to review. `files_reviewed` is not a top-level
field — it sits under `summary`, where it reads `0` on this path, while
`comments` is the top-level `[]`. The same object carries
`"status": "skipped"`, `"message": "Review skipped: no items were
selected."`, and a `manifest` whose `terminal_state` is `"skipped"` with
every `coverage` array empty.

A review that examined files and found nothing also returns **a JSON
object with `comments: []`**: `summary.files_reviewed` counts the files
actually reviewed, `status` is `"complete"`, and `message` reads
`"Review complete: 0 finding(s) across N selected item(s)."`. Optional
top-level fields can differ between runs, so do not distinguish these
states by object shape or by the presence of an optional key. Use
`summary.files_reviewed` or `manifest.terminal_state` instead.
`review --format json` always writes exactly one JSON object to stdout,
never a bare array.

The no-files path for `ocr scan` is leaner: it omits `summary`, while
still reporting `"status": "skipped"`, `"message": "No supported files
changed."`, and `"comments": []`. Other normal output fields such as
`tool_calls`, and optional metadata such as `llm` or `trace_id`, may
also be present.

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

- The effort preset controls how many review rounds each group gets:
  `low` = 1, `medium` (the default) = 2, `high` = 3. Cost scales roughly
  with the round count, so `--effort low` is the single biggest lever if
  you want a cheaper run; `--effort high` is the most expensive.
- Plan phase is on for groups whose largest file is ≥ 50 lines, or whose
  2+ files total ≥ 100 lines. It costs an extra LLM call per group, so
  **raising** those thresholds is what makes a run cheaper; lowering
  them sends more groups through planning and costs more. The two do not
  behave alike at zero. `PLAN_MODE_LINE_THRESHOLD` at `0` or below means
  *always plan* — the dearest setting available — whereas
  `PLAN_MODE_GROUP_LINE_THRESHOLD` at `0` turns the group gate off. That
  can save the plan call when the group gate would otherwise be the only
  trigger. See "Plan phase took forever and the file is small" above for
  the trigger rules.
- `MAX_TOOL_REQUEST_TIMES = 100` is generous. A model that uses every
  round will produce a longer (more tokens) conversation than one that
  finishes in 3 rounds. Stronger models tend to finish faster.
  Conversely, if you raised it with `--max-tools` to fight "max tool
  requests reached", expect cost per group to grow roughly linearly.
- Memory compression itself is an LLM call. Long subtasks pay for
  compression rounds in addition to review rounds.
- Semantic grouping adds one small LLM call per run. It only sees file
  metadata (paths, status, insertion/deletion counts) — never diff
  content — so it is cheap, and it usually pays for itself by reviewing
  related files together instead of once per file.

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
