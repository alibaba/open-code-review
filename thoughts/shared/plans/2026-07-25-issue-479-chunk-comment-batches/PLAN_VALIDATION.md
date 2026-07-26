<!-- SIGNPOST | 3/5: PLAN_VALIDATION | adversarial review; default FAIL, cite evidence | Prev: PLAN.md | Next: implement+review -->
# PLAN_VALIDATION — Issue #479 (bounded comment batches)

Reviewer stance: adversarial. Every checklist item defaulted to FAIL; PASS only where the plan cites a mechanism that is verified correct against the real code (`scripts/github-actions/post-review-comments.js`, `post-review-comments.test.js`, `action.yml`). Citations are `file:line` into the actual code, not the plan's self-citations.

Code read in full: `post-review-comments.js` (1204 lines), `post-review-comments.test.js` (1338 lines), `action.yml` (306 lines). Other consumers searched (`.github/workflows/*`, `examples/**`): none read script outputs.

## Verdict summary

VERDICT: **MINOR-FAIL** — the plan is structurally sound and its core correctness claim (global fence-ID dedup making per-batch reconciliation work) is correct against the code. Two localized issues were found and fixed in PLAN.md directly (full context loaded, no structural rethink needed). See "Changes I made to PLAN.md" at the end.

| # | Item | Result | Evidence |
|---|------|--------|----------|
| 1 | B1–B7 each have a correct named mechanism | PASS | see §1 |
| 2 | Partial-success path for a MIDDLE batch reconciles correctly | PASS (one wording fix applied) | see §2 |
| 3 | ALL callers/consumers of changed interfaces enumerated + back-compat | PASS | see §3 |
| 4 | Moving try/catch into `publishBatch` loses no outer-scope state | PASS (one missing-state callout applied) | see §4 |
| 5 | New helpers justified vs existing ones | PASS | see §5 |
| 6 | No TBDs in PLAN.md; spec is requirements-only | PASS | see §6 |
| 7 | Phase 3 tests exercise B3/B4/B6 | PASS | see §7 |
| 8 | (small scale — no resource math) | N/A | — |

---

## 1. Spec coverage (B1–B7) — PASS

Each invariant verified against the real code, not the plan's assertion.

