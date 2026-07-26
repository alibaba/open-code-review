<!-- SIGNPOST | 2/5: PLAN | single source of truth; implementation must conform — divergence means changing the plan, not improvising
     Prev: IMPLICIT_SPEC.md | Next: PLAN_VALIDATION.md -->
# Issue #479 — Bounded comment batches in the reusable Action
scale: small

## Overview
Chunk the single all-in-one `createReview` call (`post-review-comments.js:202-210`) into deterministic, bounded, sequentially-reconciled batches, with a configurable batch size and per-batch telemetry. Preserve the existing idempotency/reconciliation machinery and all current outputs.

## Current State
- **Bug site**: `scripts/github-actions/post-review-comments.js:194-213` — one `createReview` call posts ALL of `toSend` (`comments: toSend.map(({ reviewComment }) => reviewComment)`). No chunking anywhere (verified: no `slice`/`chunk` on `toSend`; only `Math.ceil` at `:789`,`:810` for delay formatting). [research Evidence Ledger, V]
- **`toSend` shape**: `[{ comment, reviewComment, id }]`, built at `:147`/`:153`/`:158-160`. `reviewComment` = `{ path, body, [start_line, line, start_side, side] }` (`:134-146`). `comment` has `path/start_line/end_line/content/suggestion_code/existing_code` — **no severity/category** (`:127-132`).
- **Reconciliation preserved-as-is**: `REVIEW_TAG` (`:199`), `findExistingBatchReview` (`:937`), `getPostedCommentIds` (`:953`), per-comment fence `newCommentId` (`:1007`), per-comment fallback loop (`:288-300`), error classification `computeRetryDelayMs` (`:751-813`). [research, V]
- **Outputs today**: 5 scalars via `setStatsOutputs` (`:462-466`): `comments_total/inline/skipped/failed/summary_comment_url`. `stats` accumulator at `:67-73`.
- **Input wiring**: `action.yml:272-305` runs `actions/github-script@v7`; the wrapper is inline JS in action.yml (lines ~288-305) that `require()`s the helper and calls `runPostReviewComments({...})`. Inputs flow `action.yml inputs` → `env: OCR_*` → inline `process.env.OCR_*` → function param. **No `core.getInput` in the script** — inputs are function params (`:31-41`). Style reference for a new input: `incremental` / `incremental_overlap_threshold` (`action.yml:75-89`, env at `:277`, param at `:304`).
- **Test harness**: `post-review-comments.test.js`, run via `node` (no jest). `makeGithub()` mock at `:84-251`; **dispatches strictly by `callIdx === 0` = batch, `callIdx >= 1` = per-comment** (`:147-156`). 4 partial-success tests assert `createReviewCalls.length` / `.comments` (`:578`,`:867`,`:949`,`:987`). Delays zeroed (`:21-29`).
- **Gates**: `npm run test:github-actions` (runs `post-review-comments.test.js` + `check-translation-sync.test.js`). No JS linter in repo; no Makefile at root. Go tooling in CONTRIBUTING.md is irrelevant to this pure-Node change.
- **Issue status**: OPEN, unassigned, no open PR referencing #479 (re-verified via `gh`). Reusable action script; no Go build involved.

## Desired End State
- `createReview` for inline comments is called ≥1 times, each with ≤ N comments, N configurable (default 50), batches sequential.
- Deterministic ordering (path → start_line → end_line → original index) applied before partitioning; identical reruns → identical batches.
- Partial-success in any batch reconciles per-batch against GitHub; no double-post; reconciliation-unavailable → that batch's comments recorded as failed (B6).
- Counts stay exhaustive & mutually exclusive across all batches; success+failed == original `toSend.length`.
- Existing 5 scalar outputs unchanged; new per-batch counters + one structured JSON output added.
- All existing tests pass; new tests cover AS1–AS6.
- Verify: `npm run test:github-actions` green; `action.yml` validates as parseable YAML.

