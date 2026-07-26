# REASONING — Issue #479 (bounded comment batches)

Append-only log of non-obvious decisions and confusions. Survives compaction.

## Phase 0 — Orient
- Bundle: `thoughts/shared/plans/2026-07-25-issue-479-chunk-comment-batches/`. PLAN_VALIDATION verdict = MINOR-FAIL, but validator applied 3 fixes directly to PLAN.md (wording, REVIEW_TAG discriminator promotion, multi-batch B6 test). The post-fix PLAN.md is the source of truth and is sound. Scale: small.
- Worktree: created `../open-code-review-479` on branch `feat/479-bounded-comment-batches` from `main` (c9b1456). Plan bundle + research doc copied in (untracked, local reference).
- ARCHITECTURE.md / AGENTS.md referenced in the task prompt do NOT exist in this repo. CONTRIBUTING.md is the actual guideline doc (Go-focused, but Conventional Commits + focused-PR + sign-CLA conventions apply). No JS linter in repo (PLAN notes this).
- Verified every plan line citation against current source: bug site `:194-213`, publish block `:194-409`, setStatsOutputs `:461-467`, parseNonNegInt `:733-736`, action.yml input pattern `:75-89`/`:277`/`:295-305`. All match exactly.
- Baseline: `node scripts/github-actions/post-review-comments.test.js` → green.
- Plan validates against the current code (c9b1456 == HEAD, research staleness check clean).

## Key implementation notes from PLAN_VALIDATION findings (must honor)
1. **Mock discriminator**: primary = `params.body === REVIEW_TAG` (per-comment calls use `body === ""`). `comments.length > 1` is UNSOUND under the N=1 test. Must export REVIEW_TAG or reconstruct in test (test already imports internals via require at `:15`).
2. **publishBatch pacing env vars**: must re-read (or receive via config) the 7 env-derived pacing knobs (`MAX_RETRIES`, `SUCCESS_DELAY`, `FAILURE_DELAY`, `LOW_REMAINING_THRESHOLD`, `LOW_REMAINING_SPACING`, `READ_SUCCESS_DELAY`, `READ_LOW_REMAINING_SPACING`) — currently read INSIDE the catch at `:222-229`. Scoping config to only batchSize would silently drop per-comment pacing.
3. **B6 multi-batch test required**: existing single-batch `testPerComment5xxIdempotencyUnavailableSkipsRetry` does NOT exercise B6 across batch boundaries.
4. **findExistingBatchReview may match earlier batch's review** under multi-batch (REVIEW_TAG shared) — harmless for correctness but log messages must not assume single-batch. The `:280` log "Batch review already exists (review_id=...)" becomes misleading; reword.
5. **batchPostedComments() mock reads `createReviewCalls[0]`** — under multi-batch, batch calls are NOT all at index 0. This mock helper needs to read across ALL batch calls (those with body===REVIEW_TAG), or echoPosted/landedKeys logic breaks for batch #2+. Critical for AS5 test.

## Phase 3/4 review — MINOR-FAIL → PASS (resolved)

Fresh adversarial reviewer returned MINOR-FAIL with two localized fixes:
1. **Dead `ambiguous` field** — `publishBatch` computed/returned `ambiguous` but the caller ignored it, no test asserted it, and spec B7/A3 explicitly exclude an ambiguous counter. Resolved by removing it (lowest scope). The B6 invariant is unaffected (the reconcile-unavailable path still records failures via the unchanged per-comment `alreadyPosted === null` branch). Tension note: PLAN line 42/78 lists `ambiguous` in the return shape, but since A3 provides no telemetry surface for it, keeping it plan-conformant would ship permanent dead code — removal is the correct resolution.
2. **Weak assertion** — `testBatchReconcileUnavailableStopsVisibly` used `assert.ok(length <= 2)`. Tightened to `assert.strictEqual(length, 2)` with explanatory comment.

Optional findings left as-is (out of plan scope / faithful to moved code):
- `READ_SUCCESS_DELAY`/`READ_LOW_REMAINING_SPACING` unused directly in `publishBatch` — kept as a faithful move of the pre-existing catch-block declarations (used transitively via `isCommentAlreadyPosted`).
- Mock `listReviews` `batchLanded` echoes `createReviewCalls[0].body` — correct under shared REVIEW_TAG.

## Test red-verification
Confirmed `testChunkArray` (and transitively the integration tests) fail RED when `chunkArray` is neutered to return a single chunk, and pass GREEN when restored. Tests are meaningful, not no-ops.

## Mock multi-batch subtlety (resolved during Phase 3)
`batchPostedComments()` originally read `createReviewCalls[0]` only — under multi-batch, batch calls are NOT all at index 0. Rewrote to scan ALL createReviewCalls with `body === REVIEW_TAG`. Critical for AS5: `testBatchPartialSuccessReconcilesPerBatch` initially failed because `postedCount` semantics differ across batches (batch #1's successes ARE genuinely on the server, so the global posted set must include them). Fixed the test to reflect real semantics: `postedCount: 3` = batch #1's 2 + 1 of batch #2's.

## Phase 5 — security review
No BLOCKER/MAJOR/MINOR findings. One NIT: `resolveBatchSize` has no upper bound, so an absurd `review_comment_batch_size` (e.g. 100000) silently reverts to a single giant batch, reproducing the original failure mode. 
- Decision: NOT added. Spec A2 explicitly scopes validation to "N < 1 is rejected" — an upper clamp is an unplanned constraint nobody validated, and the workflow rule is "do not widen scope." The action.yml description already documents "Integer >= 1"; users setting absurd values get the behavior they configured. Flag as a potential follow-up issue, not an in-scope fix. (If the team wants a ceiling, it's a one-line addition to `resolveBatchSize` + a test, but that's a separate decision.)

Test suite: GREEN (post-review-comments + check-translation-sync).
