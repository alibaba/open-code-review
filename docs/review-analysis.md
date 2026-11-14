# Review with Behavior and Impact analysis

Run the existing Review pipeline and a separate read-only analysis together:

```sh
ocr review --analysis
ocr review --from main --to feature --analysis
ocr review --commit abc123 --analysis --format json --output review.json
```

Without `--analysis`, review and scan keep their existing tools, output contract,
and model-call cost. This is not a replacement for line-level review comments.
With the flag, the terminal prints an **Analysis Report** alongside the comments.
It covers the following when relevant, not as a mandatory list of sections:

- **Behavior**: triggers, preconditions, before/after behavior and source evidence.
- **Impact**: affected workflows, propagation, compatibility and attribution.
- **Maintainer Decisions**: evidence-backed policy choices, options and tradeoffs.
- **Verification**: source tests separately from captured execution evidence.
- **Questions & Limitations**: missing facts, uncertainty and execution failures.

The model writes one Markdown report, without filling nested scenario/impact
objects, relationship IDs or evidence arrays. These sections are prompt guidance,
not a parser-enforced template; headings and order can vary. Interactive text output
renders the report as terminal Markdown. No third summarization
model call is made. Decisions are not defects and do not become review comments.
Missing factual evidence is an open question, not a policy decision. Empty sections,
including a decision section when there is no genuine choice, should be omitted.

### Terminal rendering

On an interactive terminal, analysis headings, emphasis, inline code, lists and
tables are rendered instead of displaying their Markdown delimiters. Prose wraps
to the terminal width; no report-length limit or summarization is applied. Styling
uses the terminal's default foreground/background and basic ANSI colors, without
querying its background or loading external themes.

Rendering follows `--color`: `auto` renders only on terminal stdout (except
`TERM=dumb`), `never` keeps raw Markdown, and `always` also styles redirected stdout
(for example, `| less -R`). `--output <file>` always retains raw Markdown in text
output, even with `--color=always`. JSON `analysis` and the saved `report.md` remain
the original Markdown string. Text output sanitizes terminal controls; rendering
allows only styling escape sequences, not model-provided terminal commands or
active hyperlinks. Links and images are not fetched. If rendering fails, the CLI
falls back to sanitized Markdown without failing the review.

Live progress uses a dim `[ocr]` prefix, cyan tool names, and colored status
markers (cyan start, green success, red failure). Arguments and message text are
unchanged. These styles also follow `--color`, with terminal detection performed
on the actual progress stream: human-readable JSON runs can show colored progress
on terminal stderr while stdout remains a plain JSON document. Redirected streams
stay unstyled in `auto` mode, and `--audience agent` still suppresses progress.

### Report focus

The report is a concise maintainer brief, not an investigation transcript. It leads
with a takeaway and explains each material effect once: trigger, before/after,
consequence and nearby evidence. Behavior and impact can be combined rather than
repeating a mechanism under two headings. A small change should normally fit on
one screen; complex changes can expand to retain independent risks and decisive
details. This is guidance, not a word limit or a truncation rule.

Exact flags/fields, defaults, branch conditions, compatibility boundaries, failures,
and meaningful uncertainty remain when they affect the conclusion. Empty categories,
generic advice, repeated background, file inventories, tool diaries and closing
recaps are omitted. Verification and important gaps get one compact note instead
of repeated disclaimers. Investigation scope and budgets are unchanged; complete
observations and run metadata remain in their own artifacts, not repeated in prose.

Text reports omit per-observation read errors, search truncation notices, and
observation pagination hints. These are investigation diagnostics, not a reliable
measure of unresolved evidence gaps. The report should describe material gaps
once, with their effect on the conclusions; successful submission alone does not
prove recovery. Execution limits, delivery errors, and other warning types remain
visible. JSON `analysis_meta.warnings`, session records, and saved analysis
artifacts retain the complete diagnostics; text formatting does not modify them.

## Inputs and scope

Without revision flags, analysis covers staged, unstaged, and untracked workspace
changes. Both roles share the pinned HEAD as their old side, or an empty old tree
for an unborn repository. The new side is the live working tree, not the index
alone or an immutable snapshot; avoid editing files during a run. Workspace
comparison metadata has `mode: "workspace"` and an empty `head`, and new-side
observations do not claim a commit revision.

`--commit` and a complete `--from`/`--to` range retain immutable comparisons.
Range mode uses the merge base; root commits use the empty old tree. No checkout,
fetch, build, or repository code execution is performed by the analysis task.

Only `text` and `json` formats are supported. `--preview`, `--resume`, and SARIF
are rejected rather than silently dropping part of the report.
To rerun a combined investigation, start a fresh run. The Review session can still
be inspected independently; do not resume it expecting analysis to be resumed.

Analysis considers the full authorized change inventory, including deleted,
renamed and Review-filtered files. It can follow unchanged consumers on either
side. User `--exclude` patterns and rule exclusions apply to both roles; Review
include/size/selection filters do not shrink the analysis inventory. Credential-like
paths, symlinks, built-in excluded directories and unsupported paths are denied
to analysis tools. Excluded and binary changes remain visible as scope metadata,
not silently claimed as analyzed source. If either side of a rename is excluded,
its diff is excluded. `--background` and `--background-file` are captured with
provenance; analysis cannot fetch arbitrary external material, run commands or
call Review's MCP tools. This is not a sandbox against hostile local Git config.

Diff preparation and code search use bounded batches of authorized paths instead
of passing every excluded file to Git. Search patterns narrow that scope; returned
matches are checked against the authorized side before observation persistence.