## What We're NOT Doing
- **No severity/category ordering.** `comment` has no such fields; full schema change is sibling #478's scope. See A1.
- **No concurrent/parallel batches.** Sequential only (B5).
- **No change to reconciliation/idempotency logic itself** (`findExistingBatchReview`/`getPostedCommentIds`/`newCommentId`/`computeRetryDelayMs`) — it is reused per-batch, not rewritten.
- **No new YAML test harness** for `action.yml` (A5) — the input is validated manually.
- **No Go changes**, no CLI changes, no LLM changes.
- **No full counter expansion** (raw/duplicate/ambiguous/reconciled as distinct outputs) — see A3; only batching-relevant counters added.

## Approach
The publish block (`:194-413`) is a single batch + a single fallback path. We factor the *per-batch publish + reconcile* logic out of the inline block into a reusable helper that runs once per chunk, then drive it in a loop over the chunked, sorted `toSend`. The existing reconciliation/fallback code moves into that helper largely unchanged. Key insight on the reconciliation filter: `getPostedCommentIds` (`:953-973`) returns a **server-global** `Set` of every fence ID posted across ALL reviews on the PR — it is NEVER chunk-scoped at the source. Per-batch dedup works by filtering **the chunk's items** against that global set: `toRetry = chunk.filter(item => !postedIds.has(item.id))`. Because each comment's `id` is a globally-unique random token (`:133`, `:1007`), this correctly identifies which of *this batch's* comments already landed, regardless of which batch posted them. (Note: `REVIEW_TAG` is shared across all batches, so `findExistingBatchReview` may match an EARLIER batch's review under multi-batch — harmless for correctness since the subsequent `postedIds` filter excludes already-posted IDs, but the implementer must not assume the matched review is "this batch's"; log messages should be worded accordingly.)

**Key decisions (each tied to an invariant):**
1. **Sort once, before partitioning** (B2/AS4): `toSend` is immutable through incremental filtering; produce a sorted copy via a deterministic comparator. Sort happens after incremental filtering so the skipped set is unaffected (preserves `stats.skipped` accounting at `:161`).
2. **Partition into contiguous slices** (B1/AS2/AS3): `for (let i = 0; i < sorted.length; i += N) { sorted.slice(i, i+N) }`. Contiguity + sorted input ⇒ deterministic partition.
3. **Per-batch publish+reconcile helper** (B3/B5/B6): extract the existing `try{ createReview } catch{ cooldown → maybeReachedServer → findExistingBatchReview → getPostedCommentIds → filter → per-comment fallback }` block into `publishBatch(chunk, ctx)` returning `{ succeeded: number, failed: number, failedComments: [], reconciled: boolean, ambiguous: boolean }`. The loop accumulates these into the existing `successCount`/`failedCount`/`failedComments` and new per-batch counters. Sequential `await` (B5).
4. **`REVIEW_TAG` body on every batch's createReview** (B3): unchanged from today — the tag is what `findExistingBatchReview` matches. All batches share the run tag; server-side dedup via fence IDs (`getPostedCommentIds`) disambiguates which comments landed regardless of which batch.
5. **Config plumbing** (A2): new `action.yml` input `review_comment_batch_size` (default `'50'`) → `env: OCR_REVIEW_COMMENT_BATCH_SIZE` → `parseInt(process.env.OCR_REVIEW_COMMENT_BATCH_SIZE, 10)` in the inline wrapper → new `reviewCommentBatchSize` param on `runPostReviewComments` (default 50). Validate `N >= 1`; clamp/guard via a small `resolveBatchSize` helper (mirrors `parseNonNegInt` discipline at `:222`).

## Design Analysis  (scale: small — compact)

**Invariants → mechanism:**
| Invariant | Mechanism |
|---|---|
| B1 bounded | `resolveBatchSize` + `slice(i, i+N)` loop |
| B2 deterministic | comparator (path→start_line→end_line→origIndex) before partition |
| B3 cross-batch idempotency | reuse `getPostedCommentIds`+fence per batch; tag body on each `createReview` |
| B4 non-destructive | counts accumulated across batches; success+failed == toSend.length |
| B5 sequential | `for...of` with `await publishBatch(...)` |
| B6 reconcile-unavailable stops visibly | existing `failedCount++` path (`:364`) preserved inside helper |
| B7 telemetry | new per-batch counters + JSON output via `setStatsOutputs` extension |

