# Interrupted review: generated comments without file checkpoints

This is a retrospective investigation of an actual local OCR self-review of this
contribution, using an instrumented development build and real model responses.
It is not an external production incident, an injected failure or a scripted
model test. The cause of the interruption remains unknown.

## Question and original records

Session `9e3d5483-89ed-4a73-a1ae-fe8498e279db` started on November 6, 2026 at
13:34:37 UTC. Its saved JSONL has 61 records and 1,060,598 bytes, with SHA-256
`da8be5a27a40c4de509cebf85c2ac993e73a9821f81e082fa23b23729647fbd7`.
It contains five completed file checkpoints, each with no comments, and no
`session_end`. There is no checkpoint for `internal/session/diagnose_comments.go`.

The existing session summary reports zero checkpoint comments and `aborted`.
An initial inspection of that count missed the generated candidates. The
investigation question is: **did the candidates remain pending after async
dispatch, or did processing reach location resolution before the interruption?**

Commands below were executed in the repository worktree that created the session.
They require the saved session locally. A [selected-field evidence bundle](session-diagnostics-interrupted-comments-evidence.json)
provides the original line references, tool arguments, dispatch results, captured
records and the reviewed file's diff fragment. It is not a complete replayable
session; complete prompts and native provider payloads are excluded.

## What existing tools establish

```sh
ocr session show 9e3d5483-89ed-4a73-a1ae-fe8498e279db --json
ocr session comments 9e3d5483-89ed-4a73-a1ae-fe8498e279db --json
```

Relevant summary fields are `total_comments: 0`, `completed_files: 5`,
`aborted: true`; the comment command returns `[]`. These counts correctly describe
persisted checkpoints. They do not imply a completed, clean review.

The existing Viewer export has zero final comment cards and two `code_comment`
tool entries. Its conversations already expose both generated candidates:

| Original JSONL line | Record | Existing information |
| --- | --- | --- |
| 19 | request #3 | Original request and reviewed file content |
| 30 | response #3 | One `code_comment` candidate and its original snippet |
| 31 | tool result | `ok: true`, `result: "(async)"` |
| 33 | request #4 | Next request in the same task |
| 37 | response #4 | A second `code_comment` candidate |
| 38 | tool result | `ok: true`, `result: "(async)"` |

Searching those response/tool records is enough to establish generation and the
async dispatch path. Their result does not record whether the background worker
reached location resolution. No later comment checkpoint supplies that boundary.
The older records therefore leave pending processing and resolved-but-not-
checkpointed processing indistinguishable. Calculating where the resolver would
match a snippet does not establish that the worker actually reached that point.

The Viewer implementation is unchanged by this PR; its source matches the
contribution base. This comparison uses its real export of the same session.
Existing-tool inspection leaves the additional comment-evidence records unused;
it is not a second execution with an unmodified producer or a timing experiment.
The comparison is limited to the saved JSONL and Viewer export; it does not claim
that separately retained telemetry traces could never supply a completion signal.

## What the captured records establish

This historical run used the larger experimental producer. Its records retain
the old `comment_evidence` type; they were not rewritten into the reduced format.
The current CLI can open them as original records by physical line:

```sh
ocr session diagnose 9e3d5483-89ed-4a73-a1ae-fe8498e279db --event 32
ocr session diagnose 9e3d5483-89ed-4a73-a1ae-fe8498e279db --event 39
ocr session diagnose 9e3d5483-89ed-4a73-a1ae-fe8498e279db \
  --request 01143741-2148-4eed-89e8-4f84a1c6f6ff --json
ocr session diagnose 9e3d5483-89ed-4a73-a1ae-fe8498e279db --event 31
```

Their captured positions are:

| Original line | Path | Original range | Captured range |
| --- | --- | --- | --- |
| 32 | `internal/session/diagnose_comments.go` | 0:0 | 210:211 |
| 39 | `internal/session/diagnose_comments.go` | 0:0 | 222:224 |

The first candidate is linked to response
`83783e2d-4167-41ff-82e1-af805fc43850` and tool call
`call_d01f0f4759014bb3bb7c55754aba60cd`. Its original coordinates are `0:0`,
and its captured coordinates are `210:211`. The second candidate changes from
`0:0` to `222:224`. The stored span hashes match the captured excerpts.

