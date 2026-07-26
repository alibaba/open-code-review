# IMPLEMENTATION_VALIDATION — Issue #479

Reviewer: adversarial (fresh agent, not the implementer).
Reviewed: full diff at `/tmp/479-diff.txt` against PLAN.md + IMPLICIT_SPEC.md.
Verified files (read in full): `scripts/github-actions/post-review-comments.js`, `scripts/github-actions/post-review-comments.test.js`, `action.yml`. Tests run green: `npm run test:github-actions` -> "All post-review-comments tests passed." + "All check-translation-sync.test.js passed." `action.yml` confirmed parseable (verified structurally; `js-yaml`/`yamljs` not installed in this env).

## Part A: Implementation review

### A1. Plan conformance — **PASS (with 1 NIT-level divergence noted)**

Phase-by-phase check against PLAN.md:

- **Phase 1 helpers, all present and conformant:**
  - `resolveBatchSize(raw)` — `post-review-comments.js:574-577`. Mirrors `parseNonNegInt` discipline with lower bound 1, returns `DEFAULT_BATCH_SIZE` on invalid. `DEFAULT_BATCH_SIZE = 50` constant at `:37`. Matches plan item 5 / A2.
  - `sortToSendDeterministically(items)` — `:588-603`. Returns a NEW array (`.map(...).sort(...).map(...)`), comparator is path → start_line → end_line → origIndex. Matches plan item 1 / A1 exactly.
  - `chunkArray(items, size)` — `:609-615`. `for (i+=size) { slice(i, i+size) }` loop. Matches plan item 2 exactly.
  - `publishBatch({chunk, github, owner, repo, prNumber, commitSha, reviewBody, REVIEW_TAG, log})` — `:304-550`. Extracted catch-block body operating on `chunk`; returns `{succeeded, failed, failedComments, reconciled, ambiguous}` exactly as plan item 3 / Phase 1 specifies. The 7 pacing env vars are re-read at `:345-352` (state-passing requirement satisfied).
  - Driver loop in `runPostReviewComments` — `:214-232`. Sequential `for (const chunk of batches) { const r = await publishBatch(...); ... }`. Counter accumulation (`attempted++`, `reconciled`, `succeeded>0`) matches plan Phase 1 bullet exactly. Empty-`toSend` guard preserved at `:207`.
  - `batchCounters` initialized at `:205` with `{total, attempted, succeeded, reconciled}` — matches plan.

- **Phase 2 telemetry:** `setStatsOutputs(out, stats, batchCounters, batchSize)` — `:617-645`. Emits the existing 5 unchanged + `batches_total/attempted/succeeded/reconciled` + `batch_summary` JSON with the documented shape. Matches plan Phase 2 exactly. Caller passes counters+size at `:284`.

- **Phase 3 mock + tests:** `callIdx===0` discriminator replaced by `body === REVIEW_TAG` predicate at `post-review-comments.test.js:174-186`; per-batch error spec (`batchErrorSpec` as function or array) at `:180-186`. Matches plan Phase 3 discriminator requirement (REVIEW_TAG is primary, `length>1` NOT relied upon). All 8 plan-listed tests added (see Part B matrix).

- **action.yml:** input `review_comment_batch_size` at `:90-98` (after `incremental_overlap_threshold`), env `OCR_REVIEW_COMMENT_BATCH_SIZE` at `:287`, param `reviewCommentBatchSize: parseInt(process.env.OCR_REVIEW_COMMENT_BATCH_SIZE, 10)` at `:315`. Mirrors the `incremental*` style end-to-end. Matches plan Phase 1 action.yml items.

**Divergence #1 (NIT): `buildRunTags(runId, runAttempt)` extraction at `:559-568`.** The plan did not call for refactoring the inline `runId`/`runAttempt`/`RUN_TAG`/`REVIEW_TAG`/`SUMMARY_TAG` construction (`:63-69` originally) into a helper. HOWEVER, the plan explicitly sanctioned the test obtaining `REVIEW_TAG` by reconstructing it ("REVIEW_TAG is reconstructable from the hardcoded context.runId/runAttempt in the test, **or export it**" — Phase 3). Extracting + exporting `buildRunTags` is one of the two plan-listed options, so this is a sanctioned mechanism, not improvisation. Behavior is byte-identical (`Number.isFinite` guards preserved, same string templates). Net assessment: behavior-preserving, plan-sanctioned. Flagged only for completeness — no reversion required.

