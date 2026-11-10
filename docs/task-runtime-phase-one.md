# Task runtime: phase one

This is an internal, behavior-preserving extraction. It adds no command, agent,
model request, output format, or session migration. Behavior analysis and a
combined report remain outside this phase.

## Ownership

- `internal/llmloop` owns model requests, context compression, cancellation,
  round/token limits, session records, request identity, and aggregate metrics.
- `llmloop.Task` supplies tool definitions and an executor, completion/context
  limits, empty-response guidance, compression templates, and final-round
  messages/tools. Initial messages and the task key are passed to `Run`.
- `ToolOutcome` returns model-visible content independently of its action:
  `Continue`, `Complete`, or `Fail`. Recoverable argument/execution errors return
  `Continue` with corrective text; only an explicit task failure returns `Fail`.
- `RunResult.Completed` is independent of `StopReason`. A successful final round
  can complete a task while retaining a round/token-budget stop reason.
- `internal/reviewtask` adapts the existing review/scan configuration. It owns
  completion-tool validation, comment repair, deterministic and model-assisted
  relocation, collection, asynchronous workers, retry guidance, and the existing
  review-limit warning. Ordinary tools, including MCP tools, retain their string
  provider interface behind this adapter.

The dependency direction is `agent` / `scan` -> `reviewtask` -> `llmloop`.
The loop does not import review templates, diffs, comment collectors, or tools.
Relocation uses the runtime's shared request-context and usage APIs, preserving
its original request identity and accounting.
Tool executors record calls and failures through the runtime's shared metrics
APIs. The review completion tool keeps its original uncounted, unrecorded
behavior rather than acquiring ordinary-tool accounting during the extraction.

## Lifecycle and compatibility

Review and scan keep their existing per-key comment waits and run-boundary
drains. The runtime's `WaitBackground` joins compression only; the adapter's
`CollectPendingComments` joins comment processing only. Session finalization and
retry-report freezing remain after the original waits.

The stored task names (`main_task`, `memory_compression_task`, and
`re_location_task`), request numbering, cache-affinity keys, warning strings,
CLI/JSON fields, manifest classification, and resume behavior are unchanged.
Review-specific compression text and summary delimiters are injected unchanged,
not replaced by newly worded generic prompts.

## Verification and evaluation gate

`internal/reviewtask/testdata/equivalence` captures fixed-response execution from
the pre-extraction implementation at `34517e30`. The 20 fixtures compare complete
model requests, tool definitions/order, tool results, comments, warnings,
completion/stop states, usage, and session/request associations. Random
conversation/session identifiers and durations are normalized; affinity is
checked against the task identity before normalization.

The `repaired` cases recover a serialized comment batch with unescaped quotes,
preserve both findings and their quoted prose, and emit `comment_args_repaired`
without tool failures. Explicit assertions guard these outcomes even when
regenerating the fixtures. The `invalid_comment_shape` cases separately preserve
outer-argument trailing-text recovery followed by rejection of a non-array batch.

Existing relocation, worker-isolation, compression, retry-identity, manifest,
scan, CLI, and resume tests continue to protect their respective boundaries.
`internal/llmloop/task_test.go` exercises a test-only data-reading/submission task
with no comment tool, diff lookup, or collector, including validation retries,
custom final-round tools, cancellation, and task-specific compression.

Run `make check`, `make test`, and `make coverage`. On hosts that forbid executing
temporary artifacts outside the repository, set `GOTMPDIR` and `TMPDIR` to a
Git-ignored local directory. Set `GIT_CEILING_DIRECTORIES` to that directory's
parent so non-repository fixtures cannot accidentally discover the project.

Engineering equivalence is not model-quality acceptance. Freeze this candidate
for maintainer evaluation using the same model, inputs, settings, and concurrency
as the baseline; compare findings, coverage, failures, usage, latency, and request
traces for anomalous samples. Do not start phase two until the maintainer has
completed real-model evaluation and explicitly accepted the result.
