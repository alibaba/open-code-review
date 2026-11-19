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

- **Problem and Behavior Changes**: practical value, triggers, before/after behavior
  and affected workflows; a comparison table when multiple scenarios benefit.
- **Compatibility**: material compatibility costs and scope boundaries, alongside
  the affected behavior rather than repeated under a separate heading.
- **Verification and Limits**: inspected test source, captured execution evidence
  and unresolved gaps that affect the conclusions.
- **Pre-merge maintainer decisions**: actions the comparison supports. These can
  be a fix for a demonstrated contract violation, an acceptance check with a
  specific setup and observable pass condition, or a choice with a real tradeoff.

The model writes one Markdown report, without filling nested scenario/impact
objects, relationship IDs or evidence arrays. These sections are prompt guidance,
not a parser-enforced template; headings and order can vary. Interactive text output
renders the report as terminal Markdown. No third summarization
model call is made. Decisions do not become line-level review comments or change
the exit status. The report omits this section when no action is supported;
missing test logs alone do not make a change a merge blocker.

### Terminal rendering

On an interactive terminal, analysis headings, emphasis, inline code, lists and
tables are rendered instead of displaying their Markdown delimiters. Prose wraps
to the terminal width; no report-length limit or summarization is applied. Styling
uses the terminal's default foreground/background and basic ANSI colors, without
querying its background or loading external themes. Section and subsection headings
are blue and bold; inline code stays cyan. Ordinary prose uses the terminal's
default color, and bold emphasis in prose is not promoted to a blue heading.
Tables use content-sized columns with whitespace between them and faint horizontal
rules between rows. Long cells wrap rather than stretching short columns to fill
the terminal. The shared Markdown renderer applies these styles to project summaries as well.

Rendering follows `--color`: `auto` renders only on terminal stdout (except
`TERM=dumb`), `never` keeps Markdown syntax, and `always` also styles redirected stdout
(for example, `| less -R`). `--output <file>` always retains Markdown syntax in text
output, even with `--color=always`. All text destinations remove literal
`[[obs-...]]` citation markers from a display copy before rendering, including
reserved markers inside code blocks. Paragraphs, lists and tables are retained;
nearby citation spacing is cleaned without summarizing the report. JSON `analysis`
and the saved `report.md` remain the original Markdown string with citations.
Text output sanitizes terminal controls; rendering
allows only styling escape sequences, not model-provided terminal commands or
active hyperlinks. Links and images are not fetched. If rendering fails, the CLI
falls back to sanitized Markdown without failing the review.

The normal analysis block puts the comparison and report first, then a single
execution status and evidence directory:

```text
──────── Analysis Report ────────
Comparison: 7e5ce06c -> 6d7c1c75

The report's conclusion and supported behavior changes...

Analysis: completed
Evidence: /path/to/analysis/session
```

Revision hashes are abbreviated for display; full comparison identities remain in
JSON and artifacts. Workspace comparisons still say `workspace (live)`, and an
empty base still says `empty tree`. Execution limits or failures, their reasons,
and delivery errors appear before any available report. If no report was accepted,
the block says so explicitly. An evidence path is shown only when available.
An abnormal Review status (`partial` or `failed`) is also shown before the report,
even when Analysis succeeds, so text exports retain Review execution or delivery
failures. Normal Review states are still omitted from the Analysis block.

Normal stop/submission fields, successful or unknown model self-assessments,
analysis token counts, and analysis retry/compaction details are not printed in
this block. A partial model self-assessment gets a separate investigation-incomplete
notice, without changing execution status or exit behavior. `Analysis: completed`
means execution completed, not that the conclusions are correct or exhaustive.
The evidence directory retains the raw report, observations and recorded run facts
for auditing. Use `--format json` for the full analysis metadata, including retry
details. Review's own diagnostic output is unchanged.