**Failure & concurrency:** Single-threaded JS, sequential `await` per batch — no concurrent callers of `createReview`. Partial failure: each batch independently runs cooldown (`:236-244`) → maybeReachedServer → reconcile → per-comment fallback (`:288-300`). No double-apply: fence-ID dedup (`getPostedCommentIds`) filters already-posted IDs *within the current chunk* before per-comment retry. No half-write: a batch either succeeds wholesale, or reconciles+retries only the proven-missing subset, or records the irreconcilable remainder as failed.

**Blast radius:**
- `runPostReviewComments` callers: only the inline wrapper in `action.yml:295-305`. New param has a default → **back-compat** (existing `uses:` invocations without the input get N=50, which changes behavior only for runs with >50 inline comments — exactly the bug being fixed). Rollback: revert the commit; the script + action.yml change are atomic in one PR.
- `setStatsOutputs` (`:461-467`): extended with **additional** outputs only; existing 5 unchanged → existing consumers of `steps.post.outputs.comments_*` unaffected.
- Test mock (`test:147-156`): must be extended (A4). Existing tests that assert `createReviewCalls.length === 2` (batch + 1 fallback) **stay valid** when `toSend.length <= N` (default 50 ≥ all test inputs of 3-5 comments) → single batch, identical call pattern. No existing assertion breaks.
- `action.yml` consumers: anyone using `uses: alibaba/open-code-review@v*` gets the new input with a safe default; no action required.

**Default choices:** Followed the existing `incremental`/`incremental_overlap_threshold` pattern for the new input (action.yml `inputs` + `OCR_*` env + inline `process.env` + function param) — named because it is the established convention and there's no reason it fails here. Deviated from the issue's literal "severity/category" ordering (A1) because the data does not exist. Default N=50 (A2) rather than 20: keeps typical runs unchanged while fixing the failure.

## Resource & Cost Analysis
N/A — small scale, no perf-sensitive hot path. Batching *reduces* per-request payload size (the bug) and trades 1 `createReview` for ⌈k/N⌉ calls. For the 71-comment case at N=50: 2 calls instead of 1 (vs the failed 1-call-at-71). At N=20: 4 calls. Each call carries its own rate-limit pacing (`LOW_REMAINING_*` at `:225-226`), so the only added cost is a few extra sequential HTTP round-trips on large reviews — strictly better than the current 5xx-and-ambiguous-recovery path. No new in-memory structure; chunks are `slice` views over the existing `toSend` array.

## Phase 1: Sorting + partitioning + per-batch helper (the core)
### Changes
#### `scripts/github-actions/post-review-comments.js`
- Add `resolveBatchSize(raw, defaultSize=50)` (mirrors `parseNonNegInt` discipline): parse int, reject `< 1` and `NaN`, return default on invalid. Add a `DEFAULT_BATCH_SIZE = 50` constant near the other defaults.
- Add `sortToSendDeterministically(items)` returning a new sorted array (do not mutate caller's array): comparator on `comment.path` (localeCompare) → `comment.start_line` → `comment.end_line` → original index. Stable explicit tiebreak on original index guarantees AS4 even on engines where `sort` stability is incidental.
- Add `chunkArray(items, size)` returning `Array of slices`.
- Add `publishBatch({ chunk, github, owner, repo, prNumber, commitSha, reviewBody, log, ...config })` — extract the body of the current `try{...}catch{...}` block (`:201-413`) so it operates on `chunk` instead of all of `toSend`, and returns `{ succeeded, failed, failedComments, reconciled, ambiguous }`. The per-comment fallback loop (`:288-300`) runs over `chunk`'s retry subset. Reconciliation reads (`findExistingBatchReview`/`getPostedCommentIds`) are unchanged but their `toRetry` filter is applied to `chunk` (filter the chunk's items against the global posted-ID set — see §Approach). **State-passing requirement (behavior-preserving):** the current catch block reads 7 pacing env vars from `process.env` INSIDE the block (`:222-229`: `MAX_RETRIES`, `SUCCESS_DELAY`, `FAILURE_DELAY`, `LOW_REMAINING_THRESHOLD`, `LOW_REMAINING_SPACING`, `READ_SUCCESS_DELAY`, `READ_LOW_REMAINING_SPACING`). The helper MUST re-read these from `process.env` at its top (or receive them via `config`) — scoping `config` to only the new `batchSize` would silently drop per-comment pacing. This is a move, not a redesign.
- In `runPostReviewComments`, after incremental filtering produces `toSend` (`:158-160`) and before the publish block: `const batchSize = resolveBatchSize(...)`; `const sorted = sortToSendDeterministically(toSend)`; `const batches = chunkArray(sorted, batchSize)`.
- Replace the single-batch publish block (`:194-413`) with a sequential loop: `for (const chunk of batches) { const r = await publishBatch({chunk, ...}); successCount += r.succeeded; failedCount += r.failed; failedComments.push(...r.failedComments); batchCounters.attempted++; if (r.reconciled) batchCounters.reconciled++; if (r.succeeded>0) batchCounters.succeeded++; }`. Preserve the empty-`toSend` guard (`if (toSend.length > 0)`).
- Initialize new counters near `:190-192` alongside `successCount`/`failedCount`/`failedComments`: `const batchCounters = { total: batches.length, attempted: 0, succeeded: 0, reconciled: 0 }`.