- **B1 (bounded size)** — `resolveBatchSize` + `chunkArray(sorted, batchSize)` partition via `slice(i, i+N)` (PLAN §Approach #2, §Phase 1). The bug site is real: `post-review-comments.js:202-210` posts ALL of `toSend` in one `createReview`. Default N=50 is a sensible floor on the 71-comment failure (IMPLICIT_SPEC A2). Mechanism is correct.
- **B2 (deterministic order)** — comparator on `path → start_line → end_line → original index`, applied **before** partitioning (PLAN §Approach #1). Verified the comment shape carries these fields (`:127-132` has `path/start_line/end_line/...`); no severity/category fields exist, so A1's deviation is forced, not a shortcut. Original-index tiebreak guarantees AS4. Correct.
- **B3 (cross-batch idempotency)** — reuse `findExistingBatchReview` (`:937`) + `getPostedCommentIds` (`:953`) + fence `newCommentId` (`:1007`) per batch; `REVIEW_TAG` body on every batch call. **The load-bearing fact:** `getPostedCommentIds` walks ALL review comments on the PR (`:953-973`, comment header "across all reviews") and returns a `Set` of every fence ID posted server-wide. Each comment's `id` is a globally-unique random token (`:133`, `:1007-1009`). Therefore filtering a chunk's items against this global set is correct regardless of which batch posted them. Correct.
- **B4 (non-destructive)** — counts accumulated across batches; `successCount + failedCount == toSend.length` (PLAN §Desired End State). Verified the existing accounting holds: today `successCount = toSend.length - toRetry.length` (`:278`) and the per-comment loop only ever increments `successCount` or `failedCount`/`failedComments` mutually exclusively (`:301`, `:353`, `:364`, `:374`, `:397`). Per-batch accumulation preserves the invariant. Correct.
- **B5 (sequential)** — `for...of` with `await publishBatch(...)` (PLAN §Phase 1). Single-threaded JS, no `Promise.all`. Correct.
- **B6 (reconcile-unavailable stops visibly)** — the `alreadyPosted === null` branch (`:359-370`) increments `failedCount` and pushes to `failedComments` instead of retrying. PLAN says this path is "preserved inside helper" (§Invariant→mechanism). Correct.
- **B7 (telemetry)** — new `batches_total/attempted/succeeded/reconciled` + `batch_summary` JSON via `setStatsOutputs` extension (PLAN §Phase 2). Existing 5 outputs at `:462-466` unchanged. Correct.

All 6 acceptance scenarios (AS1–AS6) are covered by named tests in Phase 3 (PLAN §Phase 3 lists `testBatchPartitioningDeterministic` AS1-4, `testBatchPartialSuccessReconcilesPerBatch` AS5, AS6 = "existing tests remain valid"). Correct.

## 2. Partial-success path for a MIDDLE batch (batch #2 of 4) — PASS (one wording fix)

Trace requested: batch #2 throws 5xx but landed. Does `publishBatch` reconcile only batch #2's comments without disturbing batch #1?

The reviewer flagged the plan's wording: PLAN §Approach (line 37) says "the reconciliation already operates on the *whole* `toSend` via `getPostedCommentIds` ... when we filter its result to the current chunk." This is imprecise — you do NOT filter the result set to the chunk; you filter the **chunk's items** against the global result set. The two are functionally distinct if misread literally ("filter the result to the chunk" could imply dropping IDs not in the chunk, which would be wrong). Corrected in PLAN.md.

Substance, verified correct:
- `publishBatch` operates on `chunk` only (PLAN §Phase 1: "extract the body ... so it operates on `chunk` instead of all of `toSend`"). The batch's own `createReview` sends only `chunk`'s reviewComments.
- On 5xx, `toRetry` is computed by filtering `chunk` (not `toSend`) against `postedIds` (PLAN §Phase 1: "their `toRetry` filter is applied to `chunk`"). Because batch #1 already returned success (the loop is sequential, B5), batch #1's comments are not re-processed at all — there is nothing to "disturb."
- Because `postedIds` is server-global and batch #1's comments are already on the server (or were retried into success), any fence ID already in `postedIds` is excluded from batch #2's `toRetry`. No double-post of batch #1 comments. Correct.
- Batch #2's per-comment fallback loop runs over batch #2's `toRetry` only (PLAN §Phase 1: "The per-comment fallback loop runs over `chunk`'s retry subset"). Correct.

The catch-block's `existingReview` lookup (`findExistingBatchReview`, `:937`) searches by `REVIEW_TAG`, which is the SAME for every batch. ⚠ **Implementation hazard flagged (not a plan defect):** when batch #2 lands-ambiguously, `findExistingBatchReview` may match batch #1's successfully-created review (same tag), causing `existingReview.found` to be true and `getPostedCommentIds` to be consulted. That is actually fine for correctness (the filter then excludes already-posted IDs), but the log message "Batch review already exists (review_id=...)" (`:280`) becomes misleading under multi-batch. This is an implementation-detail/log-wording concern, not a correctness or plan defect; noted here so the implementer handles it. No plan change needed.

## 3. All callers/consumers of changed interfaces — PASS

Searched `scripts/`, `action.yml`, `.github/workflows/*`, `examples/**` for `runPostReviewComments`, `require.*post-review-comments`, `setStatsOutputs`, `comments_total`/`comments_inline`/`comments_failed`/`comments_skipped`/`summary_comment_url`, and `uses: alibaba/open-code-review`.

**Changed interface 1: `runPostReviewComments` signature (new param `reviewCommentBatchSize`).**
- `action.yml:295-305` — the inline wrapper. Only call site in production code. PLAN §Blast radius addresses it: default → back-compat (existing `uses:` invocations without the input get N=50). Verified the wrapper passes named params (`:295-305`), so a new key with a default is non-breaking. Correct.
- `post-review-comments.test.js:268` — the `run()` test helper. PLAN §Phase 3 explicitly says the mock/test wiring is extended; the `run` helper spreads `...options` (`:275`), so passing `reviewCommentBatchSize` through `opts` works without signature change. Correct.

No other production caller exists (grep returned only the two above). The example workflow `examples/github_actions/ocr-review.yml:114` and in-repo `.github/workflows/ocr-review.yml:54` consume the **action** via `uses:`, not the function — they get the new input by safe default.

**Changed interface 2: `setStatsOutputs` signature.**
- Single caller: `:458` (`setStatsOutputs(out, stats)`). PLAN §Phase 2 says "extend its signature or pass a single `stats` object that now includes batch fields" — both options keep the existing call valid if `stats` gains optional fields. No external consumer; the function is local (`:461`). Correct.

**Changed interface 3: `makeGithub` mock.**
- Single caller family: the test file's own tests via `run()` → `makeGithub(githubOpts)` (`:265`). PLAN §Phase 3 extends the mock in place. No external consumer. Correct.

**Outputs consumers (B7 back-compat):** grep found NO consumer of `comments_*`/`summary_comment_url` outputs in any `.yml`/`.yaml` in `.github/` or `examples/` (they use the action but don't read its outputs by name). PLAN §Blast radius's claim that existing 5 outputs stay unchanged and "existing consumers unaffected" is vacuously-true-but-safe; even if a downstream consumer existed, adding NEW outputs is non-breaking. Correct.

## 4. Does moving try/catch into `publishBatch` lose outer-scope state? — PASS (one callout added)

The current catch block mutates outer-scope `successCount`, `failedCount`, `failedComments` (declared at `:190-192`). PLAN §Phase 1 has `publishBatch` RETURN `{ succeeded, failed, failedComments, reconciled, ambiguous }` and the loop accumulate them (`successCount += r.succeeded; ...`). That captures the state correctly.

However, the current block reads several config values from `process.env` INSIDE the catch (`:222-229`: `MAX_RETRIES`, `SUCCESS_DELAY`, `FAILURE_DELAY`, `LOW_REMAINING_THRESHOLD`, `LOW_REMAINING_SPACING`, `READ_SUCCESS_DELAY`, `READ_LOW_REMAINING_SPACING`). PLAN's `publishBatch({ chunk, github, ..., ...config })` destructures a `config` bag — but the plan does not explicitly state these 7 env-derived values are passed in or re-read inside the helper. If the implementer naively scopes `config` to only the new `batchSize`, the per-comment fallback inside `publishBatch` loses its pacing knobs. Added an explicit note to PLAN.md §Phase 1 that `publishBatch` must re-read (or receive) all 7 existing pacing env vars — behavior-preserving move, not a redesign.

No other outer-scope state is at risk: rate-limit cooldown state is local to each invocation (`computeRetryDelayMs` is pure, `:751`; `sleep` is stateless, `:724`), and there is no cross-batch mutable cooldown accumulator in the current code. So per-batch cooldown is correct (each batch cools down per its own error — which is exactly B5's intent).

## 5. New helpers justified vs existing ones — PASS

Verified by grep that the file contains NO existing `slice`/`chunk`/`partition`/`sortToSend`/`resolveBatchSize`/`batchSize` (only `Math.ceil` at `:789`,`:810` for delay formatting, unrelated). So:
- `chunkArray` — genuinely new; no existing array-partitioner. Justified.
- `sortToSendDeterministically` — genuinely new; the only sorts in the file are inside `listExistingReviewComments` (HTTP `sort=created` query param, `:624`), unrelated. Justified.
- `resolveBatchSize` — mirrors the existing `parseNonNegInt` discipline (`:733-736`) but for a positive int (N≥1, not ≥0). PLAN explicitly justifies the divergence ("batch size is a positive int, not non-negative", A2). Justified — `parseNonNegInt` would accept N=0, violating B1.
- `publishBatch` — the extraction itself; this is the core refactor, not a gratuitous helper. Justified by §Approach.

No unjustified duplication.

## 6. No TBDs / spec is requirements-only — PASS

PLAN.md: scanned for "TBD", "TODO", "FIXME", "open question", "??", "TBC" — none present. The `A1–A6` items are labeled "Bounding assumptions (analyst-set; user did not confirm — flagged for review)" which is appropriate analyst-disclosure, not an open question blocking the plan.

IMPLICIT_SPEC.md: scanned — it contains requirements (B1–B7 invariants, AS1–AS6 scenarios) plus a "Bounding assumptions" section (A1–A6). The A-section is borderline (it states analyst decisions like default N=50 and the sort key) but is explicitly framed as assumptions for review, not as prescribed mechanisms. The spec's SIGNPOST line 1 says "requirements only, no designs" and the body largely honors this. The one place that leans toward mechanism is A3 (telemetry counter names) and A1 (exact sort key) — but both are presented as the analyst's bounded choice with rationale, and the validator is invited to challenge them. Acceptable as requirements-with-stated-assumptions; not a defect.

## 7. Do Phase 3 tests actually exercise B3/B4/B6? — PASS

Cross-referencing the named tests in PLAN §Phase 3 against invariants:
- **B3 (cross-batch idempotency)** — `testBatchPartialSuccessReconcilesPerBatch` (AS5): "2 batches, batch #2 throws 5xx but landed → only its missing comments retried; batch #1 comments untouched (no double-post)". This directly exercises cross-batch idempotency. The success criterion "new tests fail (red) when the implementation is reverted" (PLAN §Phase 3) guards against the test being a no-op. Correct.
- **B4 (exhaustive counts)** — `testBatchCountsExhaustive` (B4): "71 comments, one batch partially fails irrecoverably → `comments_inline + comments_failed == 71`; `batches_total == 4`". Directly asserts the exhaustive partition invariant. Correct.
- **B6 (reconcile-unavailable)** — PLAN §Testing Strategy lists "reconciliation-unavailable per batch (B6)" as an edge covered. ⚠ Minor: this edge is named in the Testing Strategy but is NOT in the enumerated Phase 3 test list (which has 7 tests). The existing test `testPerComment5xxIdempotencyUnavailableSkipsRetry` (`:643-667`) covers the per-comment case for a SINGLE batch today, but with the new batching it runs under default N=50 → still one batch, so it does NOT actually prove B6 holds per-batch. To genuinely exercise B6 across batches you need a multi-batch test where the idempotency read is unavailable mid-sequence. This is a test-coverage gap, not a plan-correctness defect. Added a note to PLAN.md §Phase 3 to add an explicit multi-batch B6 test.

Also verified: the `testBatchPartialSuccessReconcilesPerBatch` test REQUIRES the extended mock's per-batch error spec (PLAN §Phase 3 "allow a per-batch-index error spec ... so a test can fail batch #2 but not batch #1") — and the current mock dispatches strictly by `callIdx === 0` (test `:147-156`), which cannot express "batch #2 fails but #1 succeeds". So the mock extension is necessary, not optional. PLAN correctly identifies this (A4). Correct.

## Sanity-check of the three flagged risk areas

**(a) `getPostedCommentIds` filtering to chunk membership.** Verified the real shape: `getPostedCommentIds` (`:953-973`) returns a `Set` of ALL fence IDs across ALL reviews on the PR (global). The plan correctly does NOT try to filter this set to "the current chunk" at the source — instead each batch's `toRetry = chunk.filter(item => !postedIds.has(item.id))`. Because IDs are globally unique random tokens (`:1007`), this is sound. The one imprecision was wording (filter chunk-against-set, not set-to-chunk) — fixed in PLAN.md §Approach.

**(b) Mock discriminator soundness.** Verified the two call shapes:
- Batch call: `body: reviewBody` (= `REVIEW_TAG`, `:199`/`:207`), `comments: toSend.map(...)` (`:209`, length > 1 when `toSend.length > 1`).
- Per-comment fallback: `body: ""` (`:297`), `comments: [reviewComment]` (`:299`, length exactly 1).

PLAN §Phase 3 proposes `params.body === REVIEW_TAG` OR `params.comments.length > 1` as the batch discriminator. Both individually are sound:
- `body === REVIEW_TAG`: per-comment calls have `body === ""`, never `REVIEW_TAG`. Sound. (Caveat: `REVIEW_TAG` is module-internal; the test would need it exported or reconstructed. PLAN does not mention exporting it — minor implementation note, the test already imports internals freely via the require at test `:15`, and `REVIEW_TAG` is derivable from context.runId/runAttempt which the test hardcodes. Not a plan defect.)
- `comments.length > 1`: per-comment calls always have exactly 1. Sound for all current call paths. The ONLY way this breaks is a future batch of size N=1 (single-comment batch) — which `testBatchSizeOnePerComment` (PLAN §Phase 3) explicitly adds. Under N=1, a batch call has `comments.length === 1`, colliding with per-comment. So `comments.length > 1` ALONE is unsound once N=1 tests exist; the discriminator MUST be `body === REVIEW_TAG` (or an explicit marker), with `length > 1` only as a secondary signal. PLAN lists them as alternatives ("or") which is loose. Tightened in PLAN.md to require `body === REVIEW_TAG` as primary. This is a real soundness fix.

**(c) Largest existing test input vs default N=50.** Verified by counting comment-array sizes: the largest existing inputs are **5 comments** (`testIncrementalMultiLineIoUDefaultThreshold` `:526`, `testBatchLandedWithPerCommentMixedStates` `:989`). All others are 1–3. With default N=50, every existing test produces exactly ONE batch → `callIdx === 0` mock semantics still hold → existing `createReviewCalls.length` assertions (`:315`, `:437`, `:492`, `:541`, `:568`, `:607`, `:635`, `:689`, `:893`, `:939`, `:978`, `:1080`, `:1127`, `:1166`) remain valid. PLAN's claim ("default N=50 ≥ all test inputs of 3-5 comments") is accurate. Correct.

---

## VERDICT: MINOR-FAIL

The plan is correct on every load-bearing point (the global fence-ID dedup insight is the crux and it is right; back-compat is sound; the mock extension is necessary and the existing tests don't break). Three localized issues were found and fixed in PLAN.md:

1. **Wording imprecision** on the reconciliation filter direction (§Approach / §2) — could be misread as "filter the posted-ID set down to the chunk", which is wrong. Fixed.
2. **Mock discriminator unsoundness** under the N=1 test (§Phase 3 / sanity-check b) — `comments.length > 1` breaks when the plan's own `testBatchSizeOnePerComment` test runs. Promoted `body === REVIEW_TAG` to the required discriminator. Fixed.
3. **Missing B6 multi-batch test** + **unmentioned pacing-env passing into `publishBatch`** (§4, §7) — added an explicit test and an explicit state-passing note so the implementer does not drop the 7 env-derived pacing knobs or skip the cross-batch reconcile-unavailable scenario.

No structural or correctness rethink needed; full context was loaded and the fixes are within the plan's own scope.

## Changes I made to PLAN.md

1. **§Approach (the paragraph at line ~37)** — clarified the reconciliation filter wording from "filter its result to the current chunk" to "filter the chunk's items against the global posted-ID set" (the set is server-global and never chunk-scoped). Added a one-line note that `REVIEW_TAG` is shared across batches, so `findExistingBatchReview` may match an earlier batch's review — harmless for correctness but log messages should not assume single-batch.

2. **§Phase 1 (`publishBatch` bullet)** — added an explicit requirement that `publishBatch` must re-read (or receive via `config`) the existing pacing env knobs (`MAX_RETRIES`, `SUCCESS_DELAY`, `FAILURE_DELAY`, `LOW_REMAINING_THRESHOLD`, `LOW_REMAINING_SPACING`, `READ_SUCCESS_DELAY`, `READ_LOW_REMAINING_SPACING`) so the move does not silently drop per-comment pacing. Framed as behavior-preserving, not a redesign.

3. **§Phase 3 (mock discriminator bullet)** — promoted `params.body === REVIEW_TAG` to the REQUIRED primary discriminator (per-comment calls have `body === ""`, verified `:297`); demoted `comments.length > 1` to a secondary signal only, with an explicit note that it is unsound under the plan's own N=1 test (`testBatchSizeOnePerComment`), where a single-comment batch collides with per-comment shape.

4. **§Phase 3 (test list)** — added `testBatchReconcileUnavailableStopsVisibly` (B6, multi-batch): a ≥2-batch run where the idempotency read API is unavailable mid-sequence; assert the affected batch's comments are recorded as `failed` (not reposted) and earlier/later batches are undisturbed. This closes the gap where the existing single-batch `testPerComment5xxIdempotencyUnavailableSkipsRetry` does not exercise B6 across batch boundaries.
