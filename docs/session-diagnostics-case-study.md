# Session diagnostics: a real-run investigation

This case examines a local OCR self-review performed while preparing this
contribution. It demonstrates the terminal investigation path, not a production
defect or a measured reduction in human debugging time.

This case evaluates explicit request/response/tool links and read-only session,
task, request and original-event inspection. The PR also includes
[comment evidence](comment-evidence.md), which was not exercised by these runs.
These results do not establish the need for that larger persistence model.

## The investigation

Session `34251e0c-3542-442f-a8fb-9439091fdcf9` ran on November 4, 2026. It finished
`partial` after 3m23s: 10 selected files, 2 completed and 8 failed. The question was
which LLM request failed during execution, how that related to the final file
classifications, and where its original evidence could be checked.

The original JSONL has 39 records and 505860 bytes. Its SHA-256 is
`139784358d8603a52b95929e37a4e2e18f92604707bce79f9573e37a2113a6ef`.
Historical file paths reflect the original, larger contribution scope.

## Existing tools

`ocr session show <session-id>` already reports `partial`, the coverage counts and
one LLM error. The file-level checkpoint for `internal/session/history.go` says:

```text
main_task did not complete before stopping (round 1/1)
```

The saved review report already explains the budget limit. Its `retry_report`
identifies task group `internal/session/history.go,internal/session/persist.go`,
phase `main_task` and request number 4. The existing Viewer also displays the
specific error in that task's Request #4 card:

```text
caller deadline exceeded: error reading response body: context deadline exceeded
```

Searching the raw JSONL for its `llm_error` record finds the same information at
line 36. A query using the existing `filePath`, `taskType` and `request_no` fields
finds the corresponding request at line 35. These tools are sufficient to reach
the conclusion; this example does not expose an ambiguous positional association.

## CLI investigation path

These commands were run against the saved local session. They require that
session to be present; its full prompts and source payloads are not bundled here.

```bash
ocr session diagnose 34251e0c-3542-442f-a8fb-9439091fdcf9
```

Relevant output excerpts:

```text
Recorded outcome: partial
Coverage: selected=10 completed=2 reused=0 failed=8 waived=0
  llm_request_failed: line=36 task="internal/session/history.go,internal/session/persist.go" phase="main_task" request_id="2f45350f-ee1c-4d7a-a86f-5e73030784ae" "caller deadline exceeded: error reading response body: context deadline exceeded"
```

```bash
ocr session diagnose 34251e0c-3542-442f-a8fb-9439091fdcf9 \
  --request 2f45350f-ee1c-4d7a-a86f-5e73030784ae
ocr session diagnose 34251e0c-3542-442f-a8fb-9439091fdcf9 --event 36
```

The request query lists the request at line 35 and its error at line 36; the event
query returns the original error record, including `request_no: 4`,
`taskType: main_task` and `duration_ms: 11024`. A separate query using only the old
fields returned identical task, phase, request number, error and line values.

The final file classification remains `budget`. The request's deadline error is
an execution observation, not a replacement classification or proof of a server
root cause. The diagnostic `request_id` is the JSONL request UUID, distinct from
`retry_report.logical_request_id` and an HTTP attempt's request ID.

The convenience is a built-in request selector, source-line index and raw-record
entry point. Users can avoid writing that composite-field query or moving from a
Viewer card to a separate log search. Familiar users may already accomplish the
same work with short raw-log queries.

## Exploratory AI investigation timings

On November 6, 2026, two Codex sub-agents with no inherited conversation history
investigated two real saved sessions. They were given the question and original
logs, review reports and existing Viewer exports, with no answer hints, and were
instructed not to inspect host answer files or each other's results. This was
instruction-based separation on a shared host, not a sandboxed human study.

Each investigator saw case A before case B. Investigator one used existing tools
for A and the new CLI for B; investigator two used the opposite conditions.
Existing tools and raw-log parsing were allowed in both conditions. The new
condition additionally allowed `session diagnose`. The executable was a frozen
local build before scope reduction; no comment-evidence feature was used.

Case B is session `946fe336-5ca9-4450-a62d-312bf81232be`: two failed `plan_task`
requests, with error records at lines 6 and 9, and final file failures classified
as `timeout`. Both cases ended `partial`.

The timer started before reading a case's materials and stopped after writing
the answer. It includes agent reasoning, tool execution, verification and answer
serialization. All answers were checked against the original records and final
manifest; source and submitted-answer hashes were retained.

| Case | Existing tools, seconds | New CLI allowed, seconds | Correct answers |
|---|---:|---:|---|
| A | 60.292 | 104.155 | Both correct |
| B | 56.539 | 74.739 | Both correct |

No elapsed-time reduction was observed. Both investigators in the new condition
also parsed raw logs to independently verify their answers. There is only one
observation per case/condition; shared-host scheduling, agent behavior, order and
individual differences prevent a causal performance claim. No human participants
or human investigation times were measured. These observations do not establish
either a general speedup or a general slowdown, nor do they justify the
comment-evidence persistence model now included in the PR.
