<!-- SIGNPOST | 1/5: SPEC | requirements only, no designs | Next: PLAN.md
     Pipeline: SPEC -> PLAN -> PLAN_VALIDATION -> implement+review -> tests+TEST_VALIDATION -> green -->

# Implicit Spec — Issue #479 (bounded comment batches)

> Requirements, not designs. A mechanism written here becomes exempt from validation.

Source: [research doc](../../research/2026-07-25-issue-479-chunk-comment-batches.md) (scale: small, status: complete, reused wholesale — staleness check at commit `c9b1456` shows **no diff** on any cited file: `git diff --stat c9b1456..HEAD -- scripts/github-actions/post-review-comments.js scripts/github-actions/post-review-comments.test.js action.yml package.json` → empty). Plus the issue body's 9 required behaviors and acceptance scenarios ([#479](https://github.com/alibaba/open-code-review/issues/479)).

## Invariants (any change here must uphold)

- **B1 — Bounded batch size.** Inline-comment publication must send comments in `createReview` calls of ≤ N comments each, N configurable. Observable today: one run posted 71 candidates in one request → GitHub Server Error after partial success (`post-review-comments.js:202-210`). Edge: N ≥ 1; `toSend.length <= N` → exactly one batch (current behavior, no regression); empty `toSend` → zero calls (unchanged, `:194`).

- **B2 — Deterministic batch composition & order.** Identical input `toSend` must produce identical batches on every run, so a partial-success + retry reproduces the same partition and dedup works. Sort key is fixed (see Bounding assumptions A1) and applied **before** partitioning.

- **B3 — Idempotency across batch boundaries.** A partial success (5xx after some chunks landed) must not double-post. Reconciliation must dedup against already-posted comment IDs for **every** batch that may have reached the server, not only the first. Existing machinery preserved: `REVIEW_TAG` (`:199`), `findExistingBatchReview` (`:937`), `getPostedCommentIds` (`:953`), per-comment fence `newCommentId` (`:1007`). The per-comment fallback loop (`:288-300`) must still run for any batch whose retry set is non-empty; a retry must not repost IDs GitHub already accepted.

- **B4 — Non-destructive / fail-open.** A batching failure must not drop or destroy already-collected findings. Counts (`successCount`/`failedCount`/`failedComments`) must remain mutually exclusive and exhaustive across all batches, and reconcile with GitHub-observed state. (Sibling #478 is the dedicated fail-open request; this change must not regress it.)

- **B5 — Sequential batches, each reconciled before the next.** Batches submit sequentially; a batch that returns an ambiguous result (5xx/408/network) is reconciled against GitHub before the next batch proceeds. No concurrent writes (issue: "concurrent writes risk secondary rate-limit cascades").

- **B6 — Reconciliation-unavailable stops visibly.** If reconciliation (idempotency reads) is itself unavailable, do not silently risk duplicates — record the affected comments as failed (`failedCount++`) rather than optimistically reposting. Already implemented at `:364`; preserve the behavior for every batch.

- **B7 — Existing outputs remain valid; telemetry extended.** Current outputs `comments_total/inline/skipped/failed/summary_comment_url` (`:462-466`) remain. New structured output(s) expose per-batch counters so a 71-comment / batch-size-20 run yields four predictable batches and accounts for all 71 findings.

## Acceptance scenarios (from issue #479, must be testable)

- AS1: 0 comments → zero `createReview` calls (unchanged).
- AS2: 1 comment / exactly N / N+1 / "many" → deterministic partitions: ceil(k/N) batches, batch sizes as expected (last batch = k mod N, or N when k mod N == 0).
- AS3: 71 candidates, N=20 → exactly 4 batches (20,20,20,11); all 71 accounted (success+failed == 71).
- AS4: Identical re-run produces byte-identical batch composition (deterministic ordering).
- AS5: Ambiguous partial success in any one batch → reconciled without duplicates before later batches proceed.
- AS6: Existing ambiguous-write / rate-limit / line-anchoring tests remain valid (no regression).

## Bounding assumptions (analyst-set; user did not confirm — flagged for review)

> The user declined the over-constraint confirmation questions. The following are evidence-grounded analyst decisions, **not** user-confirmed. They are recorded here so the validator and implementer can challenge them.

- **A1 — Sort key = path → start_line → end_line → original array index.** The issue requests "severity/category/path/line/finding identity" ordering, but `comment` objects carry **no** severity/category/priority fields (verified `:127-132`: fields are `path/start_line/end_line/content/suggestion_code/existing_code` only). Severity/category ordering is physically impossible without a cross-cutting schema change spanning the Go reviewer output — explicitly out of scope (#479 is a batch-publishing issue; severity routing is the separate sibling #478). The path→line→original-index key is fully deterministic and satisfies B2/AS4. The original-index tiebreak guarantees stability for same-file same-line findings.

- **A2 — Default N = 50.** Production failed at 71 in one request; GitHub's documented soft guidance is ~50 inline comments per review. 50 keeps current behavior for typical runs (most reviews are <50) while preventing the failure mode. Configurable via a new `action.yml` input; `N < 1` is rejected (clamped to 1, matching the `parseNonNegInt` convention at `:222-229` — though batch size is a positive int, not non-negative).

- **A3 — Telemetry scope = per-batch counters + one structured JSON output, existing scalars unchanged.** The issue's full counter list (raw/duplicate/historical-overlap/ambiguous/reconciled) is only partially tracked internally today and is tangential to the batching core; adding them fully would expand scope into the reconciliation accounting. The batching-specific telemetry (`batches_total`, `batches_attempted`, `batches_succeeded`, `batches_reconciled` + a JSON summary object) gives the fleet-level signal the issue's 71→4-batch acceptance scenario needs, without a counter-expansion subproject.

- **A4 — Mock harness must be extended to express N>1 batch calls.** Today the mock dispatches strictly by `callIdx === 0` = batch (`test:147-156`); it cannot represent multi-batch failure scenarios. Extending the mock (e.g. a per-batch error spec keyed by batch index, separate from per-comment errors) is in-scope and necessary — local tests are the primary verification surface (research verdict: FULLY testable offline). Cross-chunk idempotency against the real GitHub API remains untestable offline (mock fabricates posted IDs); local tests prove batching + reconciliation **logic** only.

- **A5 — `action.yml` has no YAML test harness; the new input is validated manually.** Confirmed in research doc. The Node test suite covers the script function params, not the action.yml wiring.

- **A6 — No concurrency added.** Batches stay sequential (B5). No new in-memory cache or growth surface; batch slices are views over the existing `toSend` array.