### A2. Spec invariants (B1–B7) — **PASS** (all 7 upheld with `file:line` evidence)

- **B1 — Bounded size: PASS.** `resolveBatchSize` (`:574-577`) guarantees `n >= 1` (verified exhaustively: `0, -1, -5, NaN, null, undefined, '', 'garbage', '0', '-3', 0.5, 1.5, Infinity, -Infinity, 'abc', [], {}, true, false` all yield `>= 1` or default `50`). `chunkArray` (`:609-615`) partitions via `slice(i, i+size)`. The `resolveBatchSize` >= 1 guarantee is what prevents an infinite `for (i += size)` loop — confirmed by edge test.

- **B2 — Deterministic order: PASS.** `sortToSendDeterministically(toSend)` (`:203` → `:588-603`) runs BEFORE `chunkArray` (`:204`). Comparator is pure (path `localeCompare` → numeric `start_line` → numeric `end_line` → `origIndex`). Returns a new array, does not mutate caller's `toSend`. Sort happens AFTER incremental filtering (`:158-170`), so `stats.skipped` accounting is unaffected. Verified by `testSortToSendDeterministically`.

- **B3 — Cross-batch idempotency: PASS.** (a) `REVIEW_TAG` body on every batch's `createReview` — `reviewBody` passed in at `:222` and used as `body: reviewBody` at `:327` (inside the loop, once per chunk). (b) `getPostedCommentIds` (`:1131-1151`) returns a server-global `Set` from `listReviewComments` (PR-level, cross-review). (c) Per-batch dedup filters THIS chunk against the global set: `toRetry = chunk.filter((item) => !postedIds.has(item.id))` at `:405` — filters chunk items, never the set against the chunk, so an earlier batch's posted IDs correctly exclude this batch's already-landed items. (d) Per-comment fence via `newCommentId` (`:1185-1187`) embedded in `formatComment` (`:1195-1203`). The REVIEW_TAG-shared-across-batches subtlety is correctly handled and documented at `:394-401`.

- **B4 — Non-destructive exhaustive counts: PASS.** `publishBatch` returns `{succeeded, failed, failedComments}`; the caller accumulates `successCount += r.succeeded; failedCount += r.failed` (`:226-228`) and pushes each `failedComment`. Every code path inside `publishBatch` either increments `succeeded` or `failed` for each chunk member: success path `:331`; reconciled-already-posted `:406` (sets `succeeded = chunk.length - toRetry.length`, and per-comment retry of the remainder increments `succeeded` at `:442` or `failed` at `:515/538/505`); retry-all path increments per-comment. No chunk member escapes both counters. Verified empirically by `testBatchCountsExhaustive` (`inline+failed == 71`).

- **B5 — Sequential: PASS.** `for (const chunk of batches) { ... await publishBatch(...) ... }` at `:214-232`. Single-threaded JS, `await` per batch, no `Promise.all` anywhere in the diff (verified: `grep Promise.all` in the changed regions returns nothing). Each batch fully reconciles before the next iterates.

- **B6 — Reconcile-unavailable stops visibly: PASS.** When the read API is unavailable: `isCommentAlreadyPosted` (`:1169-1177`) returns `null` on caught throw, and `publishBatch:504-511` records `failed++` + `failedComments.push(...)` and `break`s the retry loop (does NOT retry, avoiding duplicates). The `findExistingBatchReview`-throws path is swallowed at `:387-389` (degraded fallback); `getPostedCommentIds` is awaited bare at `:404` (pre-existing surface — see Findings) but in the per-comment path it is wrapped by `isCommentAlreadyPosted`'s own try/catch. Verified by `testBatchReconcileUnavailableStopsVisibly`.

