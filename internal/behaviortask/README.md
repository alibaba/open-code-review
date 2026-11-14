# Behavior / Impact task

This package implements a single, read-only investigation using `llmloop.Task`.
It does not change `ocr review` or `ocr scan`, start MCP, construct a comment
collector, execute repository code, or make a report-summarization model call.
`ocr review --analysis` combines the full Review pipeline with this independent
task through `internal/workflow`. The opt-in flag does not change default review
or scan costs. See [the CLI contract](../../docs/review-analysis.md).

## Preparation boundary

The caller is a trusted preparation layer, not model output. Resolve comparison
endpoints with `agent.ResolveComparisonInput`, then call `PrepareInput` to obtain
the comparison's full authorized diff inventory, including deletions and
renames. It ignores caller-supplied diffs and does not use Review's selected-file
list. Supply captured context with provenance and user exclusions. Direct library
callers bypassing `PrepareInput` remain responsible for binding their diffs to
those endpoints. Neither API rediscovers a PR or fetches missing objects.

`NewRunner` accepts full-SHA range/commit comparisons and workspace comparisons
with a pinned old commit or empty tree. It constructs the existing old/new
`CodeAccess` and creates its own session. Only workspace mode reads the live
working tree; its new side includes staged, unstaged, and untracked files and
does not claim a commit revision or snapshot consistency. Explicit exclusions, built-in
excluded directories, credential-like paths, symlinks, and unsupported paths are
unavailable through reads, searches, enumeration, and diffs. Unchanged authorized
consumers remain readable. This is not a sandbox against a hostile local Git
configuration or a preparation layer that supplies false data.

Pass a fresh `Deps.ArtifactDir` outside the analyzed repository, with an existing
parent directory. Existing directories are rejected. Run once with `Run(ctx)`.
The caller owns the LLM client configuration; the task never selects a different
model. A custom `BehaviorTemplate` can change budgets and language without
changing Review/scan templates. Use `Deps.Language` to set prose language (English
by default);
schema identifiers and enum values remain unchanged.

## Evidence and submission

Every executed code/context read returns and persists an observation with its
comparison identity, normalized arguments, returned text hash, exact observed
locations, truncation, and error classification. Adjacent observed lines share
one range; gaps in search results remain gaps. The adapter does not invent total
search counts. File enumeration conservatively reports a possible truncation at
the existing provider's 100-file cap, including when scope filtering removes hits.

`context_read` can only read injected material or replay an existing observation.
Its cursor is a UTF-8 byte offset and its default page size is 16 KiB (maximum
64 KiB). Replaying an observation does not create another evidence identity.
Context indexes preserve provenance and content hashes. Missing metadata cannot
be replaced by inference about CI or dependency behavior.

`submit_analysis` is the only completion tool. Its v2 contract requires one
`markdown` string and optionally `declared_completion` (`complete`, `partial`, or
`unknown`; omission becomes `unknown`). Markdown is limited to 256 KiB UTF-8 and
must not be whitespace-only. Headings, ordering, lists, tables, and prose are not
parsed into a semantic schema. The prompt requests a concise maintainer brief:
conclusion first, each material effect once, with decisive conditions, consequences
and evidence retained. Behavior and impact may be combined; empty sections, repeated
background and investigation narration are omitted. Brevity does not narrow the
investigation, impose a hard length cap or remove independent risks. Decisions
describe genuine policy choices and tradeoffs rather than code-answerable questions.

Use literal `[[obs-000001]]` markers to cite tool observations. Validation only
checks marker syntax and that each referenced ID exists in this run and has no
error classification. Markers are reserved even inside code blocks. The program
derives `cited_observation_ids`; uncited reports are allowed and have an empty
list. It does not check every prose claim, line location, old/new attribution,
causal relationship, execution claim, or whether evidence supports a conclusion.
Exact observed locations and verification provenance remain in the observation
log for human inspection. Finding a source test still does not prove it ran.

Invalid submissions are repairable within the existing conversation budget.
The first valid report is frozen. A final budget round advertises only
`submit_analysis`; no additional recovery loop is created. `Result.Completed`
means the loop accepted a submission, not that the investigation is complete or
correct. `workflow.Report.Status` describes execution, separately from the
model-declared completeness. A self-declared partial report can be delivered
successfully; a complete declaration never overrides a budget stop, cancellation,
provider error or artifact persistence failure.

## Artifacts and validation

- `result.json`: `ocr.behavior-impact/v2`, separating model analysis from program
  comparison, scope, submission state, stop reason, usage, session, and versions.
- `report.md`: the accepted Markdown exactly as submitted, without generated headings.
- `observations.jsonl`: append-only, run-local evidence. Persistence failures stop
  the task rather than accepting evidence that cannot be recovered.
- The normal session history captures model calls, compression, and tool results.
  Background work is joined before session and result finalization.

Run unit tests through `make test PACKAGES='./internal/behaviortask'`.
Set `OCR_BEHAVIOR_REPLAY` to a verified handoff replay directory to additionally
exercise the three fixed smoke inputs with a **fixture client**, not a real model.
That test checks engineering integration and preserves explicit partial results;
it neither reads evaluator rubrics nor measures Behavior/Impact quality. Run the
handoff helper's verification before and after. Do not format or run tests in the
historical source views.

Still pending: fixed-model scored replay and ablation (2A-4). Snapshot integrity,
fixture-client tests, and binary replay against a local mock endpoint are
engineering validation, not acceptance of real model analysis quality.