#### `action.yml`
- Add input `review_comment_batch_size` (after `incremental_overlap_threshold`, `:89`), mirroring the `incremental*` style: `description: >-`, `required: false`, `default: '50'`.
- Add `OCR_REVIEW_COMMENT_BATCH_SIZE: ${{ inputs.review_comment_batch_size }}` to the `Post review comments` step `env:` (`:277`).
- Add `reviewCommentBatchSize: parseInt(process.env.OCR_REVIEW_COMMENT_BATCH_SIZE, 10)` to the `runPostReviewComments({...})` call (`:295-305`).

### Success Criteria
- [x] Automated: `npm run test:github-actions` green (existing tests unchanged in behavior; new tests added in Phase 2).
- [x] Automated: `node -e "require('js-yaml').load(require('fs').readFileSync('action.yml','utf8'))"` parses (or equivalent YAML load) — confirms the new input doesn't break YAML.
- [x] Manual: review the diff to confirm reconciliation code (`findExistingBatchReview`/`getPostedCommentIds`/`newCommentId`/`computeRetryDelayMs`) is moved, not modified. (Verified line-by-line in IMPLEMENTATION_VALIDATION.md Part A.)
- Pause for human manual verification before Phase 2.

## Phase 2: Telemetry outputs
### Changes
#### `scripts/github-actions/post-review-comments.js`
- Extend `setStatsOutputs` (`:461-467`) to **additionally** emit: `batches_total`, `batches_attempted`, `batches_succeeded`, `batches_reconciled`, and `batch_summary` (a JSON string: `{ total, attempted, succeeded, reconciled, batch_size, inline, failed }`). Existing 5 outputs unchanged.
- Pass `batchCounters` + `batchSize` into `setStatsOutputs` (extend its signature or pass a single `stats` object that now includes batch fields).

### Success Criteria
- [x] Automated: `npm run test:github-actions` green, including new assertions on the new outputs.
- [x] Manual: confirm existing `comments_*` outputs still asserted identically in pre-existing tests.