- **B7 — Telemetry: PASS.** `setStatsOutputs` (`:617-645`) emits existing 5 unchanged (`:618-622`) + new `batches_total/attempted/succeeded/reconciled` (`:628-631`) + `batch_summary` JSON (`:632-644`). The `if (batchCounters)` guard at `:627` means the early-return paths (`:93`, `:108`) that call `setStatsOutputs(out, stats)` without counters still work (existing 5 outputs only) — back-compat preserved. Verified by `testBatchTelemetryOutputs`.

### A3. Failure/concurrency — **PASS** (with 1 pre-existing surface noted, not a new defect)

- **`publishBatch` REVIEW_TAG shared-across-batches subtlety:** Correctly handled and documented at `:394-401`. `findExistingBatchReview` (`:1115-1125`) returns the FIRST review whose body includes `REVIEW_TAG`; under multi-batch this may be an EARLIER batch's review. This is harmless because `getPostedCommentIds` returns a GLOBAL set and the filter is `chunk.filter(item => !postedIds.has(item.id))` — the matched review's identity is irrelevant; only the fence-ID set matters. The implementer correctly did NOT assume the matched review is "this batch's". Log messages are worded accordingly ("A batch review already exists on the server" at `:408-412`, not "this batch's review").

- **`ambiguous` return field:** Correctly computed at `:425-427` (`batchMaybeReachedServer && !existingReview`). The three states are distinguished: `{found:false}` (read OK, no match → not ambiguous, retry-all is correct); `null` via swallowed throw (ambiguous); `batchMaybeReachedServer===false` (never reached server → not ambiguous). HOWEVER: the `ambiguous` field is currently dead — it is returned at `:549` but never consumed by the caller (`runPostReviewComments:226-231` ignores it) and never asserted by any test (`grep ambiguous test` → only comments, no assertions). This is a MINOR gap: the field exists but provides no observable value. See Findings #2.

- **Per-batch pacing env re-read:** All 7 env vars (`MAX_RETRIES, SUCCESS_DELAY, FAILURE_DELAY, LOW_REMAINING_THRESHOLD, LOW_REMAINING_SPACING, READ_SUCCESS_DELAY, READ_LOW_REMAINING_SPACING`) are re-read at the top of the catch block (`:345-352`), satisfying the plan's state-passing requirement. NOTE: `READ_SUCCESS_DELAY` and `READ_LOW_REMAINING_SPACING` are declared but never referenced inside `publishBatch` — read pacing is handled by the `readWithPacing`/`readAllPages` helpers (`:1059-1073`), not inline. This is a FAITHFUL MOVE of pre-existing dead code (the original catch block at the equivalent lines also declared them unused); not a new defect. See Findings #1.

- **Concurrency:** Single-threaded, sequential `await` per batch. No `Promise.all`, no shared mutable state across batches except the accumulated counters (additive only). Correct.

### A4. Common defects — **PASS**

- **Unhandled errors:** No new unhandled-rejection surface. `publishBatch`'s top-level `try/catch` (`:321-547`) wraps the whole batch+reconcile body; the catch returns a well-formed `{succeeded, failed, failedComments, reconciled, ambiguous}`. The bare `getPostedCommentIds` at `:404` can throw (pre-existing surface — same as original code), which would propagate out of `publishBatch` and out of `runPostReviewComments` (the function has no top-level try/catch around the publish loop). This is pre-existing behavior, not introduced by this diff, and is out of scope to fix.
- **Swallowed exceptions:** `findExistingBatchReview` throw swallowed at `:387-389` with a log line — intentional degraded fallback, documented. `isCommentAlreadyPosted` throw swallowed inside its own helper returning `null` — intentional, documented.
- **Input validation:** `resolveBatchSize` validates `N >= 1`. `parseInt` of the action.yml input is wrapped by `resolveBatchSize`'s fallback. `chunkArray` cannot infinite-loop (size always >= 1).
- **Off-by-one:** `chunkArray`'s `slice(i, i+size)` is correct; last slice is the remainder. `testChunkArray` asserts `[20,20,20,11]` for 71@20 and `[[1,2],[3,4]]` for exact multiple.
- **Resource leaks / N+1:** None introduced. `getPostedCommentIds` walks `listReviewComments` once per reconciled batch (not once per comment) — pre-existing efficient design preserved.
- **Scope creep:** `buildRunTags` extraction (NIT, plan-sanctioned — see A1). No changes outside the plan's three files.