## JSON and status contract

Combined output has `schema_version: "ocr.review-with-analysis/v2"`. Existing
`comments`, `groups`, `manifest`, warnings and Review retry diagnostics are retained.
The peer `analysis` field is the whole Markdown report as a **string**. The program
places run facts in `analysis_meta` (`ocr.review-analysis/v2`), including comparison,
scope, observations, usage, session, versions, diagnostics and independent states.
For example, a successfully delivered report can admit incomplete investigation:

```json
{
  "schema_version": "ocr.review-with-analysis/v2",
  "status": "completed",
  "review_status": "complete",
  "comments": [],
  "analysis": "## Behavior\nSupported observations...\n\n## Limitations\nConsumer behavior remains unverified.",
  "analysis_meta": {
    "schema_version": "ocr.review-analysis/v2",
    "execution_status": "completed",
    "submission_accepted": true,
    "declared_completion": "partial",
    "cited_observation_ids": []
  }
}
```

`review_status` retains Review's own `complete`, `partial`, `failed`, or `skipped`.
`analysis_meta.execution_status` is program-recorded: `completed` when a valid
report was accepted without execution errors or budget stops; `partial` when an
accepted report exists but execution was limited or delivery failed; `failed`
when no report was accepted. `submission_accepted` only records tool-loop
acceptance. Neither field certifies analysis quality or completeness.

`declared_completion` is a separate optional **model self-assessment** (`complete`,
`partial`, `unknown`; omission means `unknown`). It does not change execution
status or process exit by itself. Consumers needing complete investigations must
inspect this field and the report, not infer completeness from exit code zero.

The top-level `status` combines execution outcomes: `completed` when Review
completed (or selected no files) and analysis execution completed; `failed` when
both failed; otherwise `partial`. Review run failures and analysis execution
failures/limits return nonzero **after** publishing available output. Review-only
controlled partial coverage retains its existing exit contract. A model's partial
or unknown self-assessment alone does not cause a nonzero exit. Failed analysis
emits `analysis: ""` and its diagnostics, not a fabricated report.

### Observation citations

The lightweight submission accepts `markdown` and optional `declared_completion`.
Only nonempty content, a 256 KiB UTF-8 size limit, the optional enum and literal
observation markers are validated. Use `[[obs-000001]]` citations to observations
actually returned by the tools. Unknown, failed or malformed markers are rejected
with repair feedback. Valid repeated citations are deduplicated in program-derived
`cited_observation_ids`; `observation_ids` retains all observations. The marker
syntax is reserved, including inside code blocks. Reports without citations are
allowed; an empty citation list is not evidence of correctness.

This validates observation **existence**, not semantic support, prose source
locations, old/new attribution, causal links, or verification claims. Observations
still record exact source locations and captured verification provenance so readers
can audit them. Source tests are never automatically promoted to execution proof.
Treat Markdown as untrusted model/repository content: downstream HTML renderers
should sanitize raw HTML and control external links/images rather than executing
or automatically fetching them. The CLI does not render HTML or fetch links.

### Migration from the initial v1 prototype

The combined schema and analysis metadata are v2 because `analysis` changes from
an object to a string. Use `analysis_meta` for run metadata instead of looking
inside `analysis`. Nested behavior/impact/decision arrays are removed. A partial
self-assessment no longer means an execution failure. Default review JSON without
`--analysis` is unchanged and does not emit these fields.

## Resources and artifacts

Both roles run concurrently with separate sessions, observations, raw request
writers and retry collectors. A role returning an error does not cancel its peer;
parent cancellation reaches both and background work is joined before reports
are frozen. `--concurrency` caps all concurrent logical model calls across roles,
including Review grouping/planning/filtering/relocation and memory compression.
`--max-tokens-budget` is shared across those calls. It is an admission limit:
already admitted calls can finish and overshoot it, but no new model request is
admitted after the accumulated input plus output usage reaches the limit. Budget
rejection is not a provider error and is not retried by HTTP transport.

The top-level token summary counts both roles once. Analysis usage is separately
available in `analysis_meta`. Provider usage takes precedence; absent usage is estimated
from request messages, tool definitions, response text, reasoning, and tool arguments.
`--timeout` bounds analysis preparation and execution together. Preparation runs in
the analysis branch, so slow or failed preparation does not block the Review branch.
`--max-tools` bounds the single investigation loop.
`--effort` continues to control Review rounds, not an extra analysis loop.

Analysis artifacts are written outside the analyzed repository to
`~/.opencodereview/analysis/<review-session-id>/`:

- `report.md`: the exact accepted Markdown, if a submission was accepted.
- `result.json`: the v2 submission plus program-recorded run facts.
- `observations.jsonl`: append-only evidence returned by read-only tools.

`analysis_meta.session_id` is analysis's own session, different from the top-level
Review session. `analysis_meta.artifact_dir` locates its files. Markdown write
failures preserve the in-memory report in CLI output and produce execution errors;
existing artifact files are never overwritten. Reports may contain repository
source; apply the same retention controls as review sessions.

## Validation boundaries

Run `make test` and `make coverage` for unit and local HTTP integration checks.
Build with `make build` before replaying fixed comparisons with
`dist/opencodereview review --analysis`. Engineering replay with a mock model
validates the binary, flags, Git scope, tools, output, usage and failure handling;
it does not measure real-model analysis quality. Keep evaluator rubrics and later
fixes out of the model context, and never execute historical replay source.