The [archived producer](https://github.com/biheto/open-code-review/blob/60c0893ea9610bcaefaf15ed5ec4951c57400f73/internal/llmloop/loop.go#L832)
wrote these records after location processing and
immediately before `CommentCollector.Add` in `resolveAndCollect`. The saved records
establish that both jobs reached this capture boundary after resolving their
locations. They do not establish that `Add` returned, that a file checkpoint was
written, that either candidate is a valid bug, or why the process ended. One
candidate's own prose even says that no defect was confirmed; neither should be
treated as an accepted finding.

The concrete gain is a more definite processing-stage conclusion without
assuming that an async dispatch result means worker completion.
No human investigation time or percentage speedup was measured.
The additional fact narrows uncertainty about worker progress; it does not
explain the interruption or demonstrate a large reduction in overall debugging
effort. The question and endpoints were examined retrospectively, not in a
blinded productivity experiment.

## Reduced implementation

The merge scope now uses one `comment_location` event at the same boundary. It
stores only `comment_index`, `comment_path`, `start_line`, `end_line`, and the
existing event/task/request/response/tool metadata. There is no global comment ID,
comment-body copy, code snapshot/hash or checkpoint/lifecycle model. Filtering,
deduplication, resume and `model.LlmComment` are unchanged.

For new recordings, the overview includes `comment_locations` counts; `--task`
and `--request` list stage events and their locations, and `--event` opens the
original record. The original response supplies the candidate body through the
tool-call ID and parsed batch index. A zero range records returned-but-unlocated
processing, not failure or acceptance. The removed experimental `--comment`
selector is not part of the reduced CLI; no compatibility decoder for that larger
experimental schema is retained.
The index counts the comments accepted by the tool parser, not rejected entries
in the raw JSON array. Buffered session writes retain their existing behavior;
missing stage records do not prove processing never happened.

The writer and diagnostic integration are tested with synchronous and
asynchronous processing, cross-file resolution, LLM relocation and unlocated
comments. Those scripted tests establish correctness, separately from the actual
historical case above. No new real run or measured speedup is claimed by changing
the record format.

## Which persistence is supported by this case

A field-deletion check of the actual captured records gives a narrower result:

| Needed fact | Evidence necessary in this case |
| --- | --- |
| Candidate generation and original text | Existing response/tool arguments already suffice |
| Worker reached the post-resolution capture point | A signal absent from the saved older JSONL types; a small completion/stage record would suffice |
| Actual resolved coordinates and origin | Stage record with path/range and request/response/tool/index references |
| Historical reviewed source bytes | Existing request #3 already includes the entire added-file diff |
| Filtering, dedup and cross-session provenance | None occurred in this session; this case supplies no justification for their persistence |

The complete 254-line file was independently reconstructed from the added lines
in the existing request. Its SHA-256 is
`75ae0e94ea0a4368aceb94ff755350b5a4d73a506d6c33d56190fda4c84e52b8`, matching the
new snapshot digest. Both captured spans can also be reconstructed from those
bytes. Code excerpts and hashes make lookup convenient here, but their additional
persistence is not needed to recover that source. The field-deletion check is an
analysis of actual data, not an implemented smaller format or another real run.

The two complete evidence events add 5,833 bytes in this session. That measures
stored bytes only, not runtime or long-term maintenance cost. A smaller
post-resolution event still answers the stage question without retaining both
comment copies, code excerpts/hashes or the broader lifecycle model.

This case supports persistent async-stage evidence plus origin/location links.
The larger comment-persistence model has been removed from this PR because this
case does **not** establish that its compatibility, lifecycle and maintenance
obligations are justified. Separate real cases are still needed for those
additional mechanisms. The existing request-failure
[case study](session-diagnostics-case-study.md) likewise does not justify them.

As a completed-session counterexample, the actual earlier self-review
`bd653555-183a-43cf-9da4-a1724a1d353c` has one candidate in response line 26,
its request at line 25, and a matching final checkpoint comment at line 39.
There are no comment-evidence events. The existing response arguments and
checkpoint body uniquely match that candidate; no new comment persistence is
needed to establish its origin in that run. Its source JSONL SHA-256 is
`7187175c56de0513c388f098367c762ef7b9d48dd5fa923ec6b562be47802b5a`.
