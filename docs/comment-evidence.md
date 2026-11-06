# Comment evidence

Comment evidence connects a collected finding to the recorded model turn, its
location processing, the reviewed code span, subsequent filtering or deduplication,
and resume checkpoints. It is diagnostic history, not a verdict on the finding.

## Inspect a finding

```sh
ocr session comments <session-id> --json
ocr session diagnose <session-id>
ocr session diagnose <session-id> --comment <evidence-id>
ocr session diagnose <session-id> --request <request-id>
ocr session diagnose <session-id> --event <physical-line>
```

New collected comments expose `evidence_id` in their JSON representation. The
overview lists IDs and metadata without comment prose, code excerpts or decision
output bodies. `--comment` deliberately outputs a JSON diagnostic report even
without `--json`, including observations that qualify the captured evidence.
`--event` outputs the original JSON record; adding `--json` includes the report.

A finding's origin request belongs to its reviewing task or file group, which may
differ from its final file path. `comment_index` is the zero-based index in the
parsed tool comment batch. Tool IDs are qualified by their explicit response ID.
`parentUuid` remains physical write order and does not establish causality.

## Records and decisions

| Record | Meaning |
| --- | --- |
| `comment_evidence`, `evidence_version=ocr.comment-evidence/v1` | Original parsed comment, comment after location processing, source request/response/tool IDs, optional re-location request ID, and captured code evidence. Its UUID becomes the comment's `evidence_id`. |
| `comment_decision`, `decision_version=ocr.comment-decision/v1` | A processor's applied input-to-output decision, with the input evidence ID, zero-based input index, stage, outcome and processing request/response IDs. |
| `review_item_done` | File checkpoint, retaining comment IDs and checkpoint comment bodies. |
| `review_item_reused` | Checkpoint reused from the explicitly named source session; preserves IDs. |
| `review_item_failed` | Invalidates replay of an earlier checkpoint with the same fingerprint. Historical evidence remains inspectable. |

Decision stages are `review_filter` and `scan_dedup`. Outcomes are `retained`,
`removed`, `rewritten`, and `merged`. A merge records the canonical output comment
and its evidence ID; the canonical diagnostic also lists `merged_from` input IDs.
The decision's input index maps to the existing prompt's `c-N` identifiers. IDs
are not added to model prompts and no additional model calls are made.

When a newly tracked input is merged into a legacy canonical comment without an
ID, the decision retains the legacy output body and reports
`comment_merge_target_untracked`. No target ID, local capture or origin is
invented. Inputs that already lack IDs remain available through the existing
request and checkpoint records.

Failed or skipped processors have no applied decision; their existing request,
error and response records remain authoritative. A filter that keeps its inputs
records retained comments, including its existing fail-open response behavior.
A dedup result that the existing implementation treats as a no-op does not
record a transformation. Missing decisions do not imply removal or acceptance.

Captured comments remain immutable. Selected comment details include the
checkpoint bodies and processing outputs, so rewrites can be compared directly.
A differing checkpoint raises `comment_snapshot_changed`; it does not silently
replace the original claim or recompute its code evidence.

`checkpoint_state` follows comment replay semantics:

- `present`: at least one current checkpoint contains this ID.
- `absent`: all recorded checkpoint references were superseded or invalidated.
- `unrecorded`: no checkpoint containing this ID was recorded.

Each checkpoint reference includes `active`. These states describe stored file
checkpoints, not final publication, reviewer acceptance or finding correctness.
Cross-file dedup can deliberately retain per-file original checkpoints even when
only one canonical comment remains in the current output.

## Reviewed code and limits

Evidence uses the in-memory `model.Diff` that was reviewed. It never reads the
working tree when diagnosing a saved session. Capture records SHA-256 of the raw
diff and available new-file content, plus the full selected span's hash. Hashes
identify bytes; they cannot reconstruct the entire file and do not establish
that the selected position is semantically correct.

Spans preserve indentation, normalize CRLF to LF and omit the ending newline.
Old-side hunk and new-side content evidence are retained separately; both may
exist for the same coordinates. `matched` means that at least one captured span
matches the snippet exactly or after whitespace normalization. It does not mean
that the position is unique, that a particular side was chosen by the resolver,
or that the comment describes the right branch. The other states are `unmatched`,
`snippet_missing` and `unavailable`.

Each excerpt is capped at 16 KiB on a UTF-8 character boundary. Invalid UTF-8 bytes
are omitted from the display excerpt; hashes and matching use the original span.
The truncation flag also reports that display bytes were omitted. Original
comments and checkpoint bodies retain their existing model-output size limits.

Repeated snippets are a useful example of the distinction between textual and
semantic location. `TestRunMainTaskCommentEvidenceDuplicateAnchor` is a scripted
fixture based on the pattern reported in
[issue #991](https://github.com/alibaba/open-code-review/issues/991). It preserves
the resolver's existing first-match behavior and verifies that the original
claim, actual location and source request remain inspectable. It is not the
reporter's original session and does not measure investigation time or fix that
issue. The candidate-selection work in
[PR #992](https://github.com/alibaba/open-code-review/pull/992) is separate.

## Resume and compatibility

Reused comments without local capture are marked `external`. Their checkpoint
references identify the source session. Inspect that session explicitly with the
same evidence ID, repeating the step if it was also resumed:

```sh
ocr session diagnose <source-session-id> --comment <same-evidence-id>
```

Diagnostics never invent a new local origin or automatically follow arbitrary
session chains. If a source session was deleted, the surviving checkpoint can
still be inspected but cannot supply the missing code evidence. Old comments
without IDs remain inspectable by task and event. Unversioned prototype evidence
is readable; unknown evidence or decision versions reject aggregate comment
selection while raw events remain accessible.

Duplicate capture IDs, inconsistent identities, invalid decision shapes,
ambiguous checkpoint bodies and broken relocation or request links are reported.
Existing `ok`, asynchronous tool dispatch, virtual `task_done`, filtering,
deduplication and final manifest semantics are preserved.

## Validation and overhead

Tests drive the actual runner, filter, scan dedup, persistence and checkpoint
replay using scripted model responses. They cover synchronous and asynchronous
collection, cross-file relocation, LLM relocation, original claims surviving code
changes, removed comments, canonical merge links, checkpoint supersession, and
multiple resume hops. These are correctness tests, not production demand or
human productivity measurements.

The model adds hashing, JSON serialization and local JSONL storage, without extra
LLM calls. To measure capture and serialization separately:

```sh
make test 'PACKAGES=./internal/diff ./internal/session -race=false -run=^none -bench=CommentEvidence -benchmem'
```

Capture benchmarks use 100-line and 10,000-line snapshots. Serialization measures
one capture plus one retained-filter decision, reports JSONL bytes per comment,
and writes to a counting sink. It excludes disk latency, code capture, existing
request/response logs and checkpoints. Results depend on comment and span size;
they are not a total-session overhead or debugging speedup claim.
