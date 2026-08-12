---
status: accepted
---

# Base-gated implementation and range-review loop

The implementation and review skills share one explicit baseline and one task
source. `/base` writes the immutable baseline manifest to `.scratch/base`.
`implement` and `code-review` refuse to continue when that manifest is absent or
invalid. The baseline remains unchanged until `/base reset` is used.

## Decision

`.scratch/base` records a full `base_sha`, a lowercase task-source `source`, and
exactly one of an external `ref` or a user `summary`. The host agent reads the
external task once per round and passes the complete payload to both
implementation and OCR. It does not store the payload in repository state.

`code-review` provides range review only. It reviews the fixed baseline through
the current `HEAD`, resolves both endpoints to full SHAs, rejects non-ancestor
ranges, excludes `.scratch/**`, and skips OCR for an empty range.

The MCP adapter remains the owner of finding identity and repetition state. It
adds a stable `finding_id` to completed native OCR comments and maintains
`.scratch/finding-counts.json`. Three consecutive appearances mark a finding
`deferred_for_human`; the implement loop skips that finding and continues other
work. A completed review with no finding clears disappeared IDs. `/base reset`
clears all counters.

`implement` runs an unbounded convergence loop. It assumes the baseline passed
existing tests, runs focused checks during editing, runs the full suite at the
end, commits only the work it changed, and reaches `completed` only when the
final range review is clean, the full suite passes, and no finding is deferred.

## Consequences

- Review scope cannot drift as commits accumulate.
- Task requirements remain authoritative because OCR receives the same raw
  external payload as implement.
- OCR core, CLI native output, and persisted OCR session schema remain unchanged.
- Repository-local `.scratch` state is never staged or committed.
- A finding that remains unresolved for three completed reviews is surfaced for
  human handling without blocking unrelated fixes.
