# Behavior / Impact task

This package implements a single, read-only investigation using `llmloop.Task`.
It does not change `ocr review` or `ocr scan`, start MCP, construct a comment
collector, execute repository code, or make a report-summarization model call.
`ocr review --commit <sha> --analysis` and `ocr review --from <base> [--to <head>] --analysis`
combine the full Review pipeline with this independent task through `internal/workflow`.
For uncommitted changes use ordinary `ocr review` without `--analysis`. The opt-in flag does not change default review
or scan costs. Both roles use the configured model and private sessions. Analysis
uses at most one additional concurrent model request. Both roles use the resolved
`--max-tokens` limit for each prompt and share one `--max-git-procs` pool.
Analysis has no total token
limit by default (`--analysis-max-tokens-budget 0`). A positive limit counts tokens
from investigation, compression and final submission. A request in progress and
one submission-only final round can take the total above the limit. The Review summary and Analysis metadata
report their respective usage. An Analysis failure does not change Review's exit
status; command cancellation applies to both roles.

## Preparation boundary

The caller is a trusted preparation layer, not model output. Resolve comparison
endpoints once in the Review preparation path, then pass the resulting SHA values
to `PrepareInput` to obtain
the comparison's full authorized diff inventory, including deletions and
renames. It ignores caller-supplied diffs and does not use Review's selected-file
list. Both roles filter changed paths using the root `.gitignore` from the head
commit. This filters diffs, not reads or searches for supporting context.
Supply captured context with provenance and user exclusions. Direct library
callers bypassing `PrepareInput` remain responsible for binding their diffs to
those endpoints. Commit and range patches use the head revision's versioned
attributes. If the active Git environment overrides the repository, index or
command configuration in a way Analysis cannot reproduce, it is skipped. Neither API rediscovers a PR or fetches missing objects.

`NewRunner` accepts full-SHA range and commit comparisons; the old side of a
root commit is the empty tree. It constructs the existing old/new `CodeAccess`
and creates its own session. File contents come from the selected revisions,
not the live working tree. Explicit exclusions, built-in
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
search counts. File enumeration matches only authorized paths and reports truncation
when more than 100 authorized paths match.

`context_read` reads injected material, paginated inventory indexes, or an existing observation.
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
investigation, impose a hard length cap or remove independent risks. When the
evidence warrants action, the brief ends with a fix for a demonstrated contract
violation, a concrete acceptance check, or a genuine maintainer choice. An
unverified hypothesis is not a blocker and a proposed check is not a test result.

Use literal `[[obs-000001]]` markers to cite tool observations. Validation only
checks marker syntax and that each referenced ID exists in this run and has no
error classification. Markers are reserved even inside code blocks. The program
derives `cited_observation_ids`; uncited reports are allowed and have an empty
list. It does not check every prose claim, line location, old/new attribution,
causal relationship, execution claim, or whether evidence supports a conclusion.
Exact observed locations and verification provenance remain in the observation
log for human inspection. Finding a source test still does not prove it ran.

Invalid submissions are repairable within the existing conversation budget.
The first valid report is frozen. Analysis defaults to 300 investigation rounds.
If it exhausts its round or token budget, it can make at most one additional
round with only `submit_analysis`; an invalid final submission is not retried. `Result.Completed`
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