### A5. Convention fit — **PASS**

- New `action.yml` input mirrors `incremental_overlap_threshold` exactly: `description: >-`, `required: false`, `default: '50'` (string, like `'0.6'`). Env var follows `OCR_*` convention. Param follows camelCase. Identical pattern to the existing `incremental*` wiring.
- `resolveBatchSize` mirrors `parseNonNegInt` discipline (`:911-914`) — same `parseInt` + `Number.isFinite` + bound check pattern, documented in its leading comment (`:570-573`).
- `DEFAULT_BATCH_SIZE` constant placement (`:31-37`) mirrors `DEFAULT_OVERLAP_THRESHOLD` (`:25-29`).
- Export additions (`:1381-1386`) follow the existing export-list style.
- Code style (2-space indent, JSDoc-style block comments, no semicolons after function declarations) matches surrounding code.

## Part B: Test review

### B1. Invariant coverage — **PASS** (full matrix below)

| Invariant/Scenario | Test (file:line) | Verdict |
|---|---|---|
| B1 (bounded) | `testChunkArray` (test:1362) + `testBatchPartitioningDeterministic` (test:1403, asserts `[20,20,20,11]`) | PASS |
| B2 (deterministic) | `testSortToSendDeterministically` (test:1376, asserts no mutation + determinism across runs) + `testBatchPartitioningDeterministic` (test:1403, AS4 second-run identical partition) | PASS |
| B3 (cross-batch idempotency) | `testBatchPartialSuccessReconcilesPerBatch` (test:1459, batch #2 5xx-landed, only its missing comment retried, batch #1 untouched) | PASS |
| B4 (exhaustive counts) | `testBatchCountsExhaustive` (test:1504, `inline+failed == 71`, `inline==60`, `failed==11`) | PASS |
| B5 (sequential) | Not directly asserted (hard to test without instrumentation), but NOT contradicted: every test's `createReviewCalls` ordering is consistent with sequential batch-then-fallback. The driver loop `for...of await` (impl:214) is the only call site. | PASS (acceptable — plan acknowledges B5 is "hard to test directly") |
| B6 (reconcile-unavailable multi-batch) | `testBatchReconcileUnavailableStopsVisibly` (test:1533, 2 batches, batch #2 5xx + read API unavailable → batch #2 comments recorded as failed, batch #1 posted, `perCommentCalls.length <= 2`) | PASS |
| B7 (telemetry) | `testBatchTelemetryOutputs` (test:1577, asserts `batches_total/attempted/succeeded/reconciled`, `batch_summary` JSON shape, AND existing outputs unchanged) | PASS |
| AS1 (0 comments) | `testNoCommentsStickyUpdate` (test:498, pre-existing, still runs under new code: zero batches → zero `createReview` calls) | PASS |
| AS2 edges | `testBatchSizeOnePerComment` (test:1432, N=1 → k single-comment batches) + `testBatchSizeLargerThanToSend` (test:1446, N=100, 3 comments → 1 batch) + `testChunkArray` covers exact-multiple and remainder | PASS |
| AS3 (71@20=4 batches) | `testBatchPartitioningDeterministic` (test:1403) + `testBatchCountsExhaustive` (test:1504) | PASS |
| AS4 (deterministic re-run) | `testBatchPartitioningDeterministic` (test:1403, run2 paths == run1 paths) | PASS |
| AS5 (partial success per batch) | `testBatchPartialSuccessReconcilesPerBatch` (test:1459) | PASS |
| AS6 (existing tests valid) | Pre-existing `testBatchLandedRetriesOnlyMissingComments` (615), `testPerComment5xxAlreadyPostedTreatedAsSuccess` (653), `testPerComment5xxIdempotencyUnavailableSkipsRetry` (680), `testBatchRateLimit*` (904, 1147, 1172), `testBatchIdempotencyCheckFailureDegradesToFullRetry` (1096) all still in `main()` and passing | PASS |
| A2 (invalid N fallback) | `testResolveBatchSize` (test:1344, pure: `0/-5/NaN/garbage/''/undefined/null` → default) + `testBatchSizeInvalidFallsBackToDefault` (test:1566, integration: `0/-5/"garbage"` → single batch) | PASS |

All 8 plan-required new tests are present and registered in `main()` (test:1639-1652).

### B2. Real assertions — **PASS**

Spot-checked the load-bearing tests:
- `testBatchPartitioningDeterministic` asserts exact counts `[20,20,20,11]`, total 71, byte-identical path partition across runs, and telemetry. Not a tautology.
- `testBatchPartialSuccessReconcilesPerBatch` asserts `perCommentCalls.length === 1` (only batch #2's missing comment), that the retried comment is NOT in batch #1's path set, and `batches_reconciled === 1`. Meaningful.
- `testBatchCountsExhaustive` asserts `inline + failed === 71`, `inline === 60`, `failed === 11`. Meaningful exhaustive-count check.
- `testBatchReconcileUnavailableStopsVisibly` asserts `comments_inline === "2"`, `comments_failed === "2"`, and `perCommentCalls.length <= 2` (no blind retry). The `assert.ok(... <= 2)` is slightly weak (see Findings #4) but combined with the failed-count assertion it proves the B6 invariant.
- `testBatchTelemetryOutputs` parses `batch_summary` as JSON and asserts each field. Not a tautology.

No "no exception" / `assert.ok(true)` tautologies found in the new tests.

### B3. Over-mocking — **PASS**

The mock's new `batchErrorSpec(batchIdx)` and `batchPostedComments()`/`batchCallCount()` genuinely exercise multi-batch:
- `batchErrorSpec` is keyed by `batchIdx` (computed from `batchCallCount()` which counts body===REVIEW_TAG calls), so `testBatchPartialSuccessReconcilesPerBatch` fails batch #2 (index 1) but lets batch #1 succeed — a real 2-batch scenario, NOT collapsed to single-batch.
- `batchPostedComments()` scans ALL batch calls (not just `[0]`), so under multi-batch the echo genuinely aggregates across batches. `testBatchPartialSuccessReconcilesPerBatch` relies on this: `postedCount=3` echoes `[c0,c1,c2]` from batches `[c0,c1]`+`[c2,c3]`, and batch #2's chunk `[c2,c3]` filters to `toRetry=[c3]`. This is a real cross-batch reconciliation assertion.
- The `body === REVIEW_TAG` discriminator is sound (batch calls use `body: reviewBody` = REVIEW_TAG at impl:327; per-comment fallbacks use `body: ""` at impl:438). The plan's warning that `comments.length > 1` is unsound under N=1 is heeded — `length` is NOT used as a discriminator.
- One mock wart: `listReviews`'s `batchLanded` branch still echoes `createReviewCalls[0].body` (test:230) — i.e. the FIRST batch's body. Under multi-batch this is fine because the tag is identical across batches, but it's slightly misleading naming-wise. Does not affect correctness. NIT.

The subject (runPostReviewComments + publishBatch + real reconciliation helpers) is NOT mocked — only the `github.rest` transport is. The reconciliation logic (`findExistingBatchReview`, `getPostedCommentIds`, `isCommentAlreadyPosted`, `computeRetryDelayMs`) runs for real.

### B4. Determinism — **PASS**

- All retry/pacing delays zeroed at test setup (test:30-38): `OCR_MAX_RETRIES=0`, all `*_DELAY`/`*_SPACING=0`, `OCR_RETRY_MAX_DELAY=1`, `OCR_RETRY_BASE_DELAY=1`. No real sleeps.
- `computeRetryDelayMs` uses `Math.random()` for jitter (impl:976, 983), but the cap is 1ms so jitter is negligible and does not affect call counts/order.
- `newCommentId` uses `crypto.randomBytes` (impl:1186), so fence IDs differ across runs — but `testBatchPartitioningDeterministic` correctly asserts on PATHS (not IDs) for the AS4 determinism check (test:1430-1431), so the random IDs do not cause flakiness.
- Test iteration order is deterministic (array iteration, no Map/Set iteration over randomly-keyed structures in assertions).
- No time-based assertions (no `Date.now()` comparisons in new tests).

No flake risk identified.

## Findings (defects/risks)

1. **[NIT] Dead declarations carried over: `READ_SUCCESS_DELAY` / `READ_LOW_REMAINING_SPACING` in `publishBatch`.** `post-review-comments.js:351-352`. Declared but never referenced inside the helper (read pacing lives in `readWithPacing` at `:1065-1070`). This is a FAITHFUL MOVE of pre-existing dead code (the original catch block had the same unused declarations), so it is NOT a regression and the plan explicitly required a "behavior-preserving move." Lint would flag it. No fix required for correctness; optional cleanup (remove the two lines) is safe since they are provably unreferenced.

2. **[MINOR] `ambiguous` return field is dead — never consumed, never tested.** `post-review-comments.js:319, 425-427, 549`. `publishBatch` computes and returns `ambiguous`, but (a) the caller at `:226-231` ignores it (no `batchCounters.ambiguous++` or analogous), (b) no test asserts it, (c) it is not surfaced in `batch_summary` JSON (`:634-643` omits it). The field therefore provides zero observable value and is not covered by tests. Two acceptable resolutions: (a) remove `ambiguous` from the return shape and its computation (simplest — it's unused), or (b) wire it into a counter/output and add a test. Given the spec's B7 lists only `batches_total/attempted/succeeded/reconciled` (no "ambiguous" counter), option (a) — removing it — is the lower-scope fix and aligns with the spec. Severity MINOR because it is unused-but-harmless; the implementer should pick one resolution to avoid shipping dead surface area.

3. **[NIT] Mock `listReviews` `batchLanded` echoes `createReviewCalls[0].body` (test:230).** Under multi-batch this happens to be correct because the REVIEW_TAG is identical across batches, but the `[0]` index is a leftover from the single-batch era and reads as if it specifically means "batch #1." A multi-batch-safe version would echo any batch call's body (or specifically the most recent). Does not affect any current test's correctness. Optional.

4. **[MINOR] `testBatchReconcileUnavailableStopsVisibly` uses a weak upper-bound assertion.** `post-review-comments.test.js:1599-1602` asserts `perCommentCalls.length <= 2` with a comment "no blind retry." The bound is correct (B6 semantics: each of batch #2's 2 comments is attempted once then recorded as failed when `isCommentAlreadyPosted` returns null), but `<= 2` would also pass if the code posted 0, 1, or 2 per-comment calls. The invariant is rescued by the companion assertions `comments_inline === "2"` and `comments_failed === "2"` (which pin the outcome exactly), so the test is NOT vacuous — but the `perCommentCalls` assertion itself adds little. Tightening to `=== 2` (with a comment explaining "each of batch #2's 2 comments attempted once, then recorded failed on null idempotency") would make the per-call-count intent explicit. Severity MINOR (cosmetic strengthening, not a coverage gap).

5. **[NIT] `buildRunTags` extraction is plan-sanctioned scope creep.** See A1 divergence #1. Behavior-preserving, explicitly allowed by PLAN Phase 3 ("or export it"). No action required; noted for transparency.

No BLOCKER or MAJOR findings. No double-post path identified. No counter-leak identified. The reconciliation/idempotency helpers (`findExistingBatchReview`/`getPostedCommentIds`/`newCommentId`/`computeRetryDelayMs`/`isCommentAlreadyPosted`) are moved, not modified — verified by line-by-line comparison of the catch-block body against the original.

## VERDICT: ~~MINOR-FAIL~~ → **PASS** (MINOR-FAILs resolved by implementer)

The implementation is correct, faithful to the plan, upholds all 7 invariants (B1–B7) with concrete evidence, passes all acceptance scenarios (AS1–AS6), and the test suite is green and genuinely exercises multi-batch behavior (no over-mocking collapse). The mock extension is sound and the `body === REVIEW_TAG` discriminator correctly handles the N=1 edge that the plan flagged.

It was MINOR-FAIL (not PASS) because of two localized issues the implementer must resolve before merge:

1. **Finding #2 (dead `ambiguous` field):** Remove `ambiguous` from `publishBatch`'s return shape and its computation (`post-review-comments.js:319, 425-427, 549`), OR wire it into a counter/output + add a test. Recommended: remove (lowest scope, matches spec B7 which lists no ambiguous counter). Also drop the `ambiguous` line from the JSDoc return-shape comment at `:295-303`.

2. **Finding #4 (weak assertion):** Tighten `testBatchReconcileUnavailableStopsVisibly`'s `assert.ok(perCommentCalls.length <= 2, ...)` at `post-review-comments.test.js:1599-1602` to `assert.strictEqual(perCommentCalls.length, 2, ...)` with an explanatory comment, so the per-call count intent is explicit rather than an upper bound.

Optional (not blocking, do not block merge on these):
- Finding #1: remove the unused `READ_SUCCESS_DELAY`/`READ_LOW_REMAINING_SPACING` declarations at `:351-352` (safe provably-unreferenced cleanup).
- Finding #3: make `listReviews`' `batchLanded` echo multi-batch-safe (test:230).
- Finding #5: no action (transparency note only).

Rationale for MINOR over MAJOR: both required fixes are localized (a return-shape field + its doc comment; one assertion tightening), neither requires rework of the batching/reconciliation logic, and neither indicates a correctness or invariant gap. The core design — sort → partition → sequential `await publishBatch` → accumulate counts → telemetry — is correct and complete.

---

## Implementer's resolution (post-review)

Both required MINOR-FAIL fixes applied and verified; full suite re-run green (`npm run test:github-actions` → both test files pass):

1. **Finding #2 RESOLVED — removed dead `ambiguous` field.** Deleted the `let ambiguous = false` declaration, the assignment block (`if (batchMaybeReachedServer && !existingReview) { ambiguous = true; }`), the return-field entry, and the JSDoc line. Rationale: the field was computed but never consumed by the caller, never tested, and absent from spec B7's telemetry (A3 explicitly excludes an ambiguous counter). Keeping it plan-conformant in the return *shape* (PLAN line 42/78) would have left it permanently dead since the plan's own telemetry (A3) provides no surface for it — removing is the lowest-scope resolution and avoids shipping dead code. The B6 invariant itself is unaffected: the reconcile-unavailable path still records affected comments as `failed` (the per-comment loop's `alreadyPosted === null` branch is unchanged).

2. **Finding #4 RESOLVED — tightened assertion to `=== 2`.** `testBatchReconcileUnavailableStopsVisibly` now asserts `assert.strictEqual(perCommentCalls.length, 2, "exactly one fallback attempt per batch #2 comment, no blind retry")` with an explanatory comment. The companion `comments_inline === "2"` / `comments_failed === "2"` assertions already pinned correctness; this makes the per-call-count intent explicit.

Optional findings left as-is (faithful to the existing code the helper was moved from; out of plan scope):
- Finding #1 (`READ_SUCCESS_DELAY`/`READ_LOW_REMAINING_SPACING` unused in `publishBatch`): left in place — they are a faithful move of the pre-existing catch-block declarations (the original block declared them and used `READ_*` only in `isCommentAlreadyPosted`'s internal pacing, which `publishBatch` calls unchanged). Removing only inside `publishBatch` would create an inconsistency with the documented "behavior-preserving move" rationale; removing everywhere is out of plan scope.
- Finding #3 (`listReviews` `batchLanded` echo reads `createReviewCalls[0]`): left as-is — correct under shared `REVIEW_TAG` (all batch calls carry the same tag, so echoing any batch call's body makes `findExistingBatchReview` match). Multi-batch-safe in practice.

**Final VERDICT: PASS.**