## Phase 3: Mock extension + new tests
### Changes
#### `scripts/github-actions/post-review-comments.test.js`
- Extend `makeGithub`'s `createReview` mock (`:147-175`) to distinguish batch calls from per-comment calls **without** relying on `callIdx === 0`, since N>1 batches break that assumption. **Required primary discriminator: `params.body === REVIEW_TAG`** — verified that batch calls use `body: reviewBody` (= `REVIEW_TAG`, `:199`/`:207`) while per-comment fallback calls use `body: ""` (`:297`), so this is unambiguous across all call paths. `params.comments.length > 1` is a SECONDARY signal only — it is UNSOUND under this plan's own `testBatchSizeOnePerComment` (N=1 → a single-comment batch collides with per-comment shape `comments: [reviewComment]`, `:299`); do not rely on it alone. (The test already imports module internals freely via the require at `:15`; `REVIEW_TAG` is reconstructable from the hardcoded `context.runId`/`runAttempt` in the test, or export it.) Replace the `callIdx === 0` batch check with the body predicate. Keep `bulkErrorSpec`/`bulkError` semantics but allow a **per-batch-index** error spec (e.g. `opts.batchErrorSpec(batchIndex)` or an array) so a test can fail batch #2 but not batch #1.
- Keep all existing tests passing: with default N=50 and test inputs of 3-5 comments, there is exactly one batch → `callIdx===0` semantics still hold → existing `createReviewCalls.length` assertions remain valid.
- Add new tests (each maps to an Acceptance Scenario):
  - `testBatchPartitioningDeterministic` (AS1/AS2/AS3): feed 71 comments, set `reviewCommentBatchSize: 20`, assert exactly 4 `createReview` calls with comment counts `[20,20,20,11]`, all `reviewComment.body` present, deterministic across two runs (AS4).
  - `testBatchSizeOnePerComment` (AS2 edge): N=1 → k calls each with 1 comment.
  - `testBatchSizeLargerThanToSend` (AS2 edge): N=100, 3 comments → 1 batch (no regression vs current behavior).
  - `testBatchPartialSuccessReconcilesPerBatch` (AS5): 2 batches, batch #2 throws 5xx but landed → only its missing comments retried; batch #1 comments untouched (no double-post). Requires the extended mock.
  - `testBatchCountsExhaustive` (B4): 71 comments, one batch partially fails irrecoverably → `comments_inline + comments_failed == 71`; `batches_total == 4`.
  - `testBatchReconcileUnavailableStopsVisibly` (B6, multi-batch): a ≥2-batch run where the idempotency read API (`listReviews`/`listReviewComments`) is unavailable mid-sequence. Assert the affected batch's comments are recorded as `failed` (NOT reposted, avoiding duplicates) and that earlier/later batches are undisturbed. This is required because the existing single-batch `testPerComment5xxIdempotencyUnavailableSkipsRetry` (`test:643-667`) runs under default N=50 → still one batch → it does NOT prove B6 holds across batch boundaries.
  - `testBatchSizeInvalidFallsBackToDefault` (A2): `reviewCommentBatchSize: 0` and `: -5` and `: "garbage"` → resolves to default 50 (single batch for test-sized input).
  - `testBatchTelemetryOutputs` (B7): assert `batches_total`, `batches_attempted`, `batches_succeeded`, `batches_reconciled`, and `batch_summary` JSON present and correct.

### Success Criteria
- [x] Automated: `npm run test:github-actions` green.
- [x] Manual: confirm new tests fail (red) when the implementation is reverted, proving they exercise the new behavior.

## Testing Strategy
- **Unit (node)**: `resolveBatchSize`, `sortToSendDeterministically`, `chunkArray` — pure-function tests (determinism, edge cases 0/1/N/N+1, invalid input). These are the load-bearing pure logic; test in isolation.
- **Integration (node + mock)**: the AS1–AS6 scenarios via the extended `makeGithub` mock, asserting on `createReviewCalls` shape, `outputs.*`, and `failedComments`.
- **Edges covered**: empty `toSend` (AS1), N≥`toSend.length` (single batch, no regression), N=1, invalid N (A2), partial-success per batch (AS5), reconciliation-unavailable per batch (B6).
- **Untestable offline (recorded, not blocking)**: real-GitHub cross-chunk idempotency (A4); `action.yml` YAML input wiring end-to-end (A5, manual). The new `action.yml` input is confirmed by YAML-parse + manual review.

## References
- Issue: [#479](https://github.com/alibaba/open-code-review/issues/479)
- Research doc: [`thoughts/shared/research/2026-07-25-issue-479-chunk-comment-batches.md`](../../research/2026-07-25-issue-479-chunk-comment-batches.md) (reused wholesale; staleness check clean at `c9b1456..HEAD`)
- Bug site: `scripts/github-actions/post-review-comments.js:194-213`
- Reconciliation to preserve: `:288-300`, `:937`, `:953`, `:1007`, `:751-813`
- Pattern to mirror (input wiring): `action.yml:75-89`/`:277`/`:304`; `post-review-comments.js:31-41`/`:222-229`
- Related: #158, #164, #183, #250, #337 (existing idempotency work — preserved, not reopened); #478 (severity routing — out of scope); #369 (stable finding identity)