Live progress uses a dim `[ocr]` prefix, cyan tool names, and colored status
markers (cyan start, green success, red failure). Arguments and message text are
unchanged. These styles also follow `--color`, with terminal detection performed
on the actual progress stream: human-readable JSON runs can show colored progress
on terminal stderr while stdout remains a plain JSON document. Redirected streams
stay unstyled in `auto` mode, and `--audience agent` still suppresses progress.

### Report focus

The report is a concise maintainer brief, not an investigation transcript. It leads
with the practical conclusion, not a command invocation or a second report title.
Exact commands belong where they explain usage. The report explains each material effect once: trigger, before/after,
consequence and nearby evidence. Behavior and impact can be combined rather than
repeating a mechanism under two headings. When at least two concrete scenarios have
evidence-supported before/after behavior, the prompt defaults to a compact
Scenario / Before / After table after the conclusion, with labels in the report's
language. Following prose explains only mechanisms, compatibility costs or limits
not already covered by the table. Single changes and unverified old behavior use
prose instead; the model must not invent rows or old behavior to fill a table.

Sections use `##` and subsections use `###`, not standalone bold text. Bold remains
available for emphasis inside paragraphs and lists. Length and section selection
follow the material effects; there is no fixed section quota, word limit or
truncation rule. This is prompt guidance, not a submission-format requirement:
the renderer styles existing headings and tables but does not invent them or
rewrite prose into a comparison table. Existing saved reports are not rewritten.

Exact flags/fields, defaults, branch conditions, compatibility boundaries, failures,
and meaningful uncertainty remain when they affect the conclusion. Empty categories,
generic advice, repeated background, file inventories, tool diaries and closing
recaps are omitted. Verification and important gaps get one compact note instead
of repeated disclaimers. Where action is needed, the report ends with a localized
pre-merge decision heading and short action-led bullets. For example, if a failed
delivery can be marked complete, the fix must preserve retryability; an acceptance
check can fail the first delivery, retry the same item and require exactly one
successful delivery. That is a proposed check, not a claim that it ran. This
section is not an overall approval to merge.
Investigation scope and budgets are unchanged; complete
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
call Review's MCP tools. In combined mode both roles ignore Git environment
overrides for the repository, index, work tree and object store. They disable
lazy fetching and external clean, smudge and process filters; operations that
require a configured filter fail instead of invoking it. This is not an OS
sandbox against hostile local Git config.

Workspace patches use snapshots of authorized regular files and respect the
working tree's `-diff` attributes. Each file is captured separately; the working
tree as a whole is not frozen. Names that cannot be addressed as UTF-8 evidence
paths are represented only as excluded metadata.

Diff preparation and code search use bounded batches of authorized paths instead
of passing every excluded file to Git. Patch selection uses escaped, anchored Git
globs: escaping even an ordinary last character prevents Git treating a file name
as a recursive directory prefix on the other side of the comparison.
Search patterns narrow that scope; returned
matches are checked against the authorized side before observation persistence.
For commit comparisons, exact renames are identified by object ID. If similarity
detection would require an excluded blob, unmatched additions and deletions stay
separate. Workspace renames are matched within each authorized path batch.

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
The Analysis text block shows its own execution status and any abnormal Review
status; it does not repeat the overall state or normal Review states. Display
changes do not alter JSON states or model-declared completeness.

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
Before dispatching a Review group, the scheduler also checks that its estimated
cost fits the shared balance, including usage already incurred by Analysis.
Unlike standalone Review, a shared budget does not admit a final submission
round after exhaustion. Both roles also share the `--max-git-procs` limit.

The top-level token summary counts both roles once. Analysis usage is separately
available in `analysis_meta`. Provider usage takes precedence; absent usage is estimated
from request messages, tool definitions, response text, reasoning, and tool arguments.
When a provider reports a total but only a partial breakdown, the cap and
top-level summary count at least the reported total, without inventing missing
input or output counts. Cache details alone do not replace a usage estimate.
Large change lists
and background inputs are paged through `context_read` if they would overflow the
initial prompt. If even the instructions and tool schemas do not fit, analysis
stops before sending a request.
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
