---
name: code-review
description: "Review the range from the fixed /base baseline to the current HEAD with synchronous OCR MCP."
---

Review only a Git range. The fixed point comes from `.scratch/base`; do not ask
for a moving `HEAD~N` ref and do not use workspace or single-commit review for
this skill.

## Preconditions

1. Require `.scratch/base` to exist and be non-empty. Do not create it or call
   `/base` automatically. `/base` owns manifest creation; the MCP adapter owns
   strict format validation.
2. Read the fixed `base_sha`, `source`, and source value needed for this review.
3. Read the complete task source once at the start of the review. External
   `source/ref` values are read through the host integration; `source: user`
   uses `summary`. Do not use a cache or substitute a summary when an external
   source fails. Keep the payload in memory only; do not write it to repository
   state, commits, or extra logs.
4. If the task source cannot be read, is not serialisable, or exceeds the
   available context, stop and return `needs_human` without calling OCR.

## Range review

1. Resolve `from` from `.scratch/base:base_sha` and resolve `to` to the current
   `HEAD` unless the caller explicitly supplies a commit ref for the current
   head. Convert both to full commit SHAs before calling MCP.
2. Require `from` to be an ancestor of `to`. A bad range stops the review.
3. Exclude `.scratch/**` from the review input automatically.
4. If the range is empty after that exclusion, return OCR's native empty-review
   JSON and do not call OCR.
5. Call `ocr_review` exactly once with the range and the raw task source in the
   deterministic `background` wrapper below. Keep the call blocking until it
   returns a terminal result.

```text
<task-source>
source: github
ref: https://github.com/org/repo/issues/123

content:
<complete provider payload>
</task-source>
```

Do not spawn review sub-agents, run the OCR CLI, inspect progress events, or
poll for completion. If the host transport is interrupted, call
`ocr_review_wait` once. If the MCP process ended, resume the same persisted
range session once with the same target. An unavailable session returns
`needs_human`.

## Result contract

Return OCR's native JSON, including its existing top-level `status`, `summary`,
`comments`, `warnings`, coverage, and session fields. Do not add a second report
format, `review_handoff`, or `recommended_disposition`.

For a completed range result, the forked MCP adapter adds `finding_id`,
`consecutive_review_count`, and `automation_status` to each comment and updates
`.scratch/finding-counts.json`. A finding appearing in three consecutive
completed reviews is returned as `deferred_for_human`; `implement` skips it and
continues with other findings. Partial, failed, cancelled, or incomplete OCR
results do not update the counter.
